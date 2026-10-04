package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type adventureGatewayFixture struct {
	*gatewayFixture
	AdventureService
	service     *adventure.Service
	saved       adventure.Adventure
	saves       int
	saveErr     error
	beforeStart func()
}

func newAdventureGatewayFixture(t *testing.T) *adventureGatewayFixture {
	f := &adventureGatewayFixture{gatewayFixture: newGatewayFixture(t)}
	f.char.Level = 1
	f.char.Stats.MaxHP = 50
	return f
}

func (f *adventureGatewayFixture) StartStage(ctx context.Context, actor, stage string) (adventure.Adventure, error) {
	f.checkContext(ctx)
	if actor != "hero" {
		f.t.Fatalf("wrong actor: %s", actor)
	}
	f.executions++
	if f.beforeStart != nil {
		f.beforeStart()
	}
	value, err := f.service.StartStage(ctx, actor, stage)
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return value, err
}

func (f *adventureGatewayFixture) Get(ctx context.Context, id string) (adventure.Adventure, error) {
	return f.service.Get(ctx, id)
}

func (f *adventureGatewayFixture) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	f.checkContext(ctx)
	if id != f.char.ID {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return f.char, nil
}

func (f *adventureGatewayFixture) Update(ctx context.Context, c corecharacter.Character) error {
	f.checkContext(ctx)
	f.char = c
	return nil
}

// The repository uses a separate view because character and adventure FindByID
// have different return types. Save records the actual service mutation.
type adventureGatewayRecords struct {
	adventure.Repository
	*adventureGatewayFixture
}

func (f adventureGatewayRecords) Save(ctx context.Context, a adventure.Adventure) error {
	f.checkContext(ctx)
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = a
	f.saves++
	return nil
}

func (f adventureGatewayRecords) FindByID(ctx context.Context, id string) (adventure.Adventure, error) {
	f.checkContext(ctx)
	if id != f.saved.ID {
		return adventure.Adventure{}, adventure.ErrNotFound
	}
	return f.saved, nil
}

type adventureGatewayBattle struct{}

func (adventureGatewayBattle) Resolve(corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{}, errors.New("unexpected solo battle")
}

func (adventureGatewayBattle) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	hp := make(map[string]int)
	for _, ally := range req.Allies {
		hp[ally.ID] = ally.HP
	}
	return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, RemainingHP: hp, Turns: 1, TotalReward: corebattle.Reward{Experience: 1, Currency: 1}}, nil
}

func (f *adventureGatewayFixture) router(timers playercontext.TimerReader) http.Handler {
	f.t.Helper()
	service, err := adventure.NewService(adventureGatewayRecords{adventureGatewayFixture: f}, f, adventureGatewayBattle{}, nil, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	f.service = service
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, f, &struct{ ShopService }{},
		WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)),
		func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func (f *adventureGatewayFixture) request(router http.Handler, params string) (int, map[string]json.RawMessage) {
	return gatewayRequest(f.t, router, "hero", "session", "application/json", `{"action":"adventure_start","params":`+params+`}`, f.expectedContext)
}

func TestAdventureGatewaySuccessAndRefreshFailure(t *testing.T) {
	for _, failure := range []string{"", "scheduled", "profile"} {
		t.Run(failure, func(t *testing.T) {
			f := newAdventureGatewayFixture(t)
			f.char.Level = 10
			f.expectedContext = context.WithValue(context.Background(), struct{}{}, "request")
			f.afterExecute = func() {
				if failure == "scheduled" {
					f.queryErr = errors.New("secret scheduled store")
				}
				if failure == "profile" {
					f.profileErr = errors.New("secret profile store")
				}
			}
			status, got := f.request(f.router(timer.NewService(nil)), `{"stage_id":"stage-01"}`)
			if status != 200 || f.executions != 1 || f.saves != 1 || string(got["success"]) != "true" {
				t.Fatalf("status=%d starts=%d saves=%d: %s", status, f.executions, f.saves, got)
			}
			var result adventureResponse
			if err := json.Unmarshal(got["result"], &result); err != nil {
				t.Fatal(err)
			}
			if result != toAdventureResponse(f.saved) || result.StageID != "stage-01" || !result.Resolved || !result.IsCleared || result.FloorsCleared != 10 || result.ExperienceReward == 0 {
				t.Fatalf("lost stage-start result: %+v", result)
			}
			// Stage start resolves immediately; it creates no scheduled expedition.
			actions, err := f.FindPendingByActorID(f.expectedContext, "hero")
			if err != nil && failure != "scheduled" {
				t.Fatal(err)
			}
			if len(actions) != 0 {
				t.Fatalf("unexpected scheduled actions: %+v", actions)
			}
			if failure != "" {
				if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
					t.Fatalf("lost refresh failure: %s", got)
				}
				return
			}
			assertGatewayContext(t, got)
			var observation PlayerContextResponse
			if err := json.Unmarshal(got["context"], &observation); err != nil {
				t.Fatal(err)
			}
			if observation.Character.Gold != f.char.Money || f.char.Money <= 100 || len(observation.OngoingActions) != 0 {
				t.Fatalf("stale observation: %+v", observation)
			}
		})
	}
}

