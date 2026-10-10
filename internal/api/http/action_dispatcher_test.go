package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type gatewayFixture struct {
	CharacterService
	PlayerService
	HomeService
	char                                             corecharacter.Character
	queryCalls, profileCalls, executions, sleepCalls int
	characterCalls                                   int
	queryErr, profileErr, executionErr, sleepErr     error
	queryOwner                                       string
	profileOwner                                     string
	characterErr                                     error
	actions                                          []scheduling.ScheduledAction
	sleep                                            home.SleepStatus
	afterExecute                                     func()
	expectedContext                                  context.Context
	t                                                *testing.T
}

func newGatewayFixture(t *testing.T) *gatewayFixture {
	return &gatewayFixture{t: t, char: corecharacter.Character{ID: "hero", PlayerID: "owner", Money: 100, Stats: corecharacter.Stats{HP: 50}}}
}

func (f *gatewayFixture) checkContext(ctx context.Context) {
	f.t.Helper()
	if f.expectedContext != nil && ctx != f.expectedContext {
		f.t.Fatal("request context was not propagated")
	}
}

func (f *gatewayFixture) Authenticate(ctx context.Context, token string) (coreplayer.Player, error) {
	f.checkContext(ctx)
	if token != "session" {
		return coreplayer.Player{}, errors.New("invalid session")
	}
	return coreplayer.Player{ID: "owner"}, nil
}

func (f *gatewayFixture) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	f.checkContext(ctx)
	f.characterCalls++
	if id == "missing" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	if id == "other" {
		return corecharacter.Character{ID: id, PlayerID: "other"}, nil
	}
	return f.char, nil
}

func (f *gatewayFixture) GetProfile(ctx context.Context, id string) (character.ProfileView, error) {
	f.checkContext(ctx)
	f.profileCalls++
	c := f.char
	if f.profileOwner != "" {
		c.PlayerID = f.profileOwner
	}
	return character.ProfileView{Character: c, Profile: character.Profile{AvatarURL: "avatar"}}, f.profileErr
}

func (f *gatewayFixture) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	f.checkContext(ctx)
	f.queryCalls++
	c := f.char
	if f.queryOwner != "" {
		c.PlayerID = f.queryOwner
	}
	if f.characterErr != nil {
		return corecharacter.Character{}, f.characterErr
	}
	return c, ctx.Err()
}

func (f *gatewayFixture) FindPendingByActorID(ctx context.Context, id string) ([]scheduling.ScheduledAction, error) {
	f.checkContext(ctx)
	if id != "hero" {
		f.t.Fatal("wrong query actor")
	}
	return f.actions, f.queryErr
}

func (f *gatewayFixture) GetSleepStatus(ctx context.Context, id string) (home.SleepStatus, error) {
	f.checkContext(ctx)
	f.sleepCalls++
	return f.sleep, f.sleepErr
}

type gatewayParams struct {
	Amount int `json:"amount"`
}

var gatewayRejected = errors.New("domain rejection")

func (f *gatewayFixture) execute(ctx context.Context, id string, amount int) (any, error) {
	f.checkContext(ctx)
	if id != "hero" {
		f.t.Fatal("wrong execution actor")
	}
	f.executions++
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return map[string]any{"amount": amount, "receipt": "kept"}, f.executionErr
}

func gatewayRejection(err error) (int, ErrorDetail) {
	if errors.Is(err, gatewayRejected) {
		return 422, ErrorDetail{Code: "DOMAIN_REJECTED", Message: "rejected"}
	}
	return 0, ErrorDetail{}
}

func (f *gatewayFixture) router(action string, timers playercontext.TimerReader, options ...Option) http.Handler {
	var command Option
	if action == "bank_deposit" {
		command = withActionCommand(action, func(ctx context.Context, id string, p gatewayParams) (any, error) {
			return f.execute(ctx, id, p.Amount)
		}, gatewayRejection)
	} else if action == "rescue_request" {
		command = withActionCommand(action, func(ctx context.Context, id string, p rescueActionParams) (any, error) { return f.execute(ctx, id, 0) }, gatewayRejection)
	} else {
		command = withActionCommand(action, func(ctx context.Context, id string, p struct{}) (any, error) { return f.execute(ctx, id, 0) }, gatewayRejection)
	}
	opts := []Option{WithPlayerContext(playercontext.NewService(f, f, gatewayTimers{TimerReader: timers, check: f.checkContext})), command, func(h *Handler) { h.homes = f }}
	opts = append(opts, options...)
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		f.t.Fatal(err)
	}
	return h.Router()
}

