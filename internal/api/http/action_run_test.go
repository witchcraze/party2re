package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/runhost"
)

// runStub records run port calls and keeps receipts like the run host:
// a stored identity replays only for identical input.
type runStub struct {
	t         *testing.T
	obs       *runhost.Observation
	receipts  map[string]runhost.Receipt
	commands  []runhost.Command
	enters    []string
	err       error
	recovered int
	tames     []battle.MonsterTameResult
	after     func()
}

func (s *runStub) Command(_ context.Context, cmd runhost.Command) (runhost.Receipt, error) {
	s.commands = append(s.commands, cmd)
	if r, ok := s.receipts[cmd.RunID+"/"+cmd.ID]; ok {
		if r.Command != cmd {
			return runhost.Receipt{}, runhost.ErrConflict
		}
		return r, nil
	}
	if s.err != nil {
		return runhost.Receipt{}, s.err
	}
	if s.receipts == nil {
		s.receipts = map[string]runhost.Receipt{}
	}
	r := runhost.Receipt{Command: cmd, Version: cmd.Version + 1, Round: 1, Disposition: "", MonsterTames: s.tames}
	if cmd.Action == "exit" {
		r.Disposition = "exit"
	}
	s.receipts[cmd.RunID+"/"+cmd.ID] = r
	if s.after != nil {
		s.after()
	}
	return r, nil
}

func (s *runStub) GetRunReceipt(_ context.Context, player, actor, runID, key string) (*runhost.Receipt, error) {
	r, ok := s.receipts[runID+"/"+key]
	if !ok {
		return nil, nil
	}
	if r.Command.PlayerID != player || r.Command.ActorID != actor {
		return nil, runhost.ErrForbidden
	}
	return &r, nil
}

func (s *runStub) ObserveRun(_ context.Context, player, actor string) (*runhost.Observation, error) {
	if player != "owner" || actor != "hero" {
		return nil, runhost.ErrForbidden
	}
	return s.obs, nil
}

func (s *runStub) RecoverTimeout(context.Context, string, string, string) error {
	s.recovered++
	return s.err
}

func (s *runStub) EnterSolo(_ context.Context, player, actor, key, stage, mode string, speed int) (runhost.Receipt, error) {
	s.enters = append(s.enters, fmt.Sprintf("%s/%s/%s/%s/%s/%d", player, actor, key, stage, mode, speed))
	if s.err != nil {
		return runhost.Receipt{}, s.err
	}
	return runhost.Receipt{Command: runhost.Command{PlayerID: player, ActorID: actor, RunID: "run", ID: "enroll", Action: "enroll"}, Version: 1}, nil
}

func (s *runStub) EnterParty(_ context.Context, player, actor, partyID string) (runhost.Receipt, error) {
	s.enters = append(s.enters, player+"/"+actor+"/"+partyID)
	if s.err != nil {
		return runhost.Receipt{}, s.err
	}
	return runhost.Receipt{Command: runhost.Command{PlayerID: player, ActorID: actor, RunID: "run", ID: "enroll", Action: "enroll"}, Version: 1}, nil
}

type runGatewayFixture struct {
	*gatewayFixture
	runs  *runStub
	facts []playercontext.Activity
}

func newRunGatewayFixture(t *testing.T) *runGatewayFixture {
	f := &runGatewayFixture{gatewayFixture: newGatewayFixture(t), runs: &runStub{t: t}}
	f.char.Level = 10
	f.runs.obs = &runhost.Observation{
		ActorRun: runhost.ActorRun{RunID: "run", Version: 3, Round: 2, LeaderID: "hero", Role: "participant"},
		Speed:    18,
		Actors:   []runhost.ObservedActor{{ID: "hero", Name: "Hero", TeamID: "allies", HP: 40, MaxHP: 50, Leader: true}, {ID: "foe", Name: "Slime", TeamID: "enemies", HP: 9, MaxHP: 9, NPC: true, Enemy: true}},
		Boxes:    []runhost.ObservedBox{{ID: "box-open", Name: "宝箱", OpenedBy: "hero", ItemName: "やくそう"}, {ID: "box-closed", Name: "宝箱"}},
		Choices: []runhost.OperationChoice{
			{Action: "attack", Name: "attack", Kind: runhost.OperationBattle, Submittable: true},
			{Action: "job-01-entry-0", Name: "slash", Kind: runhost.OperationBattle, MPCost: 3, Gated: true},
			{Action: "whisper", Name: "whisper", Kind: runhost.OperationWhisper},
			{Action: "exit", Name: "exit", Kind: runhost.OperationRoom, Submittable: true},
		},
	}
	f.facts = []playercontext.Activity{{Kind: "adventure", ID: "run", Revision: 3, Role: "participant", Phase: "active", Round: 2, Actions: []string{"run_operation", "run_exit"}}}
	return f
}

