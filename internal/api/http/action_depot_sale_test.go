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
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type depotSaleFixture struct {
	*depotExpansionFixture
	ids        []string
	batch      bool
	activities []playercontext.Activity
}

func (f *depotSaleFixture) SellItem(ctx context.Context, actor, id string) (depot.Depot, int, error) {
	f.ids, f.batch = []string{id}, false
	dep, err := f.Expand(ctx, actor)
	return dep, 150, err
}

func (f *depotSaleFixture) SellItems(ctx context.Context, actor string, ids []string) (depot.Depot, int, error) {
	f.ids, f.batch = ids, true
	dep, err := f.Expand(ctx, actor)
	return dep, 150, err
}

func depotSaleRouter(t *testing.T) (*depotSaleFixture, *gatewayNavigationStore, timer.Service, http.Handler) {
	t.Helper()
	f := &depotSaleFixture{depotExpansionFixture: &depotExpansionFixture{gatewayFixture: newGatewayFixture(t), dep: depot.Depot{CharacterID: "hero", Capacity: 10, ExDepot: 1, Items: []item.Instance{{ID: "survivor", DefinitionID: "weapon-01", Quantity: 2, EnhancementLevel: 6}}}}}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	timers := timer.NewService(nil)
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers, playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"}),
		playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return f.activities, nil }))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, timers, h.Router()
}

func saleGatewayBody(action, params string) string {
	if params == "" {
		return `{"action":"` + action + `"}`
	}
	return `{"action":"` + action + `","params":` + params + `}`
}

func TestDepotSaleGatewayInputs(t *testing.T) {
	for _, action := range []string{"depot_sell", "depot_sell_batch"} {
		invalid := []string{"", `null`, `{}`, `[]`, `1`, `{"item_id":null}`, `{"item_ids":null}`}
		valid := `{"item_id":"one"}`
		if action == "depot_sell" {
			invalid = append(invalid, `{"item_id":1}`, `{"item_id":[]}`, `{"item_ids":["one"]}`)
		} else {
			valid = `{"item_ids":["one","two"]}`
			invalid = append(invalid, `{"item_ids":"one"}`, `{"item_ids":1}`, `{"item_ids":[1]}`, `{"item_ids":[null]}`, `{"item_ids":["one",null]}`, `{"item_id":"one"}`)
		}
		for _, field := range []string{`"character_id":"other"`, `"player_id":"other"`, `"price":0`, `"quantity":1`, `"extra":null`} {
			invalid = append(invalid, strings.TrimSuffix(valid, "}")+","+field+"}")
		}
		for _, params := range invalid {
			t.Run(action+params, func(t *testing.T) {
				f, store, _, router := depotSaleRouter(t)
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(action, params), nil)
				if status != 400 || f.executions != 0 || f.queryCalls != 0 || store.writes != 0 {
					t.Fatalf("invalid params executed: %d %s", status, got)
				}
				assertGatewayError(t, got, "INVALID_ACTION_PARAMS")
			})
		}
	}
}