func gatewayRequest(t *testing.T, router http.Handler, path, token, contentType, body string, ctx context.Context) (int, map[string]json.RawMessage) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+path+"/actions", strings.NewReader(body))
	if ctx != nil {
		r = r.WithContext(ctx)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var result map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("status=%d body=%s: %v", w.Code, w.Body.String(), err)
	}
	return w.Code, result
}

func assertGatewayError(t *testing.T, got map[string]json.RawMessage, code string) {
	t.Helper()
	var detail ErrorDetail
	if err := json.Unmarshal(got["error"], &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Code != code || string(got["success"]) != "false" {
		t.Fatalf("want error %s: %s", code, got)
	}
}

func assertGatewayContext(t *testing.T, got map[string]json.RawMessage) {
	t.Helper()
	var c map[string]json.RawMessage
	if err := json.Unmarshal(got["context"], &c); err != nil {
		t.Fatal(err)
	}
	if len(c) != 4 || string(c["ongoing_actions"]) == "null" || string(c["available_actions"]) == "null" {
		t.Fatalf("invalid shared context: %s", got)
	}
	var response PlayerContextResponse
	if err := json.Unmarshal(got["context"], &response); err != nil {
		t.Fatal(err)
	}
	if response.Character.ID != "hero" || response.Character.IconURL != "avatar" || response.Scene.LocationID == "" {
		t.Fatalf("incomplete composition: %+v", response)
	}
	for _, a := range response.AvailableActions {
		if a.RequiredParams == nil {
			t.Fatal("null required_params")
		}
	}
}

func TestActionGatewayAuthenticationAndTransport(t *testing.T) {
	for _, tc := range []struct {
		name, path, token, ct, body string
		status                      int
	}{
		{"no session", "hero", "", "application/json", `{}`, 401},
		{"invalid session", "hero", "bad", "application/json", `{}`, 401},
		{"wrong owner", "other", "session", "application/json", `{}`, 403},
		{"missing character", "missing", "session", "application/json", `{}`, 404},
		{"content type", "hero", "session", "text/plain", `{}`, 415},
		{"malformed", "hero", "session", "application/json", `{`, 400},
		{"unknown envelope field", "hero", "session", "application/json", `{"action":"bank_deposit","actor":"other"}`, 400},
		{"two documents", "hero", "session", "application/json", `{} {}`, 400},
		{"null envelope", "hero", "session", "application/json", `null`, 400},
		{"missing action", "hero", "session", "application/json", `{}`, 400},
		{"null action", "hero", "session", "application/json", `{"action":null}`, 400},
		{"wrong action type", "hero", "session", "application/json", `{"action":123}`, 400},
		{"body too large", "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":1}}` + strings.Repeat(" ", 64*1024), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			status, got := gatewayRequest(t, f.router("bank_deposit", timer.NewService(nil)), tc.path, tc.token, tc.ct, tc.body, nil)
			if status != tc.status || f.executions != 0 || f.queryCalls != 0 || f.profileCalls != 0 {
				t.Fatalf("status=%d calls=%+v response=%s", status, f, got)
			}
			if _, ok := got["context"]; ok {
				t.Fatal("context exposed on transport rejection")
			}
		})
	}
}

func TestActionGatewayStrictParams(t *testing.T) {
	for _, params := range []string{"", `null`, `[]`, `5`, `"x"`, `{}`, `{"amount":null}`, `{"amount":"1"}`, `{"amount":1.5}`, `{"amount":true}`, `{"amount":1,"unknown":2}`, `{"amount":1,"character_id":"other"}`, `{"amount":1,"player_id":"other"}`} {
		t.Run(params, func(t *testing.T) {
			f := newGatewayFixture(t)
			body := `{"action":"bank_deposit"`
			if params != "" {
				body += `,"params":` + params
			}
			body += `}`
			status, got := gatewayRequest(t, f.router("bank_deposit", timer.NewService(nil)), "hero", "session", "application/json", body, nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("status=%d response=%s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			if _, ok := got["context"]; ok {
				t.Fatal("context exposed for invalid params")
			}
		})
	}
}

func TestActionGatewayOutcomeAndRefresh(t *testing.T) {
	for _, rejection := range []bool{false, true} {
		for _, failure := range []string{"", "query", "profile", "owner", "profile owner", "character", "sleep timer", "wake timer"} {
			t.Run(failure+map[bool]string{false: " success", true: " rejection"}[rejection], func(t *testing.T) {
				f := newGatewayFixture(t)
				if rejection {
					f.executionErr = gatewayRejected
				}
				timers := &gatewayTimers{TimerReader: timer.NewService(nil)}
				f.afterExecute = func() {
					// A visible test mutation verifies refresh observes the new state;
					// the receipt and execution count must survive refresh failures.
					if !rejection {
						f.char.Money -= 10
					}
					switch failure {
					case "query":
						f.queryErr = errors.New("secret store failure")
					case "profile":
						f.profileErr = errors.New("secret profile failure")
					case "owner":
						f.queryOwner = "other"
					case "profile owner":
						f.profileOwner = "other"
					case "character":
						f.characterErr = errors.New("secret character failure")
					case "sleep timer":
						timers.durationErr = errors.New("secret timer failure")
					case "wake timer":
						timers.asleepErr = errors.New("secret timer failure")
					}
				}
				status, got := gatewayRequest(t, f.router("bank_deposit", timers), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":10}}`, nil)
				want := 200
				if rejection {
					want = 422
				}
				if status != want || f.executions != 1 || f.queryCalls != 2 {
					t.Fatalf("status=%d calls=%+v response=%s", status, f, got)
				}
				if rejection {
					assertGatewayError(t, got, "DOMAIN_REJECTED")
				} else if string(got["success"]) != "true" || !strings.Contains(string(got["result"]), `"receipt":"kept"`) {
					t.Fatalf("lost success: %s", got)
				}
				if failure == "" {
					assertGatewayContext(t, got)
					var observation PlayerContextResponse
					if err := json.Unmarshal(got["context"], &observation); err != nil {
						t.Fatal(err)
					}
					if observation.Character.Gold != f.char.Money {
						t.Fatal("refresh reused stale preflight facts")
					}
					if _, ok := got["context_error"]; ok {
						t.Fatal("unexpected refresh error")
					}
				} else {
					var detail ErrorDetail
					if err := json.Unmarshal(got["context_error"], &detail); err != nil {
						t.Fatal(err)
					}
					if string(got["context"]) != "null" || detail.Code != "CONTEXT_REFRESH_FAILED" || strings.Contains(detail.Message, "secret") {
						t.Fatalf("partial/leaked refresh: %s", got)
					}
				}
			})
		}
	}
}

