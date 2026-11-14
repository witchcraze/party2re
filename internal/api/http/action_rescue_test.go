package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/rescue"
)

type rescueGatewayFixture struct {
	*gatewayFixture
	RescueService
	rescue.CharacterRepository
	service                    *rescue.Service
	saved, returned            rescue.RescueRecord
	saves, clears              int
	saveErr, clearErr, findErr error
	remaining                  []string
	activityErr                error
	before, after              time.Time
}

func (f *rescueGatewayFixture) EmergencyRescue(ctx context.Context, actor, reason string, now time.Time) (rescue.RescueRecord, error) {
	f.checkContext(ctx)
	if actor != "hero" || now.Before(f.before) || now.Location() != time.UTC {
		f.t.Fatalf("wrong actor/time: %s %s", actor, now)
	}
	f.executions++
	var err error
	if f.executionErr != nil {
		err = f.executionErr
	} else {
		f.returned, err = f.service.EmergencyRescue(ctx, actor, reason, now)
	}
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.returned, err
}

func (f *rescueGatewayFixture) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	f.checkContext(ctx)
	if f.findErr != nil {
		return corecharacter.Character{}, f.findErr
	}
	return f.gatewayFixture.FindByID(ctx, id)
}

func (f *rescueGatewayFixture) Save(ctx context.Context, rec rescue.RescueRecord) error {
	f.checkContext(ctx)
	f.saves++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = rec
	return nil
}

func (f *rescueGatewayFixture) FindLatestByCharacterID(context.Context, string) (rescue.RescueRecord, error) {
	return rescue.RescueRecord{}, rescue.ErrNoRescueRecord
}

func (f *rescueGatewayFixture) MarkApplied(context.Context, string, time.Time) error { return nil }

func (f *rescueGatewayFixture) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return f.FindByID(ctx, id)
}

func (f *rescueGatewayFixture) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// visibleRescue drops the internal deadline fields that the JSON result omits.
func visibleRescue(r rescue.RescueRecord) rescue.RescueRecord {
	r.SleepUntil, r.AppliedAt = time.Time{}, time.Time{}
	return r
}

func (f *rescueGatewayFixture) ClearActiveActions(ctx context.Context, actor string) (bool, error) {
	f.checkContext(ctx)
	if actor != "hero" {
		f.t.Fatal("wrong cleanup actor")
	}
	f.clears++
	if f.clearErr != nil {
		return false, f.clearErr
	}
	cleared := len(f.actions) > 0
	f.actions = nil
	return cleared, nil
}

func (f *rescueGatewayFixture) ActivityKinds(ctx context.Context, owner, actor string) ([]string, error) {
	f.checkContext(ctx)
	if actor != "hero" {
		f.t.Fatal("wrong activity actor")
	}
	return f.remaining, f.activityErr
}

func (f *rescueGatewayFixture) router(timers timer.Service) http.Handler {
	f.service = rescue.NewService(f, f, f, timers, f)
	f.service.SetActivityReader(f)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithRescue(f), WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers)),
		func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func (f *rescueGatewayFixture) request(router http.Handler, params string) (int, map[string]json.RawMessage) {
	f.before = time.Now().UTC()
	status, got := gatewayRequest(f.t, router, "hero", "session", "application/json", `{"action":"rescue_request"`+params+`}`, f.expectedContext)
	f.after = time.Now().UTC()
	return status, got
}