func TestDepotSaleGatewayOutcomeRecovery(t *testing.T) {
	for _, action := range []string{"depot_sell", "depot_sell_batch"} {
		for _, failure := range []string{"", "query", "profile", "depot"} {
			for _, rejected := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%t", action, failure, rejected), func(t *testing.T) {
					f, store, _, router := depotSaleRouter(t)
					f.expectedContext = context.WithValue(context.Background(), struct{}{}, "sale context")
					params, wantIDs := `{"item_id":"one"}`, []string{"one"}
					if action == "depot_sell_batch" {
						params, wantIDs = `{"item_ids":["two","one"]}`, []string{"two", "one"}
					}
					wantStatus := 200
					if rejected {
						f.executionErr, wantStatus = fmt.Errorf("wrapped: %w", depot.ErrItemNotFound), 404
					}
					f.afterExecute = func() {
						switch failure {
						case "query":
							f.queryErr = errors.New("private query")
						case "profile":
							f.profileErr = errors.New("private profile")
						case "depot":
							f.readErr = errors.New("private Depot")
						}
					}
					status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(action, params), f.expectedContext)
					if status != wantStatus || f.executions != 1 || f.batch != (action == "depot_sell_batch") || !reflect.DeepEqual(f.ids, wantIDs) {
						t.Fatalf("lost intent: %d %s ids=%v", status, got, f.ids)
					}
					if rejected {
						assertGatewayError(t, got, "DEPOT_ITEM_NOT_FOUND")
					} else {
						var result sellDepotResponse
						if err := json.Unmarshal(got["result"], &result); err != nil {
							t.Fatal(err)
						}
						if string(got["success"]) != "true" || !reflect.DeepEqual(result, sellDepotResponse{Depot: toDepotResponse(f.dep), GoldEarned: 150}) {
							t.Fatalf("lost result: %s", got)
						}
					}
					if failure == "" {
						assertGatewayContext(t, got)
					} else {
						assertLoopRefreshFailure(t, got)
					}
					f.queryErr, f.profileErr, f.readErr, f.expectedContext = nil, nil, nil, nil
					if status, _ := navigationGET(t, router); status != 200 || f.executions != 1 || store.writes != 0 {
						t.Fatal("GET replayed sale")
					}
				})
			}
		}
	}
}

func TestDepotSaleGatewayErrors(t *testing.T) {
	for _, action := range []string{"depot_sell", "depot_sell_batch"} {
		params := `{"item_id":""}`
		if action == "depot_sell_batch" {
			params = `{"item_ids":[]}`
		}
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{depot.ErrInvalidCharacterID, 400, "DEPOT_INVALID_CHARACTER_ID"}, {depot.ErrInvalidItemInstanceID, 400, "DEPOT_INVALID_ITEM_INSTANCE_ID"},
			{depot.ErrEmptyItemList, 400, "DEPOT_EMPTY_ITEM_LIST"}, {depot.ErrInvalidAmount, 400, "DEPOT_INVALID_AMOUNT"}, {depot.ErrInvalidQuantity, 400, "DEPOT_INVALID_QUANTITY"},
			{depot.ErrItemNotFound, 404, "DEPOT_ITEM_NOT_FOUND"}, {depot.ErrNotFound, 404, "DEPOT_NOT_FOUND"}, {corecharacter.ErrNotFound, 404, "CHARACTER_NOT_FOUND"},
			{errors.New("private catalog/repository failure"), 500, "EXECUTION_FAILED"}, {economy.ErrGoldOverflow, 500, "EXECUTION_FAILED"}, {context.Canceled, 500, "EXECUTION_FAILED"},
		} {
			t.Run(action+tc.code, func(t *testing.T) {
				f, _, _, router := depotSaleRouter(t)
				f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
				status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(action, params), nil)
				if status != tc.status || f.executions != 1 {
					t.Fatalf("error mapping: %d %s", status, got)
				}
				assertGatewayError(t, got, tc.code)
				if tc.status == 500 {
					if _, ok := got["context"]; ok || strings.Contains(fmt.Sprint(got), "private") {
						t.Fatalf("unknown failure disclosed/refreshed: %s", got)
					}
				} else {
					assertGatewayContext(t, got)
				}
			})
		}
	}
}