func TestActionGatewayUnexpectedExecutionFailure(t *testing.T) {
	f := newGatewayFixture(t)
	f.executionErr = errors.New("secret ambiguous mutation")
	status, got := gatewayRequest(t, f.router("bank_deposit", timer.NewService(nil)), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":1}}`, nil)
	if status != 500 || f.executions != 1 || f.queryCalls != 1 || f.profileCalls != 0 {
		t.Fatalf("status=%d response=%s", status, got)
	}
	assertGatewayError(t, got, "EXECUTION_FAILED")
	if _, ok := got["context"]; ok {
		t.Fatal("refresh after unknown outcome")
	}
	if strings.Contains(string(got["error"]), "secret") {
		t.Fatal("internal execution error leaked")
	}
}

func TestActionGatewayRequestContext(t *testing.T) {
	f := newGatewayFixture(t)
	f.expectedContext = context.WithValue(context.Background(), struct{}{}, "request")
	status, got := gatewayRequest(t, f.router("bank_deposit", timer.NewService(nil)), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":1}}`, f.expectedContext)
	if status != 200 || f.executions != 1 || f.sleepCalls != 1 {
		t.Fatalf("status=%d response=%s", status, got)
	}
}

func TestActionGatewayUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name, action, code string
		status             int
		option             Option
	}{
		{"unknown", "nonexistent", "ACTION_NOT_FOUND", 404, nil},
		{"unconnected", "shop_purchase", "ACTION_NOT_IMPLEMENTED", 501, nil},
		{"unconfigured query", "bank_deposit", "ACTION_NOT_IMPLEMENTED", 501, WithPlayerContext(nil)},
		{"unconfigured adapter", "bank_withdraw", "ACTION_NOT_IMPLEMENTED", 501, withActionCommand[gatewayParams]("bank_withdraw", nil, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			status, got := gatewayRequest(t, f.router("bank_deposit", timer.NewService(nil), tc.option), "hero", "session", "application/json", `{"action":"`+tc.action+`","params":{"amount":1}}`, nil)
			if status != tc.status || f.executions != 0 || f.queryCalls != 0 || f.profileCalls != 0 {
				t.Fatalf("status=%d response=%s", status, got)
			}
			assertGatewayError(t, got, tc.code)
			if _, ok := got["context"]; ok {
				t.Fatal("unsupported action exposed context")
			}
		})
	}
	// This ticket does not connect any fake or real commands by default.
	f := newGatewayFixture(t)
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(playercontext.NewService(f, f, timer.NewService(nil))))
	if err != nil {
		t.Fatal(err)
	}
	status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":1}}`, nil)
	if status != 501 || f.queryCalls != 0 {
		t.Fatalf("production command unexpectedly connected: %d %s", status, got)
	}
	assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
}

