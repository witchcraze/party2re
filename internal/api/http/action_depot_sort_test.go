package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestDepotSortGatewayOutcomeAndRecovery(t *testing.T) {
	for _, params := range []string{"", `,"params":{}`} {
		for _, failure := range []string{"", "query", "profile", "depot"} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%t", params, failure, rejected), func(t *testing.T) {
					f, store, _, router := depotExpansionRouter(t)
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "sort request")
					wantStatus := 200
					if rejected {
						f.executionErr, wantStatus = fmt.Errorf("wrapped: %w", depot.ErrNotFound), 404
					}
					f.afterExecute = func() {
						switch failure {
						case "query":
							f.queryErr = errors.New("private query")
						case "profile":
							f.profileErr = errors.New("private profile")
						case "depot":
							f.readErr = errors.New("private depot")
						}
					}
					status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_sort"`+params+`}`, f.expectedContext)
					if status != wantStatus || f.executions != 1 || f.sorts != 1 {
						t.Fatalf("sort outcome: %d calls=%d sorts=%d %s", status, f.executions, f.sorts, got)
					}
					if rejected {
						assertGatewayError(t, got, "DEPOT_NOT_FOUND")
					} else {
						var result depotResponse
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						if string(got["success"]) != "true" || !reflect.DeepEqual(result, toDepotResponse(f.dep)) {
							t.Fatalf("lost sort result: %s", got)
						}
					}
					if failure == "" {
						assertGatewayContext(t, got)
					} else {
						assertLoopRefreshFailure(t, got)
					}
					f.queryErr, f.profileErr, f.readErr, f.expectedContext = nil, nil, nil, nil
					status, observation := navigationGET(t, router)
					if status != 200 || f.executions != 1 || f.sorts != 1 || store.writes != 0 || decodeShopScene[DepotSceneData](t, observation).Items[0].EnhancementLevel != 6 {
						t.Fatalf("GET recovery: %d %+v", status, observation)
					}
				})
			}
		}
	}
}

func TestDepotSortGatewayInputsErrorsAndDiscovery(t *testing.T) {
	for _, params := range []string{`null`, `[]`, `1`, `"yes"`, `{"character_id":"other"}`, `{"order":"id"}`, `{"extra":null}`} {
		t.Run(params, func(t *testing.T) {
			f, store, _, router := depotExpansionRouter(t)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_sort","params":`+params+`}`, nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 || store.writes != 0 {
				t.Fatalf("invalid input executed: %d %s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
	for _, err := range []error{errors.New("private save failure"), context.Canceled} {
		f, _, _, router := depotExpansionRouter(t)
		f.executionErr = fmt.Errorf("wrapped: %w", err)
		status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_sort"}`, nil)
		assertGatewayError(t, got, "EXECUTION_FAILED")
		if status != 500 || f.sorts != 1 || f.queryCalls != 1 || strings.Contains(fmt.Sprint(got), "private") {
			t.Fatalf("unknown failure leaked or refreshed: %d %s", status, got)
		}
		if _, exists := got["context"]; exists {
			t.Fatal("unknown execution failure refreshed")
		}
	}
	f, store, _, router := depotExpansionRouter(t)
	status, observation := navigationGET(t, router)
	idx := slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "depot_sort" })
	if status != 200 || idx < 0 || !slices.ContainsFunc(observation.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == "depot_sort" && a.Connected && a.EntryEligible }) {
		t.Fatalf("sort discovery: %d %+v", status, observation)
	}
	action := observation.AvailableActions[idx]
	if action.RequiredParams == nil || len(action.RequiredParams) != 0 || len(action.ParamsTemplate) != 0 || !strings.Contains(string(action.ParamsSchema), `"additionalProperties":false`) {
		t.Fatalf("sort schema/template: %+v", action)
	}
	resolved := NewActionURLResolver().Resolve("hero", "depot_sort", "", "")
	if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" || f.executions != 0 || store.writes != 0 {
		t.Fatalf("sort resolver/GET: %+v", resolved)
	}
}

func TestDepotSortGatewayGuards(t *testing.T) {
	for _, tc := range []struct {
		name, path, token string
		status            int
		setup             func(*depotExpansionFixture, *gatewayNavigationStore, timer.Service)
	}{
		{"anonymous", "hero", "", 401, nil},
		{"foreign", "other", "session", 403, nil},
		{"missing", "missing", "session", 404, nil},
		{"snapshot owner", "hero", "session", 403, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) { f.queryOwner = "other" }},
		{"query", "hero", "session", 500, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) {
			f.queryErr = errors.New("read failed")
		}},
		{"location", "hero", "session", 409, func(_ *depotExpansionFixture, s *gatewayNavigationStore, _ timer.Service) {
			s.selection.Destination = "bank"
		}},
		{"wake", "hero", "session", 409, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) { f.char.PendingWake = true }},
		{"unfinished", "hero", "session", 409, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) {
			f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ActionType: "training"}}
		}},
		{"sleep read", "hero", "session", 500, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) {
			f.sleepErr = errors.New("sleep read failed")
		}},
		{"sleep", "hero", "session", 409, func(_ *depotExpansionFixture, _ *gatewayNavigationStore, timers timer.Service) {
			if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, store, timers, router := depotExpansionRouter(t)
			if tc.setup != nil {
				tc.setup(f, store, timers)
			}
			status, got := gatewayRequest(t, router, tc.path, tc.token, "application/json", `{"action":"depot_sort"}`, nil)
			if status != tc.status || f.executions != 0 || f.sorts != 0 || store.writes != 0 {
				t.Fatalf("sort guard: %d %s", status, got)
			}
		})
	}
	f := newGatewayFixture(t)
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(nil), WithPlayerContext(playercontext.NewService(f, f, timer.NewService(nil))))
	if err != nil {
		t.Fatal(err)
	}
	status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"depot_sort"}`, nil)
	if status != 501 || f.queryCalls != 0 {
		t.Fatalf("unconfigured sort: %d %s", status, got)
	}
	assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
}
