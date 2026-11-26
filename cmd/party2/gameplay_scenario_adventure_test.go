package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"
)

// runObserved is the part of the context a normal-adventure journey reads.
type runObserved struct {
	Character struct {
		Tired int `json:"tired"`
	} `json:"character"`
	Scene struct {
		Kind string `json:"kind"`
		Data struct {
			Activities []struct {
				Kind    string   `json:"kind"`
				ID      string   `json:"id"`
				Actions []string `json:"actions"`
			} `json:"activities"`
			Run *struct {
				RunID      string `json:"run_id"`
				Version    int    `json:"version"`
				Round      int    `json:"round"`
				LeaderID   string `json:"leader_id"`
				Operations []struct {
					Operation   string `json:"operation"`
					Submittable bool   `json:"submittable"`
				} `json:"operations"`
				Actors []struct {
					ID    string `json:"id"`
					HP    int    `json:"hp"`
					Enemy bool   `json:"enemy"`
				} `json:"actors"`
			} `json:"run"`
		} `json:"data"`
	} `json:"scene"`
	AvailableActions []struct {
		Action         string          `json:"action"`
		ParamsTemplate map[string]any  `json:"params_template"`
		ParamsSchema   json.RawMessage `json:"params_schema"`
	} `json:"available_actions"`
}

type runOutcome struct {
	Result struct {
		RunID       string `json:"run_id"`
		CommandID   string `json:"command_id"`
		Version     int    `json:"version"`
		Round       int    `json:"round"`
		Disposition string `json:"disposition"`
		Box         *struct {
			OpenedBy string `json:"opened_by"`
			ItemID   string `json:"item_id"`
		} `json:"box"`
		Recipient struct {
			Applied struct {
				EXP int `json:"experience"`
			} `json:"applied"`
		} `json:"recipient"`
	} `json:"result"`
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
	Context runObserved `json:"context"`
}

func (a *scenarioApp) runContext(token, hero string) runObserved {
	a.t.Helper()
	var c runObserved
	a.call(token, http.MethodGet, "/api/v1/characters/"+hero+"/context", "", http.StatusOK, &c)
	return c
}

// runCommand issues an offered run command with its observed identity.
func (a *scenarioApp) runCommand(token, hero string, observed runObserved, action string, inputs map[string]any, wantStatus int) runOutcome {
	a.t.Helper()
	var params map[string]any
	for _, offered := range observed.AvailableActions {
		if offered.Action == action {
			params = map[string]any{}
			for k, v := range offered.ParamsTemplate {
				params[k] = v
			}
		}
	}
	if params == nil {
		a.t.Fatalf("%s is not offered: %+v", action, observed.AvailableActions)
	}
	for k, v := range inputs {
		params[k] = v
	}
	body, err := json.Marshal(map[string]any{"action": action, "params": params})
	if err != nil {
		a.t.Fatal(err)
	}
	var out runOutcome
	a.call(token, http.MethodPost, "/api/v1/characters/"+hero+"/actions", string(body), wantStatus, &out)
	return out
}