func TestActionGatewayPreflightFailure(t *testing.T) {
	for _, failure := range []string{"character", "scheduled", "sleep timer", "wake timer", "shared sleep guard", "owner", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			f := newGatewayFixture(t)
			timers := gatewayTimers{TimerReader: timer.NewService(nil)}
			ctx := context.Background()
			want := 500
			switch failure {
			case "character":
				f.characterErr = errors.New("secret character store")
			case "scheduled":
				f.queryErr = errors.New("secret scheduled store")
			case "sleep timer":
				timers.durationErr = errors.New("secret sleep store")
			case "wake timer":
				timers.asleepErr = errors.New("secret wake store")
			case "shared sleep guard":
				f.sleepErr = errors.New("secret sleep status")
			case "owner":
				f.queryOwner = "other"
				want = 403
			case "cancellation":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			status, got := gatewayRequest(t, f.router("bank_deposit", timers), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":1}}`, ctx)
			if status != want || f.executions != 0 || f.queryCalls != 1 || f.profileCalls != 0 {
				t.Fatalf("status=%d response=%s", status, got)
			}
			if want == 500 {
				assertGatewayError(t, got, "ACTION_PREFLIGHT_FAILED")
			}
			if _, ok := got["context"]; ok {
				t.Fatal("failed preflight exposed context")
			}
		})
	}
}

