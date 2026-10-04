package http

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type homeGatewayFixture struct {
	*gatewayFixture
	service       *home.Service
	timers        timer.Service
	target        string
	updates       int
	beforeExecute func()
	returnedSleep home.SleepResult
	returnedWake  home.WakeResult
}

func newHomeGatewayFixture(t *testing.T) *homeGatewayFixture {
	f := &homeGatewayFixture{gatewayFixture: newGatewayFixture(t), timers: timer.NewService(nil)}
	f.char.Name = "Hero"
	f.char.Stats = corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}
	f.char.Tired = 80
	f.expectedContext = context.WithValue(context.Background(), struct{}{}, "request")
	service, err := home.NewService(homeGatewayRepository{fixture: f}, f,
		home.WithTimer(f.timers), home.WithCharacterUpdater(f))
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	return f
}

func (f *homeGatewayFixture) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	f.checkContext(ctx)
	if id == "other" {
		return corecharacter.Character{ID: id, PlayerID: "other-owner"}, nil
	}
	if id != "hero" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return f.char, nil
}

func (f *homeGatewayFixture) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return f.FindByID(ctx, id)
}

func (f *homeGatewayFixture) Update(ctx context.Context, char corecharacter.Character) error {
	f.checkContext(ctx)
	if char.ID != "hero" {
		f.t.Fatal("updated destination instead of actor")
	}
	f.updates++
	f.char = char
	return nil
}

func (f *homeGatewayFixture) beginExecution(ctx context.Context, actor string) {
	f.checkContext(ctx)
	if actor != "hero" {
		f.t.Fatalf("wrong actor: %s", actor)
	}
	f.executions++
	if f.beforeExecute != nil {
		f.beforeExecute()
	}
}

func (f *homeGatewayFixture) Sleep(ctx context.Context, actor, target string) (home.SleepResult, error) {
	f.beginExecution(ctx, actor)
	f.target = target
	var err error
	if f.executionErr != nil {
		err = f.executionErr
	} else {
		f.returnedSleep, err = f.service.Sleep(ctx, actor, target)
	}
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.returnedSleep, err
}

func (f *homeGatewayFixture) Wake(ctx context.Context, actor string) (home.WakeResult, error) {
	f.beginExecution(ctx, actor)
	var err error
	if f.executionErr != nil {
		err = f.executionErr
	} else {
		f.returnedWake, err = f.service.Wake(ctx, actor)
	}
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.returnedWake, err
}

func (f *homeGatewayFixture) GetSleepStatus(ctx context.Context, actor string) (home.SleepStatus, error) {
	f.checkContext(ctx)
	f.sleepCalls++
	if f.sleepErr != nil {
		return home.SleepStatus{}, f.sleepErr
	}
	return f.service.GetSleepStatus(ctx, actor)
}

type homeGatewayRepository struct {
	home.Repository
	fixture *homeGatewayFixture
}

func (r homeGatewayRepository) GetHome(ctx context.Context, id string) (home.CharacterHome, error) {
	r.fixture.checkContext(ctx)
	if id != "other" {
		return home.CharacterHome{}, home.ErrHouseNotFound
	}
	expires := time.Now().Add(time.Hour)
	return home.CharacterHome{CharacterID: id, TownID: "town", ExpiresAt: &expires}, nil
}

func (f *homeGatewayFixture) router(timers playercontext.TimerReader, options ...Option) http.Handler {
	options = append([]Option{WithHome(f), WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers))}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func (f *homeGatewayFixture) request(router http.Handler, action, params string) (int, map[string]json.RawMessage) {
	return gatewayRequest(f.t, router, "hero", "session", "application/json", `{"action":"`+action+`"`+params+`}`, f.expectedContext)
}

func homeObservation(t *testing.T, got map[string]json.RawMessage) PlayerContextResponse {
	t.Helper()
	assertGatewayContext(t, got)
	var observation PlayerContextResponse
	if err := json.Unmarshal(got["context"], &observation); err != nil {
		t.Fatal(err)
	}
	return observation
}

func hasHomeAction(observation PlayerContextResponse, id string) bool {
	return slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == id })
}

