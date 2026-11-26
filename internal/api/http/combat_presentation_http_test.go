package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestCombatPresentationHTTPPaths(t *testing.T) {
	logs := []corebattle.TurnLog{{Turn: 1, Kind: "damage", ActorID: "c1", ActorName: "Hero", TargetID: "enemy", TargetName: "Slime", ActionName: "攻撃", DamageDealt: 12, RemainingHP: map[string]int{"c1": 100, "enemy": 0}}}
	solo := corebattle.Result{Outcome: corebattle.OutcomeWin, Logs: logs}
	dungeonResult := dungeon.ExpeditionStepResult{EventType: dungeon.EventBattle, Monsters: []dungeon.DungeonMonster{{Name: "Slime", ExpReward: 2, GoldReward: 3}}, CombatOutcome: corebattle.OutcomeWin, BattleResult: &solo}
	moveResult := dungeonResult
	moveResult.EventType = dungeon.EventBoss
	moveResult.MemberDamages = map[string]int{"c1": 90}
	ds := &stubDungeonService{
		moveFn: func(context.Context, string, dungeon.Direction) (dungeon.ExpeditionStepResult, error) {
			return moveResult, nil
		},
		fightFn:  func(context.Context, string) (dungeon.ExpeditionStepResult, error) { return dungeonResult, nil },
		escapeFn: func(context.Context, string) (dungeon.ExpeditionStepResult, error) { return dungeonResult, nil },
	}
	cs := &stubChallengeService{advanceRoundFn: func(context.Context, string, string, int) (*challenge.RoundResult, *challenge.ChallengeSession, error) {
		return &challenge.RoundResult{BattleResult: solo}, &challenge.ChallengeSession{}, nil
	}}
	ps := &stubColosseumPvPService{advanceRoundFn: func(context.Context, string, string) (pvp.RoundResolution, error) {
		return pvp.RoundResolution{BattleLog: logs}, nil
	}}
	gs := &stubGvGService{advanceRoundFn: func(context.Context, string, string, int) (gvg.RoundResolution, error) {
		return gvg.RoundResolution{BattleLog: logs}, nil
	}}
	rs := &stubReplayService{getReplayFn: func(context.Context, string, string) (*replay.BattleReplay, error) {
		return &replay.BattleReplay{TurnLogs: json.RawMessage(replay.EncodeJSON(logs))}, nil
	}}
	h := newTestHandler(t, &stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p1"})}, &stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) {
		return corecharacter.Character{ID: "c1", PlayerID: "p1"}, nil
	}}, &stubAdventureService{}, &stubShopService{}, apihttp.WithDungeon(ds), apihttp.WithChallenge(cs), apihttp.WithPvP(ps), apihttp.WithGvG(gs), apihttp.WithReplay(rs))
	for _, tc := range []struct {
		name, path, body string
		keys             []string
	}{
		{"dungeon move", "/characters/c1/dungeons/move", `{"direction":"north"}`, []string{"result", "battle_result", "logs"}},
		{"dungeon fight", "/characters/c1/dungeons/fight", "", []string{"result", "battle_result", "logs"}},
		{"dungeon escape", "/characters/c1/dungeons/escape", "", []string{"result", "battle_result", "logs"}},
		{"challenge", "/characters/c1/challenges/advance", `{"session_id":"run","expected_revision":1}`, []string{"round_result", "battle_result", "logs"}},
		{"pvp", "/characters/c1/pvp/rooms/room/advance", "", []string{"battle_log"}},
		{"gvg", "/characters/c1/gvg/rooms/room/advance", `{"expected_round":1}`, []string{"battle_log"}},
		{"replay", "/replays/saved", "", []string{"turn_logs"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodPost
			if tc.name == "replay" {
				method = http.MethodGet
			}
			req := jsonRequest(t, method, tc.path, tc.body)
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var value map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			for _, key := range tc.keys[:len(tc.keys)-1] {
				if err := json.Unmarshal(value[key], &value); err != nil {
					t.Fatalf("%s: %v", rec.Body.String(), err)
				}
			}
			switch tc.name {
			case "dungeon move":
				var damages map[string]int
				if err := json.Unmarshal(value["member_damages"], &damages); err != nil || damages["c1"] != 90 {
					t.Fatalf("missing member_damages: %s err=%v", rec.Body.String(), err)
				}
			case "dungeon fight":
				if _, ok := value["member_damages"]; ok {
					t.Fatalf("unexpected member_damages on fight: %s", rec.Body.String())
				}
			}
			var got []struct {
				corebattle.TurnLog
				Message string `json:"message"`
			}
			if err := json.Unmarshal(value[tc.keys[len(tc.keys)-1]], &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Message != "Hero の 攻撃！ Slime に 12 のダメージ！" || got[0].Kind != "damage" || got[0].RemainingHP["c1"] != 100 {
				t.Fatalf("lost presentation/facts: %s", rec.Body.String())
			}
		})
	}
}

func TestReplayPresentationPreservesHistoricalText(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		status        int
	}{
		{"historical", `[{"turn":1,"message":"保存された戦闘記録"}]`, 200},
		{"malformed", `{"message":"not a log array"}`, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &stubReplayService{getReplayFn: func(context.Context, string, string) (*replay.BattleReplay, error) {
				return &replay.BattleReplay{TurnLogs: json.RawMessage(tc.payload)}, nil
			}}
			h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{}, apihttp.WithReplay(service))
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/replays/old", nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.status == 200 {
				var response struct {
					TurnLogs []struct {
						Message string `json:"message"`
					} `json:"turn_logs"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if len(response.TurnLogs) != 1 || response.TurnLogs[0].Message != "保存された戦闘記録" {
					t.Fatalf("lost archived text: %s", rec.Body.String())
				}
			}
		})
	}
}