func TestAdventureGatewayInvalidParamsAndOwnership(t *testing.T) {
	for _, params := range []string{`null`, `{}`, `{"stage_id":null}`, `{"stage_id":1}`, `{"stage_id":true}`, `{"stage_id":[]}`, `{"stage_id":"stage-00","character_id":"other"}`, `{"stage_id":"stage-00","player_id":"other"}`} {
		t.Run(params, func(t *testing.T) {
			f := newAdventureGatewayFixture(t)
			status, got := f.request(f.router(timer.NewService(nil)), params)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("status=%d starts=%d: %s", status, f.executions, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
	fMissing := newAdventureGatewayFixture(t)
	statusMissing, gotMissing := gatewayRequest(t, fMissing.router(timer.NewService(nil)), "hero", "session", "application/json", `{"action":"adventure_start"}`, nil)
	if statusMissing != 400 || fMissing.executions != 0 || fMissing.queryCalls != 0 {
		t.Fatalf("status=%d: %s", statusMissing, gotMissing)
	}
	assertGatewayError(t, gotMissing, "INVALID_ACTION_PARAMS")
	f := newAdventureGatewayFixture(t)
	status, got := gatewayRequest(t, f.router(timer.NewService(nil)), "other", "session", "application/json", `{"action":"adventure_start","params":{"stage_id":"stage-00"}}`, nil)
	if status != 403 || f.executions != 0 {
		t.Fatalf("status=%d: %s", status, got)
	}
}

func TestAdventureGatewayEmptyStageKeepsServiceDefault(t *testing.T) {
	f := newAdventureGatewayFixture(t)
	status, got := f.request(f.router(timer.NewService(nil)), `{"stage_id":""}`)
	if status != 200 || f.executions != 1 || f.saved.StageID != adventure.StarterAdventure {
		t.Fatalf("status=%d stage=%s: %s", status, f.saved.StageID, got)
	}
}

func TestAdventureGatewayGuardsAndReadFailures(t *testing.T) {
	for _, condition := range []string{"dead", "exhausted", "sleep", "wake", "pending", "overdue pending", "overdue processing", "shared sleep", "shared wake", "character read", "scheduled read", "sleep read", "wake read", "shared sleep read"} {
		t.Run(condition, func(t *testing.T) {
			f := newAdventureGatewayFixture(t)
			timers := timer.NewService(nil)
			reader := gatewayTimers{TimerReader: timers}
			want := 409
			switch condition {
			case "dead":
				f.char.Stats.HP = 0
			case "exhausted":
				f.char.Tired = 100
			case "sleep", "wake":
				category := timer.CategorySleep
				if condition == "wake" {
					category = timer.CategoryAsleep
				}
				if err := timers.SetLock(context.Background(), category, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			case "pending", "overdue pending", "overdue processing":
				state, deadline := scheduling.StatePending, time.Now().Add(time.Hour)
				if condition != "pending" {
					deadline = time.Now().Add(-time.Hour)
				}
				if condition == "overdue processing" {
					state = scheduling.StateProcessing
				}
				f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: state, ExecuteAt: deadline}}
			case "shared sleep":
				f.sleep.Sleeping = true
			case "shared wake":
				f.sleep.CanWake = true
			case "character read":
				f.characterErr = errors.New("secret character read")
			case "scheduled read":
				f.queryErr = errors.New("secret scheduled read")
			case "sleep read":
				reader.durationErr = errors.New("secret sleep read")
			case "wake read":
				reader.asleepErr = errors.New("secret wake read")
			case "shared sleep read":
				f.sleepErr = errors.New("secret shared sleep read")
			}
			if strings.HasSuffix(condition, "read") {
				want = 500
			}
			status, got := f.request(f.router(reader), `{"stage_id":"stage-00"}`)
			if status != want || f.executions != 0 || f.saves != 0 {
				t.Fatalf("status=%d starts=%d: %s", status, f.executions, got)
			}
			if want == 500 {
				assertGatewayError(t, got, "ACTION_PREFLIGHT_FAILED")
				if _, exists := got["context"]; exists {
					t.Fatal("failed preflight exposed context")
				}
			} else {
				assertGatewayError(t, got, "ACTION_UNAVAILABLE")
				assertGatewayContext(t, got)
				if len(f.actions) != 0 {
					var observation PlayerContextResponse
					if err := json.Unmarshal(got["context"], &observation); err != nil {
						t.Fatal(err)
					}
					if len(observation.OngoingActions) != 1 || observation.OngoingActions[0].IsReady != (condition != "pending") {
						t.Fatalf("lost unfinished action: %+v", observation)
					}
				}
			}
		})
	}
}

func TestAdventureGatewayServiceRejectionsAndUnknownFailure(t *testing.T) {
	for _, tc := range []struct {
		name, stage, code string
		status            int
		setup             func(*adventureGatewayFixture)
	}{
		{"stage", "missing-stage", "ADVENTURE_STAGE_NOT_FOUND", 422, nil},
		{"level", "stage-02", "ADVENTURE_LEVEL_REQUIRED", 403, nil},
		{"job level", "stage-02", "ADVENTURE_JOB_LEVEL_REQUIRED", 403, func(f *adventureGatewayFixture) { f.char.Level = 20 }},
		{"dead after preflight", "stage-00", "ADVENTURE_UNCONSCIOUS", 422, func(f *adventureGatewayFixture) { f.beforeStart = func() { f.char.Stats.HP = 0 } }},
		{"exhausted after preflight", "stage-00", "ADVENTURE_EXHAUSTED", 422, func(f *adventureGatewayFixture) { f.beforeStart = func() { f.char.Tired = 100 } }},
		{"daily entry", "stage-26", "ADVENTURE_DAILY_LIMIT", 422, func(f *adventureGatewayFixture) {
			f.char.Level, f.char.JobLevel = 99, 10
			f.beforeStart = func() {
				timers := timer.NewService(nil)
				if err := timers.SetLock(context.Background(), timer.CategoryDungeonOnce, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
				f.service.SetTimerService(timers)
			}
		}},
		{"storage", "stage-00", "EXECUTION_FAILED", 500, func(f *adventureGatewayFixture) { f.saveErr = errors.New("secret persistence failure") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAdventureGatewayFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			status, got := f.request(f.router(timer.NewService(nil)), fmt.Sprintf(`{"stage_id":%q}`, tc.stage))
			if status != tc.status || f.executions != 1 || f.saves != 0 {
				t.Fatalf("status=%d starts=%d saves=%d: %s", status, f.executions, f.saves, got)
			}
			assertGatewayError(t, got, tc.code)
			if tc.status == 500 {
				if _, exists := got["context"]; exists {
					t.Fatal("unknown outcome exposed context")
				}
				if strings.Contains(string(got["error"]), "secret") {
					t.Fatal("internal error leaked")
				}
			} else {
				assertGatewayContext(t, got)
			}
		})
	}
}

func TestAdventureGatewayWrappedErrorClassification(t *testing.T) {
	status, detail := adventureActionRejection(fmt.Errorf("wrapped: %w", adventure.ErrLevelRequirementNotMet))
	if status != 403 || detail.Code != "ADVENTURE_LEVEL_REQUIRED" {
		t.Fatalf("lost wrapped rejection: %d %+v", status, detail)
	}
	status, _ = adventureActionRejection(errors.New("unknown"))
	if status != 0 {
		t.Fatalf("unexpected error classified as rejection: %d", status)
	}
}