func TestRescueGatewaySuccessRecoveryAndRefresh(t *testing.T) {
	for _, state := range []string{"idle", "sleep", "cooldown", "pending", "processing", "sleeping work"} {
		for _, refresh := range []string{"", "scheduled", "profile"} {
			t.Run(state+"/"+refresh, func(t *testing.T) {
				f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "request")
				timers := timer.NewService(nil)
				if state == "sleep" || state == "cooldown" || state == "sleeping work" {
					if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
					if state != "cooldown" {
						if err := timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
							t.Fatal(err)
						}
					}
				}
				if state == "pending" || state == "processing" || state == "sleeping work" {
					workState := scheduling.StatePending
					if state == "processing" {
						workState = scheduling.StateProcessing
					}
					f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: workState, ExecuteAt: time.Now().Add(-time.Hour)}}
				}
				f.sleepErr = errors.New("ordinary sleep guard must be bypassed")
				f.afterExecute = func() {
					if refresh == "scheduled" {
						f.queryErr = errors.New("secret scheduled store")
					}
					if refresh == "profile" {
						f.profileErr = errors.New("secret profile store")
					}
				}
				hasWork := state == "pending" || state == "processing" || state == "sleeping work"
				wantClears := 0
				if hasWork {
					wantClears = 1
				}
				status, got := f.request(f.router(timers), `,"params":{"reason":" stuck "}`)
				if status != 200 || f.executions != 1 || f.clears != wantClears || f.sleepCalls != 0 || string(got["success"]) != "true" {
					t.Fatalf("status=%d calls=%d clears=%d: %s", status, f.executions, f.clears, got)
				}
				var result rescue.RescueRecord
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if result != visibleRescue(f.returned) || result.Reason != "stuck" || result.CreatedAt.Before(f.before) || result.CreatedAt.After(f.after) {
					t.Fatalf("lost service result: %+v", result)
				}
				wantSaves, wantPenalty := 0, 0
				if hasWork {
					wantSaves, wantPenalty = 1, rescue.DefaultPenaltySeconds
				}
				if f.saves != wantSaves || result.PenaltySeconds != wantPenalty || (wantSaves == 1 && result != visibleRescue(f.saved)) {
					t.Fatalf("wrong rescue result/saves: %+v / %d", result, f.saves)
				}
				if refresh != "" {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost result on refresh failure: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
					var observation PlayerContextResponse
					if err := json.Unmarshal(got["context"], &observation); err != nil {
						t.Fatal(err)
					}
					wantTimers := 1
					if state == "idle" {
						wantTimers = 0
					}
					if len(observation.OngoingActions) != wantTimers {
						t.Fatalf("wrong refreshed work/timers: %+v", observation)
					}
					for _, ongoing := range observation.OngoingActions {
						if ongoing.ID != "sleep" {
							t.Fatalf("cleared work remains in context: %+v", ongoing)
						}
						if state == "sleeping work" && ongoing.RemainingSeconds < 4000 {
							t.Fatalf("existing sleep penalty was not preserved: %+v", ongoing)
						}
					}
					for _, action := range observation.AvailableActions {
						if action.Action == "rescue_request" && (len(action.RequiredParams) != 1 || action.RequiredParams[0] != "reason") {
							t.Fatalf("wrong client-required inputs: %+v", action)
						}
					}
				}
			})
		}
	}
}