func TestDepotSaleGatewayGuardsAndDiscovery(t *testing.T) {
	for _, action := range []string{"depot_sell", "depot_sell_batch"} {
		params := `{"item_id":"one"}`
		field, kind := "item_id", "string"
		if action == "depot_sell_batch" {
			params, field, kind = `{"item_ids":["one"]}`, "item_ids", "array"
		}
		for _, tc := range []struct {
			name, path, token string
			status            int
			setup             func(*depotSaleFixture, *gatewayNavigationStore, timer.Service)
		}{
			{"anonymous", "hero", "", 401, nil}, {"foreign", "other", "session", 403, nil}, {"missing", "missing", "session", 404, nil},
			{"query owner", "hero", "session", 403, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) { f.queryOwner = "other" }},
			{"query failure", "hero", "session", 500, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) { f.queryErr = errors.New("read") }},
			{"wrong location", "hero", "session", 409, func(_ *depotSaleFixture, s *gatewayNavigationStore, _ timer.Service) {
				s.selection.Destination = "bank"
			}},
			{"wake", "hero", "session", 409, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) { f.char.PendingWake = true }},
			{"sleep", "hero", "session", 409, func(_ *depotSaleFixture, _ *gatewayNavigationStore, timers timer.Service) {
				if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			}},
			{"work", "hero", "session", 409, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) {
				f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ActionType: "training"}}
			}},
			{"activity", "hero", "session", 409, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) {
				f.activities = []playercontext.Activity{{Kind: "casino", ID: "room", Role: "player", Phase: "playing"}}
			}},
			{"conflict", "hero", "session", 409, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) {
				f.activities = []playercontext.Activity{{Kind: "casino", ID: "room"}, {Kind: "dungeon", ID: "run"}}
			}},
			{"sleep read", "hero", "session", 500, func(f *depotSaleFixture, _ *gatewayNavigationStore, _ timer.Service) { f.sleepErr = errors.New("read") }},
		} {
			t.Run(action+tc.name, func(t *testing.T) {
				f, store, timers, router := depotSaleRouter(t)
				if tc.setup != nil {
					tc.setup(f, store, timers)
				}
				status, got := gatewayRequest(t, router, tc.path, tc.token, "application/json", saleGatewayBody(action, params), nil)
				if status != tc.status || f.executions != 0 || store.writes != 0 {
					t.Fatalf("guard bypass: %d %s", status, got)
				}
			})
		}
		t.Run(action+"discovery", func(t *testing.T) {
			f, store, _, router := depotSaleRouter(t)
			f.char.Money, f.char.Stats.HP, f.char.Tired = 0, 0, 100
			status, got := navigationGET(t, router)
			idx := slices.IndexFunc(got.AvailableActions, func(a ContextAction) bool { return a.Action == action })
			if status != 200 || idx < 0 || !slices.ContainsFunc(got.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == action && a.Connected && a.EntryEligible }) {
				t.Fatalf("missing sale discovery: %d %+v", status, got)
			}
			a := got.AvailableActions[idx]
			var schema struct {
				AdditionalProperties bool `json:"additionalProperties"`
				Properties           map[string]struct {
					Type  string
					Items struct{ Type string }
				}
				Required []string
			}
			if err := json.Unmarshal(a.ParamsSchema, &schema); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a.RequiredParams, []string{field}) || !slices.Equal(schema.Required, []string{field}) || schema.AdditionalProperties || schema.Properties[field].Type != kind || len(a.ParamsTemplate) != 0 || (kind == "array" && schema.Properties[field].Items.Type != "string") {
				t.Fatalf("incorrect sale params: %+v %s", a, a.ParamsSchema)
			}
			resolved := NewActionURLResolver().Resolve("hero", action, "", "")
			if resolved.Method != "POST" || resolved.URL != "/api/v1/characters/hero/actions" || f.executions != 0 || store.writes != 0 {
				t.Fatal("incorrect sale link or GET mutation")
			}
		})
		t.Run(action+"unconfigured", func(t *testing.T) {
			for _, option := range []Option{WithDepot(nil), func(*Handler) {}} {
				f := newGatewayFixture(t)
				h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, option, WithPlayerContext(playercontext.NewService(f, f, timer.NewService(nil))))
				if err != nil {
					t.Fatal(err)
				}
				status, got := gatewayRequest(t, h.Router(), "hero", "session", "application/json", saleGatewayBody(action, params), nil)
				if status != 501 || f.queryCalls != 0 {
					t.Fatalf("unconfigured: %d %s", status, got)
				}
				assertGatewayError(t, got, "ACTION_NOT_IMPLEMENTED")
			}
		})
	}
}