func (f *runGatewayFixture) router() http.Handler {
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return f.facts, nil }))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithNormalRuns(f.runs), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func (f *runGatewayFixture) post(router http.Handler, action, params string) (int, map[string]json.RawMessage) {
	return gatewayRequest(f.t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":`+params+`}`, nil)
}

func decodeRunReceipt(t *testing.T, got map[string]json.RawMessage) RunReceiptResponse {
	t.Helper()
	var r RunReceiptResponse
	if err := json.Unmarshal(got["result"], &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// Solo start enrolls a floor-0 run through the run port; it resolves no
// combat and returns the run identity.
func TestRunGatewaySoloStartEnrollsFloorZero(t *testing.T) {
	f := newRunGatewayFixture(t)
	f.facts = nil
	router := f.router()
	status, got := f.post(router, "adventure_start", `{"stage_id":"stage-01","speed":18,"command_id":"k1"}`)
	if r := decodeRunReceipt(t, got); status != 200 || r.RunID != "run" || r.Version != 1 || r.Round != 0 || !slices.Equal(f.runs.enters, []string{"owner/hero/k1/stage-01/normal/18"}) {
		t.Fatalf("start: %d %s %v", status, got, f.runs.enters)
	}
	for _, params := range []string{`{"stage_id":"stage-01","speed":18}`, `{"stage_id":"stage-01","command_id":"k"}`, `{"stage_id":"stage-01","speed":18,"command_id":null}`,
		`{"stage_id":"stage-01","speed":18,"command_id":"k","mode":"try"}`, `{"stage_id":"stage-01","speed":18,"command_id":"k","character_id":"other"}`, `{"stage_id":"stage-01","speed":18,"command_id":""}`} {
		f.runs.enters = nil
		if status, _ := f.post(router, "adventure_start", params); status != 400 || len(f.runs.enters) != 0 {
			t.Fatalf("%s: status=%d enters=%v", params, status, f.runs.enters)
		}
	}
	for err, want := range map[error]string{runhost.ErrBusy: "RUN_BUSY", adventure.ErrStageNotFound: "ADVENTURE_STAGE_NOT_FOUND", runhost.ErrCharacterExhausted: "ADVENTURE_EXHAUSTED"} {
		f.runs.err = err
		if status, got := f.post(router, "adventure_start", `{"stage_id":"stage-01","speed":18,"command_id":"k2"}`); status < 400 || status >= 500 {
			t.Fatalf("%v: %d %s", err, status, got)
		} else {
			assertGatewayError(t, got, want)
		}
	}
	f.runs.err = errors.Join(runhost.ErrRecovery, runhost.ErrConflict)
	if status, got := f.post(router, "adventure_start", `{"stage_id":"stage-01","speed":18,"command_id":"k3"}`); status != 500 {
		t.Fatalf("unknown commit presented as rejection: %d %s", status, got)
	} else {
		assertGatewayError(t, got, "EXECUTION_FAILED")
	}
}

// The scene offers only submittable choices with the observed identity, and
// commands carry the typed selection to the run host unchanged.
func TestRunGatewayOperationSceneAndCommand(t *testing.T) {
	f := newRunGatewayFixture(t)
	router := f.router()
	status, observation := navigationGET(t, router)
	if status != 200 {
		t.Fatalf("GET: %d", status)
	}
	data := decodeSceneData[ActivitySceneData](t, observation)
	if data.Run == nil || data.Run.Version != 3 || len(data.Run.Operations) != 4 || data.Run.Boxes[1].ItemName != "" || data.Run.Boxes[0].ItemName != "やくそう" {
		t.Fatalf("run scene: %+v", data.Run)
	}
	var offered *ContextAction
	for i, a := range observation.AvailableActions {
		if a.Action == "run_operation" {
			offered = &observation.AvailableActions[i]
		}
	}
	if offered == nil || offered.ParamsTemplate["run_id"] != "run" || offered.ParamsTemplate["version"] != float64(3) {
		t.Fatalf("missing run_operation template: %+v", observation.AvailableActions)
	}
	var schema struct {
		Properties map[string]struct{ Enum []string } `json:"properties"`
	}
	if err := json.Unmarshal(offered.ParamsSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(schema.Properties["operation"].Enum, []string{"attack"}) || !slices.Equal(schema.Properties["target_id"].Enum, []string{"hero", "foe", "box-closed"}) {
		t.Fatalf("offered inputs: %+v", schema.Properties)
	}
	if len(f.runs.commands) != 0 {
		t.Fatal("GET executed a command")
	}

	status, got := f.post(router, "run_operation", `{"run_id":"run","version":3,"command_id":"c1","operation":"attack","target_id":"foe","input":"7","second_operation":"defend","second_target_id":"hero","follow_up_operation":"slash","follow_up_target_id":"foe"}`)
	want := runhost.Command{PlayerID: "owner", ActorID: "hero", RunID: "run", ID: "c1", Version: 3, Action: "attack", TargetID: "foe", Input: "7", SecondAction: "defend", SecondTargetID: "hero", FollowUpAction: "slash", FollowUpTargetID: "foe"}
	if status != 200 || len(f.runs.commands) != 1 || f.runs.commands[0] != want || decodeRunReceipt(t, got).Version != 4 {
		t.Fatalf("operation: %d %s %+v", status, got, f.runs.commands)
	}
	for _, params := range []string{
		`{"run_id":"run","version":3,"command_id":"c2","operation":"exit"}`,
		`{"run_id":"run","version":3,"command_id":"c2","operation":"chest"}`,
		`{"run_id":"run","version":3,"command_id":"c2","operation":""}`,
		`{"run_id":"run","version":3,"command_id":"c2","operation":"attack","target_id":null}`,
		`{"run_id":"run","version":3,"command_id":"c2","operation":"attack","extra":1}`,
		`{"run_id":"run","version":"3","command_id":"c2","operation":"attack"}`,
		`{"run_id":"run","version":0,"command_id":"c2","operation":"attack"}`,
	} {
		if status, _ := f.post(router, "run_operation", params); status != 400 || len(f.runs.commands) != 1 {
			t.Fatalf("%s: status=%d commands=%d", params, status, len(f.runs.commands))
		}
	}
	for err, want := range map[error]string{runhost.ErrConflict: "RUN_CONFLICT", runhost.ErrCooldown: "RUN_COOLDOWN", runhost.ErrForbidden: "RUN_FORBIDDEN", runhost.ErrUnavailable: "RUN_OPERATION_UNAVAILABLE"} {
		f.runs.err = err
		status, got := f.post(router, "run_operation", `{"run_id":"run","version":3,"command_id":"c3","operation":"attack"}`)
		if status < 400 || status >= 500 || string(got["context"]) == "null" {
			t.Fatalf("%v: %d %s", err, status, got)
		}
		assertGatewayError(t, got, want)
	}
}

// After personal exit the actor has no run activity; an identical retry still
// returns the stored receipt, while foreign or changed identities do not.
func TestRunGatewayExitReceiptReplay(t *testing.T) {
	f := newRunGatewayFixture(t)
	f.runs.after = func() { f.facts = nil }
	router := f.router()
	exit := `{"run_id":"run","version":3,"command_id":"leave"}`
	status, got := f.post(router, "run_exit", exit)
	first := decodeRunReceipt(t, got)
	if status != 200 || first.Disposition != "exit" || f.runs.commands[0].Action != "exit" {
		t.Fatalf("exit: %d %s", status, got)
	}
	status, got = f.post(router, "run_exit", exit)
	if status != 200 || !reflect.DeepEqual(decodeRunReceipt(t, got), first) {
		t.Fatalf("receipt replay after exit: %d %s", status, got)
	}
	if status, got := f.post(router, "run_exit", `{"run_id":"run","version":3,"command_id":"other"}`); status != 409 {
		t.Fatalf("new command after exit: %d %s", status, got)
	} else {
		assertGatewayError(t, got, "ACTION_UNAVAILABLE")
	}
	if status, got := f.post(router, "run_exit", `{"run_id":"run","version":4,"command_id":"leave"}`); status != 409 {
		t.Fatalf("changed input replayed: %d %s", status, got)
	} else {
		assertGatewayError(t, got, "RUN_CONFLICT")
	}
	stored := f.runs.receipts["run/leave"]
	stored.Command.PlayerID = "intruder"
	f.runs.receipts["run/leave"] = stored
	if status, _ := f.post(router, "run_exit", exit); status != 409 {
		t.Fatalf("foreign receipt replayed: %d", status)
	}
}

// Recovery resumes a committed cleanup's pending penalty instead of issuing
// another cleanup; otherwise it requests the owner cleanup.
func TestRunGatewayRecover(t *testing.T) {
	f := newRunGatewayFixture(t)
	f.facts[0].Actions = []string{"run_recover"}
	router := f.router()
	if status, got := f.post(router, "run_recover", `{"run_id":"run","version":3,"command_id":"r1"}`); status != 200 || f.runs.commands[0].Action != "cleanup" || f.runs.recovered != 0 {
		t.Fatalf("cleanup: %d %s", status, got)
	}
	f.runs.obs.Recipient.TimerPending = true
	if status, got := f.post(router, "run_recover", `{"run_id":"run","version":3,"command_id":"r2"}`); status != 200 || len(f.runs.commands) != 1 || f.runs.recovered != 1 {
		t.Fatalf("resume: %d %s", status, got)
	}
}

// Chest inspection offers only unopened boxes and sends the selected box.
func TestRunGatewayChest(t *testing.T) {
	f := newRunGatewayFixture(t)
	f.runs.obs.Choices = append(f.runs.obs.Choices, runhost.OperationChoice{Action: "chest", Name: "chest", Kind: runhost.OperationRoom, Submittable: true})
	f.facts[0].Actions = append(f.facts[0].Actions, "run_chest")
	router := f.router()
	_, observation := navigationGET(t, router)
	var schema struct {
		Properties map[string]struct{ Enum []string } `json:"properties"`
	}
	for _, a := range observation.AvailableActions {
		if a.Action == "run_chest" {
			if err := json.Unmarshal(a.ParamsSchema, &schema); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !slices.Equal(schema.Properties["box_id"].Enum, []string{"box-closed"}) {
		t.Fatalf("offered boxes: %+v", schema.Properties)
	}
	status, got := f.post(router, "run_chest", `{"run_id":"run","version":3,"command_id":"open","box_id":"box-closed"}`)
	if status != 200 || len(f.runs.commands) != 1 || f.runs.commands[0].Action != "chest" || f.runs.commands[0].TargetID != "box-closed" {
		t.Fatalf("chest: %d %s %+v", status, got, f.runs.commands)
	}
}

func TestRunGatewayPartyStartAndTames(t *testing.T) {
	f := newRunGatewayFixture(t)
	f.facts = []playercontext.Activity{{Kind: "party", ID: "p1", Role: "leader", Phase: "recruiting", Actions: []string{"party_start"}}}
	router := f.router()
	if status, got := f.post(router, "party_start", `{"party_id":"p1"}`); status != 200 || !slices.Equal(f.runs.enters, []string{"owner/hero/p1"}) || decodeRunReceipt(t, got).Version != 1 {
		t.Fatalf("party start: %d %s %v", status, got, f.runs.enters)
	}

	f = newRunGatewayFixture(t)
	f.runs.tames = []battle.MonsterTameResult{{MonsterID: "monster-002", MonsterName: "スライム", TamerName: "アレックス", Success: true}}
	status, got := f.post(f.router(), "run_operation", `{"run_id":"run","version":3,"command_id":"c1","operation":"attack"}`)
	if r := decodeRunReceipt(t, got); status != 200 || len(r.Tames) != 1 || r.Tames[0].Message == "" || r.Tames[0].MonsterID != "monster-002" {
		t.Fatalf("taming outcome lost: %d %s", status, got)
	}
}