func TestHomeGatewaySleepWakeLifecycle(t *testing.T) {
	f := newHomeGatewayFixture(t)
	router := f.router(f.timers, withActionCommand("bank_deposit", func(ctx context.Context, id string, p gatewayParams) (any, error) {
		return f.execute(ctx, id, p.Amount)
	}, gatewayRejection))
	status, got := f.request(router, "home_sleep", "")
	if status != 200 || f.executions != 1 || f.updates != 0 {
		t.Fatalf("sleep status=%d calls=%d: %s", status, f.executions, got)
	}
	var result sleepResponse
	if err := json.Unmarshal(got["result"], &result); err != nil {
		t.Fatal(err)
	}
	if !result.Sleeping || result.DurationSeconds != 60 || result.HomeCharacterID != "hero" || result.Message != f.returnedSleep.Message {
		t.Fatalf("lost service defaults/result: %+v", result)
	}
	observation := homeObservation(t, got)
	if !observation.Character.IsSleeping || len(observation.OngoingActions) != 1 || observation.OngoingActions[0].IsReady || hasHomeAction(observation, "home_wake") || hasHomeAction(observation, "adventure_start") {
		t.Fatalf("wrong active sleep context: %+v", observation)
	}
	for _, elapsed := range []bool{false, true} {
		if elapsed {
			// Deterministically expire only the duration; pending recovery remains.
			if err := f.timers.ReleaseLock(f.expectedContext, timer.CategorySleep, "hero"); err != nil {
				t.Fatal(err)
			}
		}
		before := f.executions
		for _, action := range []string{"bank_deposit", "adventure_start", "home_sleep"} {
			params := ""
			if action == "bank_deposit" {
				params = `,"params":{"amount":1}`
			}
			if action == "adventure_start" {
				params = `,"params":{"stage_id":"stage-00"}`
			}
			status, got = f.request(router, action, params)
			if status != 409 || f.executions != before || f.updates != 0 || f.char.Stats.HP != 10 || f.char.Stats.MP != 5 || f.char.Tired != 80 {
				t.Fatalf("recovered/executed before wake: %d %s", status, got)
			}
			assertGatewayError(t, got, "ACTION_UNAVAILABLE")
			observation = homeObservation(t, got)
			if !observation.Character.IsSleeping || len(observation.OngoingActions) != 1 || observation.OngoingActions[0].IsReady != elapsed || hasHomeAction(observation, "home_wake") != elapsed || hasHomeAction(observation, "adventure_start") {
				t.Fatalf("wrong recovery context: %+v", observation)
			}
		}
		if !elapsed {
			status, got = f.request(router, "home_wake", "")
			if status != 409 || f.executions != before+1 || f.updates != 0 || f.char.Stats.HP != 10 || f.char.Stats.MP != 5 || f.char.Tired != 80 {
				t.Fatalf("early wake recovered/bypassed service: %d %s", status, got)
			}
			assertGatewayError(t, got, "HOME_STILL_SLEEPING")
			homeObservation(t, got)
		}
	}
	status, got = f.request(router, "home_wake", "")
	if status != 200 || f.updates != 1 || f.char.Stats.HP != 100 || f.char.Stats.MP != 50 || f.char.Tired != 0 {
		t.Fatalf("wake did not recover: %d %s", status, got)
	}
	var wake wakeResponse
	if err := json.Unmarshal(got["result"], &wake); err != nil {
		t.Fatal(err)
	}
	if !wake.Success || wake.Character != toCharacterResponse(f.char) || wake.Message != f.returnedWake.Message {
		t.Fatalf("lost wake result: %+v", wake)
	}
	observation = homeObservation(t, got)
	if observation.Character.IsSleeping || len(observation.OngoingActions) != 0 || hasHomeAction(observation, "home_wake") || !hasHomeAction(observation, "adventure_start") || observation.Character.HP != 100 || observation.Character.MP != 50 || observation.Character.Tired != 0 {
		t.Fatalf("stale wake context: %+v", observation)
	}
	locked, err := f.timers.IsLocked(f.expectedContext, timer.CategoryAsleep, "hero")
	if err != nil || locked {
		t.Fatalf("wake recovery not cleared: %t %v", locked, err)
	}
}

func TestHomeGatewaySleepDefaultsTargetsAndVitality(t *testing.T) {
	for _, state := range []string{"awake", "dead", "exhausted"} {
		for _, params := range []string{"", `,"params":{}`, `,"params":{"target_home_id":""}`, `,"params":{"target_home_id":"hero"}`, `,"params":{"target_home_id":"other"}`} {
			t.Run(state+params, func(t *testing.T) {
				f := newHomeGatewayFixture(t)
				if state == "dead" {
					f.char.Stats.HP = 0
				}
				if state == "exhausted" {
					f.char.Tired = 100
				}
				before := f.char
				status, got := f.request(f.router(f.timers), "home_sleep", params)
				if status != 200 || f.executions != 1 || f.updates != 0 || f.char.Money != before.Money || f.char.Stats != before.Stats || f.char.Tired != before.Tired {
					t.Fatalf("sleep changed vitality/charged/blocked: %d %s", status, got)
				}
				expectedTarget := "hero"
				if params == `,"params":{"target_home_id":"other"}` {
					expectedTarget = "other"
				}
				if f.returnedSleep.HomeCharacterID != expectedTarget || (expectedTarget == "other" && f.target != "other") {
					t.Fatalf("wrong target: %+v", f.returnedSleep)
				}
				locked, err := f.timers.IsLocked(f.expectedContext, timer.CategorySleep, expectedTarget)
				if err != nil || locked != (expectedTarget == "hero") {
					t.Fatalf("sleep timer assigned to destination: %t %v", locked, err)
				}
				homeObservation(t, got)
			})
		}
	}
}

func TestHomeGatewayMissingTarget(t *testing.T) {
	f := newHomeGatewayFixture(t)
	status, got := f.request(f.router(f.timers), "home_sleep", `,"params":{"target_home_id":"missing"}`)
	if status != 404 || f.executions != 1 || f.updates != 0 {
		t.Fatalf("status=%d: %s", status, got)
	}
	assertGatewayError(t, got, "CHARACTER_NOT_FOUND")
	homeObservation(t, got)
}

func TestHomeGatewayAwakeWakeRemainsUnavailable(t *testing.T) {
	f := newHomeGatewayFixture(t)
	status, got := f.request(f.router(f.timers), "home_wake", "")
	if status != 409 || f.executions != 0 {
		t.Fatalf("status=%d: %s", status, got)
	}
	assertGatewayError(t, got, "ACTION_UNAVAILABLE")
}