// expireRunInterval ends actor's per-actor action interval through the run
// store, so the interval stays under test without a wall-clock wait.
func (a *scenarioApp) expireRunInterval(runID, actor string) {
	a.t.Helper()
	ctx := context.Background()
	var raw []byte
	if err := a.db.QueryRowContext(ctx, `SELECT state_json FROM runs WHERE run_id = ?`, runID).Scan(&raw); err != nil {
		a.t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		a.t.Fatal(err)
	}
	last, _ := state["last_actions"].(map[string]any)
	if last == nil {
		last = map[string]any{}
	}
	last[actor] = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	state["last_actions"] = last
	raw, err := json.Marshal(state)
	if err != nil {
		a.t.Fatal(err)
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE runs SET state_json = ? WHERE run_id = ?`, raw, runID); err != nil {
		a.t.Fatal(err)
	}
}

func submittable(o runObserved, operation string) bool {
	return o.Scene.Data.Run != nil && slices.ContainsFunc(o.Scene.Data.Run.Operations, func(op struct {
		Operation   string `json:"operation"`
		Submittable bool   `json:"submittable"`
	}) bool {
		return op.Operation == operation && op.Submittable
	})
}

// A solo normal run starts at floor 0 without combat, advances one encounter
// per explicit command, enforces the per-actor interval, survives a restart
// and settles one participant's exit; the exit receipt stays replayable.
func TestGameplayScenario_NormalAdventureSoloRun(t *testing.T) {
	a := newScenarioApp(t)
	token, hero := a.register()

	a.enter(token, hero, a.context(token, hero), "quest")
	started := a.runCommand(token, hero, a.runContext(token, hero), "adventure_start", map[string]any{"name": "solo-quest", "stage_id": "stage-00", "speed": 3, "command_id": "start-1"}, http.StatusOK)
	run := started.Context.Scene.Data.Run
	if started.Result.Version != 1 || started.Result.Round != 0 || run == nil || run.Round != 0 || run.LeaderID != hero ||
		slices.ContainsFunc(run.Actors, func(a struct {
			ID    string `json:"id"`
			HP    int    `json:"hp"`
			Enemy bool   `json:"enemy"`
		}) bool {
			return a.Enemy
		}) || !submittable(started.Context, "advance") {
		t.Fatalf("start resolved more than enrollment: %+v / %+v", started.Result, run)
	}
	runID := started.Result.RunID
	a.elapseActionWait(hero)

	advanced := a.runCommand(token, hero, a.runContext(token, hero), "run_operation", map[string]any{"command_id": "advance-1", "operation": "advance"}, http.StatusOK)
	if advanced.Result.Round != 1 || advanced.Context.Scene.Data.Run.Round != 1 || !submittable(advanced.Context, "attack") || submittable(advanced.Context, "advance") {
		t.Fatalf("advance: %+v / %+v", advanced.Result, advanced.Context.Scene.Data.Run)
	}

	// The per-actor interval rejects before mutation (_battle.cgi:641–644).
	observed := a.runContext(token, hero)
	foe := ""
	for _, actor := range observed.Scene.Data.Run.Actors {
		if actor.Enemy && actor.HP > 0 {
			foe = actor.ID
		}
	}
	early := a.runCommand(token, hero, observed, "run_operation", map[string]any{"command_id": "attack-early", "operation": "attack", "target_id": foe}, http.StatusConflict)
	if early.Error.Code != "RUN_COOLDOWN" || early.Context.Scene.Data.Run.Version != observed.Scene.Data.Run.Version {
		t.Fatalf("interval: %+v", early)
	}
	a.expireRunInterval(runID, hero)
	attacked := a.runCommand(token, hero, observed, "run_operation", map[string]any{"command_id": "attack-1", "operation": "attack", "target_id": foe}, http.StatusOK)
	if attacked.Result.Version != observed.Scene.Data.Run.Version+1 {
		t.Fatalf("attack: %+v", attacked.Result)
	}

	// SQL owns the run: a new process observes the same run without advancing it.
	a.restart()
	for range 2 {
		if again := a.runContext(token, hero); again.Scene.Data.Run == nil || again.Scene.Data.Run.Version != attacked.Result.Version {
			t.Fatalf("after restart: %+v", again.Scene.Data.Run)
		}
	}

	a.expireRunInterval(runID, hero)
	beforeExit := a.runContext(token, hero)
	exitParams := map[string]any{"command_id": "exit-1"}
	exited := a.runCommand(token, hero, beforeExit, "run_exit", exitParams, http.StatusOK)
	// Exit fatigue is round × 3 + 1 (_battle.cgi:1187–1188).
	if exited.Result.Disposition != "exit" || exited.Context.Scene.Data.Run != nil || exited.Context.Character.Tired != beforeExit.Character.Tired+4 {
		t.Fatalf("exit: %+v tired %d→%d", exited.Result, beforeExit.Character.Tired, exited.Context.Character.Tired)
	}
	var stored int
	if err := a.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM run_enrollments WHERE run_id = ? AND character_id = ?`, runID, hero).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("exit kept enrollment: %d", stored)
	}

	// The departed actor can still resolve that exit by identical retry.
	replayed := a.runCommand(token, hero, beforeExit, "run_exit", exitParams, http.StatusOK)
	if replayed.Result != exited.Result || replayed.Context.Character.Tired != exited.Context.Character.Tired {
		t.Fatalf("exit replay: %+v / %+v", replayed.Result, exited.Result)
	}
}