func TestActionGatewayEntryExclusionAndSharedGuard(t *testing.T) {
	for _, condition := range []string{"no gold", "pending", "overdue processing", "sleeping", "pending wake", "shared sleeping", "shared pending wake"} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(condition+map[bool]string{false: " refresh", true: " failed refresh"}[refreshFails], func(t *testing.T) {
				f := newGatewayFixture(t)
				timers := timer.NewService(nil)
				switch condition {
				case "no gold":
					f.char.Money = 0
				case "pending":
					f.actions = []scheduling.ScheduledAction{{ID: "work", State: scheduling.StatePending, ExecuteAt: time.Now().Add(time.Hour)}}
				case "overdue processing":
					f.actions = []scheduling.ScheduledAction{{ID: "work", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(-time.Hour)}}
				case "sleeping":
					if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
				case "pending wake":
					if err := timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
				case "shared sleeping":
					f.sleep.Sleeping = true
				case "shared pending wake":
					f.sleep.CanWake = true
				}
				if refreshFails {
					f.profileErr = errors.New("profile store failed")
				}
				status, got := gatewayRequest(t, f.router("bank_deposit", timers), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":1}}`, nil)
				if status != 409 || f.executions != 0 || f.queryCalls != 2 {
					t.Fatalf("status=%d response=%s", status, got)
				}
				assertGatewayError(t, got, "ACTION_UNAVAILABLE")
				if refreshFails {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost precondition rejection: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestActionGatewayNoInputAndRecoveryExceptions(t *testing.T) {
	for _, action := range []string{"home_sleep", "home_wake", "rescue_request"} {
		for _, params := range []string{"", `,"params":{}`} {
			t.Run(action+params, func(t *testing.T) {
				f := newGatewayFixture(t)
				timers := timer.NewService(nil)
				if action == "home_wake" {
					if err := timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
				}
				if action == "rescue_request" {
					params = `,"params":{"reason":"stuck"}`
					if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
					f.actions = []scheduling.ScheduledAction{{ID: "work", State: scheduling.StatePending}}
				}
				if action != "home_sleep" {
					f.sleepErr = errors.New("shared sleep guard must be bypassed")
				}
				status, got := gatewayRequest(t, f.router(action, timers), "hero", "session", "application/json", `{"action":"`+action+`"`+params+`}`, nil)
				if status != 200 || f.executions != 1 {
					t.Fatalf("status=%d response=%s", status, got)
				}
				if action != "home_sleep" && f.sleepCalls != 0 {
					t.Fatal("recovery command used ordinary sleep guard")
				}
				assertGatewayContext(t, got)
			})
		}
	}
	for _, params := range []string{`null`, `[]`, `{"character_id":"other"}`, `{"unknown":1}`} {
		f := newGatewayFixture(t)
		status, got := gatewayRequest(t, f.router("home_sleep", timer.NewService(nil)), "hero", "session", "application/json", `{"action":"home_sleep","params":`+params+`}`, nil)
		if status != 400 || f.executions != 0 {
			t.Fatalf("no-input params accepted: %d %s", status, got)
		}
		assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
	}
	// Premature Wake reaches the service, which owns the recovery check.
	f := newGatewayFixture(t)
	timers := timer.NewService(nil)
	if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
		t.Fatal(err)
	}
	status, got := gatewayRequest(t, f.router("home_wake", timers), "hero", "session", "application/json", `{"action":"home_wake"}`, nil)
	if status != 200 || f.executions != 1 {
		t.Fatalf("premature wake: %d %s", status, got)
	}
}

func TestActionGatewayServiceValidationAndMappings(t *testing.T) {
	for _, mappedStatus := range []int{400, 403, 404, 409, 422, 500} {
		f := newGatewayFixture(t)
		f.executionErr = gatewayRejected
		option := func(h *Handler) {
			c := h.actionCommands["bank_deposit"]
			c.reject = func(error) (int, ErrorDetail) {
				return mappedStatus, ErrorDetail{Code: "SERVICE_RULE", Message: "service rejection"}
			}
			h.actionCommands["bank_deposit"] = c
		}
		// Entry eligibility does not validate the exact requested amount.
		status, got := gatewayRequest(t, f.router("bank_deposit", timer.NewService(nil), option), "hero", "session", "application/json", `{"action":"bank_deposit","params":{"amount":999999}}`, nil)
		if status != mappedStatus || f.executions != 1 {
			t.Fatalf("mapped status=%d response=%s", status, got)
		}
		if mappedStatus == 500 {
			assertGatewayError(t, got, "EXECUTION_FAILED")
			if f.queryCalls != 1 {
				t.Fatal("unknown failure refreshed")
			}
		} else {
			assertGatewayError(t, got, "SERVICE_RULE")
			assertGatewayContext(t, got)
		}
	}
}

// A reader can fail either duration or pending wake reads independently.
type gatewayTimers struct {
	playercontext.TimerReader
	durationErr, asleepErr error
	check                  func(context.Context)
}

func (s gatewayTimers) GetRemainingLock(ctx context.Context, category, id string) (time.Duration, error) {
	if s.check != nil {
		s.check(ctx)
	}
	if s.durationErr != nil {
		return 0, s.durationErr
	}
	return s.TimerReader.GetRemainingLock(ctx, category, id)
}

func (s gatewayTimers) IsLocked(ctx context.Context, category, id string) (bool, error) {
	if s.check != nil {
		s.check(ctx)
	}
	if s.asleepErr != nil {
		return false, s.asleepErr
	}
	return s.TimerReader.IsLocked(ctx, category, id)
}
