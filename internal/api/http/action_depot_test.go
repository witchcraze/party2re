package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type depotExpansionFixture struct {
	*gatewayFixture
	DepotService
	dep     depot.Depot
	readErr error
	reads   int
	sorts   int
}

func (f *depotExpansionFixture) SortItems(ctx context.Context, actor string) (depot.Depot, error) {
	f.sorts++
	return f.Expand(ctx, actor)
}

func (f *depotExpansionFixture) GetDepot(ctx context.Context, actor string) (depot.Depot, error) {
	f.checkContext(ctx)
	if actor != f.char.ID {
		f.t.Fatal("wrong Depot owner")
	}
	f.reads++
	return f.dep, f.readErr
}

func (f *depotExpansionFixture) Expand(ctx context.Context, actor string) (depot.Depot, error) {
	f.checkContext(ctx)
	if actor != f.char.ID {
		f.t.Fatal("wrong expansion actor")
	}
	f.executions++
	if f.afterExecute != nil {
		f.afterExecute()
	}
	return f.dep, f.executionErr
}

func depotExpansionRouter(t *testing.T) (*depotExpansionFixture, *gatewayNavigationStore, timer.Service, http.Handler) {
	t.Helper()
	f := &depotExpansionFixture{gatewayFixture: newGatewayFixture(t), dep: depot.Depot{CharacterID: "hero", Capacity: 10, ExDepot: 1, Items: []item.Instance{{ID: "held", DefinitionID: "weapon-01", Quantity: 2, EnhancementLevel: 6}}}}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	timers := timer.NewService(nil)
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers, playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}))
	opts := []Option{WithDepot(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return f, store, timers, h.Router()
}

func TestDepotExpansionGatewayOutcomeAndGETRecovery(t *testing.T) {
	for _, params := range []string{"", `,"params":{}`} {
		for _, failure := range []string{"", "query", "profile", "depot"} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%t", params, failure, rejected), func(t *testing.T) {
					f, store, _, router := depotExpansionRouter(t)
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "expansion request")
					wantStatus := 200
					if rejected {
						f.executionErr, wantStatus = fmt.Errorf("wrapped: %w", depot.ErrInsufficientFunds), 400
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
					status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_expand"`+params+`}`, f.expectedContext)
					if status != wantStatus || f.executions != 1 {
						t.Fatalf("outcome: %d calls=%d %s", status, f.executions, got)
					}
					if rejected {
						assertGatewayError(t, got, "DEPOT_INSUFFICIENT_FUNDS")
					} else {
						var result depotResponse
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						if string(got["success"]) != "true" || !reflect.DeepEqual(result, toDepotResponse(f.dep)) {
							t.Fatalf("lost result: %s", got)
						}
					}
					if failure == "" {
						assertGatewayContext(t, got)
					} else {
						assertLoopRefreshFailure(t, got)
					}
					f.queryErr, f.profileErr, f.readErr, f.expectedContext = nil, nil, nil, nil
					status, observation := navigationGET(t, router)
					if status != 200 || f.executions != 1 || store.writes != 0 {
						t.Fatalf("GET replayed: %d %+v", status, observation)
					}
					data := decodeShopScene[DepotSceneData](t, observation)
					if data.ExDepot != 1 || data.Capacity != 10 || data.Items[0].EnhancementLevel != 6 {
						t.Fatalf("lost scene: %+v", data)
					}
				})
			}
		}
	}
}

func TestDepotExpansionGatewayStrictInputsAndErrors(t *testing.T) {
	for _, params := range []string{`null`, `[]`, `1`, `"yes"`, `{"character_id":"other"}`, `{"target":"other"}`, `{"cost":0}`, `{"count":20}`, `{"extra":null}`} {
		t.Run(params, func(t *testing.T) {
			f, _, _, router := depotExpansionRouter(t)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_expand","params":`+params+`}`, nil)
			if status != 400 || f.executions != 0 || f.queryCalls != 0 {
				t.Fatalf("decoded invalid input: %d %s", status, got)
			}
			assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
		})
	}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{depot.ErrDepotMaxExpanded, 400, "DEPOT_MAX_EXPANDED"},
		{depot.ErrInsufficientFunds, 400, "DEPOT_INSUFFICIENT_FUNDS"},
		{depot.ErrInvalidCharacterID, 400, "DEPOT_INVALID_CHARACTER_ID"},
		{corecharacter.ErrNotFound, 404, "CHARACTER_NOT_FOUND"},
		{errors.New("private transaction failure"), 500, "EXECUTION_FAILED"},
		{context.Canceled, 500, "EXECUTION_FAILED"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f, _, _, router := depotExpansionRouter(t)
			f.executionErr = fmt.Errorf("wrapped: %w", tc.err)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_expand"}`, nil)
			if status != tc.status || f.executions != 1 {
				t.Fatalf("mapping: %d %s", status, got)
			}
			assertGatewayError(t, got, tc.code)
			if tc.status == 500 {
				if _, exists := got["context"]; exists || strings.Contains(fmt.Sprint(got), "private") {
					t.Fatalf("unknown error disclosed or refreshed: %s", got)
				}
			} else {
				assertGatewayContext(t, got)
			}
		})
	}
}