// A ready Party enrolls through the leader's Gateway start. Only the leader
// takes the first advance (vs_monster.cgi:37–82); members act on their own
// interval, and one participant's exit leaves the other continuing as leader
// until the last exit disposes the room (_battle.cgi:1184–1227,1330–1345).
func TestGameplayScenario_NormalAdventurePartyRun(t *testing.T) {
	a := newScenarioApp(t)
	leaderToken, leader := a.register()
	memberToken, member := a.register()

	// Only the creator meets stage-01's gate; a joiner and the start never
	// re-check it (quest.cgi:518–524, add_member, vs_monster.cgi:56–82).
	if _, err := a.db.ExecContext(context.Background(), `UPDATE characters SET level = 3 WHERE id = ?`, leader); err != nil {
		t.Fatal(err)
	}
	// Both meet at 冒険に出る: the leader creates, the member selects the lobby and joins.
	a.enter(leaderToken, leader, a.context(leaderToken, leader), "quest")
	a.offered(leaderToken, leader, a.context(leaderToken, leader), "party_create", map[string]any{"name": "scenario", "stage_id": "stage-01", "speed": 3, "max_members": 4}, http.StatusOK)
	quest := a.enter(memberToken, member, a.context(memberToken, member), "quest")
	parties := quest.Scene.Data.Parties
	i := slices.IndexFunc(parties, func(p struct {
		Name         string         `json:"name"`
		MemberNames  []string       `json:"member_names"`
		SelectParams map[string]any `json:"select_params"`
	}) bool {
		return p.Name == "scenario"
	})
	if i < 0 || len(parties[i].MemberNames) != 1 {
		t.Fatalf("lobby not listed: %+v", parties)
	}
	a.offered(memberToken, member, quest, "scene_select", parties[i].SelectParams, http.StatusOK)
	a.offered(memberToken, member, a.context(memberToken, member), "party_join", nil, http.StatusOK)
	// ＠パーティー and ＠さんか end in reload, which starts the action wait (system.cgi:438–440).
	a.elapseActionWait(leader)
	a.elapseActionWait(member)

	started := a.runCommand(leaderToken, leader, a.runContext(leaderToken, leader), "party_start", nil, http.StatusOK)
	runID := started.Result.RunID
	if started.Result.Round != 0 || started.Context.Scene.Data.Run == nil || started.Context.Scene.Data.Run.RunID != runID {
		t.Fatalf("party start: %+v", started)
	}
	observed := a.runContext(memberToken, member)
	if observed.Scene.Data.Run == nil || observed.Scene.Data.Run.RunID != runID || len(observed.Scene.Data.Run.Actors) != 2 {
		t.Fatalf("member does not observe the run: %+v", observed.Scene)
	}
	early := a.runCommand(memberToken, member, observed, "run_operation", map[string]any{"command_id": "m-advance", "operation": "advance"}, http.StatusForbidden)
	if early.Error.Code != "RUN_FORBIDDEN" || early.Context.Scene.Data.Run.Version != observed.Scene.Data.Run.Version {
		t.Fatalf("nonleader first advance: %+v", early)
	}

	a.elapseActionWait(leader)
	advanced := a.runCommand(leaderToken, leader, a.runContext(leaderToken, leader), "run_operation", map[string]any{"command_id": "l-advance", "operation": "advance"}, http.StatusOK)
	if advanced.Result.Round != 1 {
		t.Fatalf("leader advance: %+v", advanced.Result)
	}
	observed = a.runContext(memberToken, member)
	foe := ""
	for _, actor := range observed.Scene.Data.Run.Actors {
		if actor.Enemy && actor.HP > 0 {
			foe = actor.ID
		}
	}
	acted := a.runCommand(memberToken, member, observed, "run_operation", map[string]any{"command_id": "m-attack", "operation": "attack", "target_id": foe}, http.StatusOK)
	if acted.Result.Version != observed.Scene.Data.Run.Version+1 {
		t.Fatalf("member action: %+v", acted.Result)
	}

	a.expireRunInterval(runID, leader)
	leaderExit := map[string]any{"command_id": "l-exit"}
	beforeExit := a.runContext(leaderToken, leader)
	if exited := a.runCommand(leaderToken, leader, beforeExit, "run_exit", leaderExit, http.StatusOK); exited.Result.Disposition != "exit" || exited.Context.Scene.Data.Run != nil {
		t.Fatalf("leader exit: %+v", exited)
	}
	observed = a.runContext(memberToken, member)
	if observed.Scene.Data.Run == nil || observed.Scene.Data.Run.LeaderID != member || len(observed.Scene.Data.Run.Actors) == 0 ||
		slices.ContainsFunc(observed.Scene.Data.Run.Actors, func(a struct {
			ID    string `json:"id"`
			HP    int    `json:"hp"`
			Enemy bool   `json:"enemy"`
		}) bool {
			return a.ID == leader
		}) {
		t.Fatalf("member did not continue as leader without the exited actor: %+v", observed.Scene.Data.Run)
	}
	// Another participant cannot read or reuse the leader's receipt.
	foreign := a.runCommand(memberToken, member, observed, "run_exit", leaderExit, http.StatusForbidden)
	if foreign.Error.Code != "RUN_FORBIDDEN" || foreign.Context.Scene.Data.Run.Version != observed.Scene.Data.Run.Version {
		t.Fatalf("foreign receipt: %+v", foreign)
	}

	a.expireRunInterval(runID, member)
	if last := a.runCommand(memberToken, member, a.runContext(memberToken, member), "run_exit", map[string]any{"command_id": "m-exit"}, http.StatusOK); last.Result.Disposition != "exit" || last.Context.Scene.Data.Run != nil {
		t.Fatalf("last exit: %+v", last)
	}
	var remaining int
	if err := a.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM run_enrollments WHERE run_id = ?`, runID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("run kept %d enrollments after the last exit", remaining)
	}
}