func TestRescueGatewayParamsAndOwnership(t *testing.T) {
	for _, params := range []string{"", `,"params":null`, `,"params":{}`, `,"params":{"reason":null}`, `,"params":{"reason":1}`, `,"params":{"reason":true}`, `,"params":{"reason":[]}`, `,"params":{"reason":"stuck","character_id":"other"}`, `,"params":{"reason":"stuck","player_id":"other"}`, `,"params":{"reason":"stuck","unknown":1}`} {
		t.Run(params, func(t *testing.T) {
			f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
			status, got := f.request(f.router(timer.NewService(nil)), params)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 || f.clears != 0 {
				t.Fatalf("status=%d executions=%d: %s", status, f.executions, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
	for _, token := range []string{"", "session"} {
		f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		status, got := gatewayRequest(t, f.router(timer.NewService(nil)), "other", token, "application/json", `{"action":"rescue_request","params":{"reason":"stuck"}}`, nil)
		want := 403
		if token == "" {
			want = 401
		}
		if status != want || f.executions != 0 || f.queryCalls != 0 {
			t.Fatalf("status=%d: %s", status, got)
		}
	}
}

func TestRescueGatewayRejectionsAndUnknownFailures(t *testing.T) {
	for _, tc := range []struct {
		name, reason, code string
		status             int
		setup              func(*rescueGatewayFixture)
	}{
		{"empty reason", "", "RESCUE_INVALID_REASON", 422, nil},
		{"blank reason", "  ", "RESCUE_INVALID_REASON", 422, nil},
		{"invalid actor", "stuck", "RESCUE_INVALID_CHARACTER_ID", 400, func(f *rescueGatewayFixture) {
			f.executionErr = fmt.Errorf("wrapped: %w", rescue.ErrInvalidCharacterID)
		}},
		{"missing character", "stuck", "CHARACTER_NOT_FOUND", 404, func(f *rescueGatewayFixture) { f.findErr = fmt.Errorf("wrapped: %w", corecharacter.ErrNotFound) }},
		{"cleanup failure", "stuck", "EXECUTION_FAILED", 500, func(f *rescueGatewayFixture) {
			f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StatePending}}
			f.clearErr = errors.New("secret cleanup store")
		}},
		{"feature activity remains", "stuck", "RESCUE_ACTIVITY_REMAINING", 409, func(f *rescueGatewayFixture) {
			f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StatePending}}
			f.remaining = []string{"dungeon"}
		}},
		{"activity read failure", "stuck", "EXECUTION_FAILED", 500, func(f *rescueGatewayFixture) { f.activityErr = errors.New("secret activity store") }},
		{"save failure", "stuck", "EXECUTION_FAILED", 500, func(f *rescueGatewayFixture) {
			f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StatePending}}
			f.saveErr = errors.New("secret rescue store")
		}},
	} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/refresh=%t", tc.name, refreshFails), func(t *testing.T) {
				f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
				if tc.setup != nil {
					tc.setup(f)
				}
				if refreshFails {
					f.afterExecute = func() { f.queryErr = errors.New("secret refresh store") }
				}
				status, got := f.request(f.router(timer.NewService(nil)), fmt.Sprintf(`,"params":{"reason":%q}`, tc.reason))
				if status != tc.status || f.executions != 1 || strings.Contains(string(got["error"]), "secret") {
					t.Fatalf("status=%d executions=%d: %s", status, f.executions, got)
				}
				assertGatewayError(t, got, tc.code)
				if (f.remaining != nil || f.activityErr != nil) && (f.clears != 0 || f.saves != 0) {
					t.Fatalf("unrecovered activity changed state: clears=%d saves=%d", f.clears, f.saves)
				}
				if status == 500 {
					if _, exists := got["context"]; exists || f.profileCalls != 0 {
						t.Fatal("unknown outcome refreshed/exposed context")
					}
				} else if refreshFails {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost rejection on refresh failure: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestRescueContextSleepWindow(t *testing.T) {
	getContext := func(t *testing.T, router http.Handler, expectedCtx context.Context) PlayerContextResponse {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(expectedCtx)
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("GET context status=%d body=%s", w.Code, w.Body.String())
		}
		var resp PlayerContextResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}

	t.Run("awake rescued character exposes sleep window and remaining time", func(t *testing.T) {
		f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		f.expectedContext = context.WithValue(context.Background(), struct{}{}, "rescue-context")
		f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}}
		timers := timer.NewService(nil)
		router := f.router(timers)

		status, _ := f.request(router, `,"params":{"reason":"stuck"}`)
		if status != http.StatusOK {
			t.Fatalf("rescue_request status=%d", status)
		}

		ctxResp := getContext(t, router, f.expectedContext)
		if !ctxResp.Character.IsSleeping || ctxResp.Scene.LocationID != "sleep" {
			t.Fatalf("rescued context did not enter sleep: char=%+v scene=%+v", ctxResp.Character, ctxResp.Scene)
		}
		if len(ctxResp.OngoingActions) != 1 {
			t.Fatalf("expected 1 sleep ongoing action, got %+v", ctxResp.OngoingActions)
		}
		sleep := ctxResp.OngoingActions[0]
		if sleep.ID != "sleep" || sleep.ActionType != "home_sleep" || sleep.IsReady {
			t.Fatalf("unexpected sleep entry: %+v", sleep)
		}
		if sleep.RemainingSeconds < int64(rescue.DefaultPenaltySeconds-5) || sleep.RemainingSeconds > int64(rescue.DefaultPenaltySeconds) {
			t.Fatalf("remaining_seconds=%d, want ~%d", sleep.RemainingSeconds, rescue.DefaultPenaltySeconds)
		}
		if sleep.ExecuteAt.Before(f.before.Add((rescue.DefaultPenaltySeconds-5)*time.Second)) || sleep.ExecuteAt.After(f.after.Add((rescue.DefaultPenaltySeconds+5)*time.Second)) {
			t.Fatalf("execute_at=%s outside expected penalty window [%s..%s]+600s", sleep.ExecuteAt, f.before, f.after)
		}
		var actions []string
		for _, a := range ctxResp.AvailableActions {
			actions = append(actions, a.Action)
		}
		if strings.Contains(strings.Join(actions, ","), "home_wake") {
			t.Fatalf("awake rescue penalty must not require home_wake: %v", actions)
		}

		if err := timers.ReleaseLock(f.expectedContext, timer.CategorySleep, "hero"); err != nil {
			t.Fatal(err)
		}
		afterExpiry := getContext(t, router, f.expectedContext)
		if afterExpiry.Character.IsSleeping || len(afterExpiry.OngoingActions) != 0 {
			t.Fatalf("sleep remained after penalty expiry: char=%+v ongoing=%+v", afterExpiry.Character, afterExpiry.OngoingActions)
		}
	})

	t.Run("sleeping rescued character preserves and extends sleep window", func(t *testing.T) {
		f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		f.expectedContext = context.WithValue(context.Background(), struct{}{}, "sleeping-rescue-context")
		f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}}
		timers := timer.NewService(nil)
		for _, cat := range []string{timer.CategorySleep, timer.CategoryAsleep} {
			if err := timers.SetLock(f.expectedContext, cat, "hero", time.Hour); err != nil {
				t.Fatal(err)
			}
		}
		router := f.router(timers)

		status, _ := f.request(router, `,"params":{"reason":"stuck"}`)
		if status != http.StatusOK {
			t.Fatalf("rescue_request status=%d", status)
		}

		ctxResp := getContext(t, router, f.expectedContext)
		if !ctxResp.Character.IsSleeping || len(ctxResp.OngoingActions) != 1 {
			t.Fatalf("expected sleeping context with 1 action, got %+v", ctxResp)
		}
		sleep := ctxResp.OngoingActions[0]
		if sleep.ID != "sleep" || sleep.IsReady || sleep.RemainingSeconds < 4190 || sleep.RemainingSeconds > 4205 {
			t.Fatalf("remaining_seconds=%d, want ~4200s (1h + 600s)", sleep.RemainingSeconds)
		}
	})

	t.Run("idle rescued character adds no sleep window", func(t *testing.T) {
		f := &rescueGatewayFixture{gatewayFixture: newGatewayFixture(t)}
		f.expectedContext = context.WithValue(context.Background(), struct{}{}, "idle-rescue-context")
		router := f.router(timer.NewService(nil))

		status, _ := f.request(router, `,"params":{"reason":"accidental"}`)
		if status != http.StatusOK {
			t.Fatalf("rescue_request status=%d", status)
		}

		ctxResp := getContext(t, router, f.expectedContext)
		if ctxResp.Character.IsSleeping || len(ctxResp.OngoingActions) != 0 {
			t.Fatalf("idle rescue created sleep observation: %+v", ctxResp)
		}
	})
}