func TestDepotExpansionGatewayGuards(t *testing.T) {
	for _, tc := range []struct {
		name, path, token string
		status            int
		setup             func(*depotExpansionFixture, *gatewayNavigationStore, timer.Service)
	}{
		{"anonymous", "hero", "", 401, nil}, {"foreign", "other", "session", 403, nil}, {"missing", "missing", "session", 404, nil},
		{"snapshot owner", "hero", "session", 403, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) { f.queryOwner = "other" }},
		{"query", "hero", "session", 500, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) {
			f.queryErr = errors.New("read failed")
		}},
		{"wrong location", "hero", "session", 409, func(_ *depotExpansionFixture, s *gatewayNavigationStore, _ timer.Service) {
			s.selection.Destination = "bank"
		}},
		{"pending wake", "hero", "session", 409, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) { f.char.PendingWake = true }},
		{"sleep", "hero", "session", 409, func(f *depotExpansionFixture, _ *gatewayNavigationStore, timerService timer.Service) {
			if err := timerService.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
				t.Fatal(err)
			}
		}},
		{"unfinished", "hero", "session", 409, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) {
			f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ActionType: "training", ExecuteAt: time.Now().Add(-time.Hour)}}
		}},
		{"sleep read", "hero", "session", 500, func(f *depotExpansionFixture, _ *gatewayNavigationStore, _ timer.Service) {
			f.sleepErr = errors.New("sleep failed")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, store, timers, router := depotExpansionRouter(t)
			if tc.setup != nil {
				tc.setup(f, store, timers)
			}
			status, got := gatewayRequest(t, router, tc.path, tc.token, "application/json", `{"action":"depot_expand"}`, nil)
			if status != tc.status || f.executions != 0 || store.writes != 0 {
				t.Fatalf("guard bypass: %d %s", status, got)
			}
		})
	}
	t.Run("unconfigured", func(t *testing.T) {
		f := newGatewayFixture(t)
		h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(nil), WithPlayerContext(playercontext.NewService(f, f, timer.NewService(nil))))
		if err != nil {
			t.Fatal(err)
		}
		status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", `{"action":"depot_expand"}`, nil)
		if status != 501 || f.queryCalls != 0 {
			t.Fatalf("unconfigured: %d %s", status, got)
		}
		assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
	})
}

func TestDepotExpansionQuoteAndDiscovery(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 19, 20} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f, store, _, router := depotExpansionRouter(t)
			f.dep.ExDepot = count
			f.char.Money = 0 // entry eligibility does not replace exact affordability checks
			status, observation := navigationGET(t, router)
			if status != 200 {
				t.Fatalf("GET status %d", status)
			}
			var facts map[string]json.RawMessage
			encoded, err := json.Marshal(observation.Scene.Data)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &facts); err != nil {
				t.Fatal(err)
			}
			quote, exists := facts["next_expansion_cost"]
			want, err := depot.ExpansionCost(count)
			if !exists || (err == nil && string(quote) != fmt.Sprint(want)) || (err != nil && string(quote) != "null") {
				t.Fatalf("wrong quote: %s", encoded)
			}
			idx := slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "depot_expand" })
			if idx < 0 || !slices.ContainsFunc(observation.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == "depot_expand" && a.Connected && a.EntryEligible }) {
				t.Fatalf("undiscoverable expansion: %+v", observation)
			}
			a := observation.AvailableActions[idx]
			if len(a.RequiredParams) != 0 || len(a.ParamsTemplate) != 0 || !strings.Contains(string(a.ParamsSchema), `"additionalProperties":false`) {
				t.Fatalf("wrong strict discovery: %+v", a)
			}
			resolved := NewActionURLResolver().Resolve("hero", "depot_expand", "", "")
			if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" || f.executions != 0 || store.writes != 0 {
				t.Fatalf("GET or resolver: %+v", resolved)
			}
		})
	}
}
