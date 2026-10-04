package http

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func setHomeWakeReady(f *homeGatewayFixture) {
	f.t.Helper()
	if err := f.timers.SetLock(f.expectedContext, timer.CategoryAsleep, "hero", time.Hour); err != nil {
		f.t.Fatal(err)
	}
}

func TestHomeGatewayInvalidParamsAndOwnership(t *testing.T) {
	for _, action := range []string{"home_sleep", "home_wake"} {
		params := []string{`null`, `[]`, `1`, `"home"`, `{"character_id":"other"}`, `{"player_id":"other"}`, `{"unknown":1}`}
		if action == "home_sleep" {
			params = append(params, `{"target_home_id":1}`, `{"target_home_id":true}`, `{"target_home_id":[]}`)
		} else {
			params = append(params, `{"target_home_id":"other"}`)
		}
		for _, p := range params {
			t.Run(action+p, func(t *testing.T) {
				f := newHomeGatewayFixture(t)
				status, got := f.request(f.router(f.timers), action, `,"params":`+p)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 || f.sleepCalls != 0 {
					t.Fatalf("invalid input reached service: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
		for _, tc := range []struct {
			actor, token string
			status       int
		}{
			{"hero", "", 401}, {"hero", "invalid", 401}, {"other", "session", 403}, {"missing", "session", 404},
		} {
			t.Run(action+tc.actor+tc.token, func(t *testing.T) {
				f := newHomeGatewayFixture(t)
				status, got := gatewayRequest(t, f.router(f.timers), tc.actor, tc.token, "application/json", `{"action":"`+action+`"}`, f.expectedContext)
				if status != tc.status || f.executions != 0 || f.queryCalls != 0 || f.sleepCalls != 0 {
					t.Fatalf("unauthorized input reached service: %d %s", status, got)
				}
				if _, ok := got["context"]; ok {
					t.Fatal("authorization failure exposed context")
				}
			})
		}
	}
}

func TestHomeGatewayNoInputWakeAndGuardException(t *testing.T) {
	for _, params := range []string{"", `,"params":{}`} {
		f := newHomeGatewayFixture(t)
		setHomeWakeReady(f)
		f.sleepErr = errors.New("ordinary sleep guard must be bypassed")
		f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(-time.Hour)}}
		status, got := f.request(f.router(f.timers), "home_wake", params)
		if status != 200 || f.executions != 1 || f.sleepCalls != 0 || f.updates != 1 {
			t.Fatalf("Wake used ordinary guard: %d %s", status, got)
		}
		observation := homeObservation(t, got)
		if len(observation.OngoingActions) != 1 || observation.OngoingActions[0].ID != "work" || !observation.OngoingActions[0].IsReady {
			t.Fatalf("Wake cleared unrelated work: %+v", observation)
		}
	}
}

func TestHomeGatewayGuardAndQueryFailures(t *testing.T) {
	for _, action := range []string{"home_sleep", "home_wake"} {
		for _, failure := range []string{"character", "scheduled", "sleep timer", "wake timer", "owner", "shared sleep guard"} {
			if action == "home_wake" && failure == "shared sleep guard" {
				continue
			}
			t.Run(action+failure, func(t *testing.T) {
				f := newHomeGatewayFixture(t)
				if action == "home_wake" {
					setHomeWakeReady(f)
				}
				reader := gatewayTimers{TimerReader: f.timers, check: f.checkContext}
				want := 500
				switch failure {
				case "character":
					f.characterErr = errors.New("secret character store")
				case "scheduled":
					f.queryErr = errors.New("secret scheduled store")
				case "sleep timer":
					reader.durationErr = errors.New("secret sleep store")
				case "wake timer":
					reader.asleepErr = errors.New("secret wake store")
				case "owner":
					f.queryOwner = "other"
					want = 403
				case "shared sleep guard":
					f.sleepErr = errors.New("secret status store")
				}
				status, got := f.request(f.router(reader), action, "")
				if status != want || f.executions != 0 || f.updates != 0 {
					t.Fatalf("failed preflight executed: %d %s", status, got)
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
}

func TestHomeGatewaySuccessSurvivesRefreshFailure(t *testing.T) {
	for _, action := range []string{"home_sleep", "home_wake"} {
		for _, failure := range []string{"scheduled", "profile"} {
			t.Run(action+failure, func(t *testing.T) {
				f := newHomeGatewayFixture(t)
				if action == "home_wake" {
					setHomeWakeReady(f)
				}
				f.afterExecute = func() {
					if failure == "scheduled" {
						f.queryErr = errors.New("secret query store")
					} else {
						f.profileErr = errors.New("secret profile store")
					}
				}
				status, got := f.request(f.router(f.timers), action, "")
				if status != 200 || f.executions != 1 || string(got["success"]) != "true" || len(got["result"]) == 0 || string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
					t.Fatalf("lost success/result: %d %s", status, got)
				}
				if action == "home_sleep" && !strings.Contains(string(got["result"]), `"duration_seconds":60`) {
					t.Fatal("lost sleep result")
				}
				if action == "home_wake" && (f.updates != 1 || f.char.Stats.HP != 100 || !strings.Contains(string(got["result"]), `"success":true`)) {
					t.Fatal("lost wake recovery/result")
				}
			})
		}
	}
}

func TestHomeGatewayServiceRejectionsAndUnknownFailure(t *testing.T) {
	for _, tc := range []struct {
		action, code string
		err          error
		status       int
	}{
		{"home_sleep", "HOME_ALREADY_SLEEPING", home.ErrAlreadySleeping, 409},
		{"home_sleep", "CHARACTER_NOT_FOUND", home.ErrCharacterNotFound, 404},
		{"home_sleep", "CHARACTER_NOT_FOUND", corecharacter.ErrNotFound, 404},
		{"home_sleep", "HOME_HOUSE_NOT_FOUND", home.ErrHouseNotFound, 404},
		{"home_sleep", "HOME_HOUSE_EXPIRED", home.ErrHouseExpired, 404},
		{"home_wake", "HOME_STILL_SLEEPING", home.ErrStillSleeping, 409},
		{"home_wake", "HOME_NOT_SLEEPING", home.ErrNotSleeping, 409},
		{"home_wake", "CHARACTER_NOT_FOUND", corecharacter.ErrNotFound, 404},
		{"home_sleep", "EXECUTION_FAILED", errors.New("secret timer write"), 500},
		{"home_wake", "EXECUTION_FAILED", errors.New("secret recovery write"), 500},
	} {
		for _, refreshFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/%t", tc.action, tc.code, refreshFails), func(t *testing.T) {
				f := newHomeGatewayFixture(t)
				if tc.action == "home_wake" {
					setHomeWakeReady(f)
				}
				f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
				if refreshFails {
					f.afterExecute = func() { f.profileErr = errors.New("secret profile read") }
				}
				status, got := f.request(f.router(f.timers), tc.action, "")
				if status != tc.status || f.executions != 1 || f.updates != 0 {
					t.Fatalf("status=%d: %s", status, got)
				}
				assertGatewayError(t, got, tc.code)
				if strings.Contains(string(got["error"]), "secret") {
					t.Fatal("internal error leaked")
				}
				if tc.status == 500 {
					if _, ok := got["context"]; ok {
						t.Fatal("unknown outcome exposed context")
					}
					if f.queryCalls != 1 || f.profileCalls != 0 {
						t.Fatal("unknown outcome refreshed")
					}
				} else if refreshFails {
					if string(got["context"]) != "null" || !strings.Contains(string(got["context_error"]), "CONTEXT_REFRESH_FAILED") {
						t.Fatalf("lost rejection: %s", got)
					}
				} else {
					homeObservation(t, got)
				}
			})
		}
	}
}

func TestHomeGatewayServiceRevalidatesEarlyWakeAndSleepRace(t *testing.T) {
	for _, action := range []string{"home_sleep", "home_wake"} {
		f := newHomeGatewayFixture(t)
		if action == "home_wake" {
			setHomeWakeReady(f)
		}
		f.beforeExecute = func() {
			if err := f.timers.SetLock(f.expectedContext, timer.CategorySleep, "hero", time.Hour); err != nil {
				t.Fatal(err)
			}
		}
		status, got := f.request(f.router(f.timers), action, "")
		if status != 409 || f.executions != 1 || f.updates != 0 {
			t.Fatalf("service revalidation bypassed: %d %s", status, got)
		}
		code := "HOME_ALREADY_SLEEPING"
		if action == "home_wake" {
			code = "HOME_STILL_SLEEPING"
		}
		assertGatewayError(t, got, code)
		homeObservation(t, got)
	}
}

func TestHomeGatewayUnconfigured(t *testing.T) {
	for _, action := range []string{"home_sleep", "home_wake"} {
		for _, missingHome := range []bool{false, true} {
			f := newHomeGatewayFixture(t)
			var option Option = WithPlayerContext(nil)
			if missingHome {
				option = WithHome(nil)
			}
			// Configure Home only once through the public construction seam.
			options := []Option{WithPlayerContext(playercontext.NewService(f.gatewayFixture, f.gatewayFixture, f.timers)), option}
			if !missingHome {
				options = append(options, WithHome(f))
			}
			h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
			if err != nil {
				t.Fatal(err)
			}
			status, got := f.request(h.Router(), action, "")
			if status != 501 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("unconfigured dependency executed: %d %s", status, got)
			}
			assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
		}
	}
}
