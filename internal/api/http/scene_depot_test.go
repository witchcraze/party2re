package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type depotSceneFixture struct {
	*gatewayFixture
	DepotService
	dep                   depot.Depot
	reads, writes         int
	repoErr, depotCharErr error
	timers                timer.Service
}

type depotSceneRepository struct {
	depot.Repository
	f *depotSceneFixture
}

func (r depotSceneRepository) FindByCharacterID(ctx context.Context, actor string) (depot.Depot, error) {
	r.f.checkContext(ctx)
	if actor != "hero" {
		r.f.t.Fatal("wrong depot owner")
	}
	r.f.reads++
	return r.f.dep, r.f.repoErr
}

func (r depotSceneRepository) Save(context.Context, depot.Depot) error {
	r.f.writes++
	return errors.New("observation must not save storage")
}

type depotSceneCharacters struct {
	depot.CharacterRepository
	f *depotSceneFixture
}

func (r depotSceneCharacters) FindByID(ctx context.Context, actor string) (corecharacter.Character, error) {
	r.f.checkContext(ctx)
	if actor != "hero" {
		r.f.t.Fatal("wrong character for Depot read")
	}
	return r.f.char, r.f.depotCharErr
}

func depotSceneRouter(t *testing.T, options ...func(*playercontext.Service)) (*depotSceneFixture, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := &depotSceneFixture{gatewayFixture: newGatewayFixture(t), timers: timer.NewService(nil),
		dep: depot.Depot{CharacterID: "hero", Capacity: 5, Items: []item.Instance{
			{ID: "z", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 6},
			{ID: "a", DefinitionID: "item-010", Quantity: 8},
		}}}
	service, err := depot.NewService(depotSceneRepository{f: f}, depotSceneCharacters{f: f}, &struct{ depot.InventoryRepository }{})
	if err != nil {
		t.Fatal(err)
	}
	f.DepotService = service
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, f.timers, playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}))
	for _, option := range options {
		option(pc)
	}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

func TestDepotSceneOwnedFactsAndReadOnlyNavigation(t *testing.T) {
	for _, tc := range []struct{ job, expanded, over, capacity int }{{0, 0, 0, 5}, {7, 2, 1, 100}, {29, 20, 5, 500}} {
		t.Run(fmt.Sprint(tc.capacity), func(t *testing.T) {
			f, store, router := depotSceneRouter(t)
			f.char.JobLevel, f.char.OverDepot, f.dep.ExDepot = tc.job, tc.over, tc.expanded
			status, observation := navigationGET(t, router)
			if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.LocationID != "depot" || observation.Scene.Support.Observation != "details" {
				t.Fatalf("Depot projection: %d %+v", status, observation)
			}
			data := decodeShopScene[DepotSceneData](t, observation)
			if data.Parent != "town" || data.CharacterID != "hero" || data.Capacity != tc.capacity || data.ExDepot != tc.expanded || data.ItemCount != 2 || len(data.Items) != 2 || data.Items[0].ID != "z" || data.Items[0].DefinitionID != "weapon-01" || data.Items[0].Quantity != 1 || data.Items[0].EnhancementLevel != 6 || data.Items[1].ID != "a" || data.Items[1].Quantity != 8 || data.Items[1].EnhancementLevel != 0 {
				t.Fatalf("lost facts or changed reader order: %+v", data)
			}
			for _, action := range observation.AvailableActions {
				if !slices.Contains([]string{"scene_back", "depot_expand", "depot_sell", "depot_sell_batch"}, action.Action) {
					t.Fatalf("unconnected mutation offered: %+v", action)
				}
			}
			for _, id := range []string{"depot_deposit", "depot_withdraw"} {
				if !slices.ContainsFunc(observation.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == id && !a.Connected }) {
					t.Fatalf("missing connection metadata: %+v", observation.Scene.Support)
				}
			}
			if status, again := navigationGET(t, router); status != 200 || !reflect.DeepEqual(again.Scene, observation.Scene) || store.writes != 0 || f.writes != 0 || f.executions != 0 || f.dep.Capacity != 5 || f.char.Money != 100 || len(f.actions) != 0 {
				t.Fatalf("GET wrote or changed state: %d %+v", status, again)
			}
			store.selection.Destination = "town"
			status, town := navigationGET(t, router)
			if status != 200 || !slices.ContainsFunc(decodeShopScene[TownSceneData](t, town).Destinations, func(d SceneDestination) bool {
				return d.ID == "depot" && d.Supported && d.EnterParams.Destination == "depot"
			}) {
				t.Fatalf("missing Depot entry: %d %+v", status, town)
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"depot"}}`, nil)
			var refreshed PlayerContextResponse
			if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
				t.Fatal(err)
			}
			if status != 200 || !reflect.DeepEqual(refreshed.Scene, observation.Scene) || store.writes != 1 {
				t.Fatalf("entry differs: %d %s", status, raw)
			}
			if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back"}`, nil); status != 200 || store.selection.Destination != "town" || store.writes != 2 || f.writes != 0 {
				t.Fatalf("back: %d %s", status, raw)
			}
		})
	}
}

func TestDepotSceneBoundedPagesAndEmptyDepot(t *testing.T) {
	f, store, router := depotSceneRouter(t)
	f.char.JobLevel = 29
	f.dep.Items = make([]item.Instance, 105)
	for i := range f.dep.Items {
		f.dep.Items[i] = item.Instance{ID: fmt.Sprintf("instance-%03d", 104-i), DefinitionID: "weapon-01", Quantity: i + 1, EnhancementLevel: i % 10}
	}
	for _, tc := range []struct{ offset, limit, count int }{{0, 0, 20}, {0, 1, 1}, {0, 100, 100}, {100, 100, 5}, {105, 20, 0}, {1000000, 20, 0}} {
		store.selection.Offset, store.selection.Limit = tc.offset, tc.limit
		status, observation := navigationGET(t, router)
		if status != 200 {
			t.Fatalf("page: %d", status)
		}
		data := decodeShopScene[DepotSceneData](t, observation)
		wantLimit := tc.limit
		if wantLimit == 0 {
			wantLimit = 20
		}
		if data.Items == nil || len(data.Items) != tc.count || data.ItemCount != 105 || data.Page.Offset != tc.offset || data.Page.Mode != "offset" || data.Page.Limit != wantLimit || (data.Page.Next != nil) != (tc.offset+tc.count < 105) {
			t.Fatalf("page bounds/count: %+v", data)
		}
		for i, row := range data.Items {
			want := f.dep.Items[tc.offset+i]
			if row.ID != want.ID || row.DefinitionID != want.DefinitionID || row.Quantity != want.Quantity || row.EnhancementLevel != want.EnhancementLevel {
				t.Fatalf("page reordered or lost instance: %+v / %+v", row, want)
			}
		}
		if p := data.Page.Next; p != nil {
			if p.Destination != "depot" || p.Offset != tc.offset+tc.count || p.Limit != data.Page.Limit {
				t.Fatalf("next: %+v", p)
			}
		}
	}
	store.selection.Offset, store.selection.Limit = 0, 0
	status, observation := navigationGET(t, router)
	data := decodeShopScene[DepotSceneData](t, observation)
	index := slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" })
	if index < 0 || observation.AvailableActions[index].ParamsTemplate["offset"] != float64(20) {
		t.Fatalf("next page choice: %+v", observation.AvailableActions)
	}
	body, err := json.Marshal(map[string]any{"action": "scene_page", "params": data.Page.Next})
	if err != nil {
		t.Fatal(err)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil); status != 200 || store.selection.Offset != 20 || store.writes != 1 {
		t.Fatalf("next navigation: %d %s", status, raw)
	}
	f.dep.Items = nil
	if status, observation = navigationGET(t, router); status != 200 || decodeShopScene[DepotSceneData](t, observation).Items == nil || decodeShopScene[DepotSceneData](t, observation).ItemCount != 0 || store.writes != 1 || f.writes != 0 {
		t.Fatalf("empty/nonmutating page: %d %+v", status, observation)
	}
}

func TestDepotSceneNavigationRefreshFailureAndGETRecovery(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(fmt.Sprint(rejected), func(t *testing.T) {
			f, store, router := depotSceneRouter(t)
			body, wantStatus, wantWrites := `{"action":"scene_enter","params":{"destination":"depot"}}`, 200, 1
			if rejected {
				body, wantStatus, wantWrites = `{"action":"scene_select","params":{"target_kind":"item","target_id":"z"}}`, 400, 0
				f.repoErr = errors.New("required Depot refresh failed")
			} else {
				store.selection.Destination = "town"
				store.afterSave = func() { f.repoErr = errors.New("required Depot refresh failed") }
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", body, nil)
			if status != wantStatus || store.writes != wantWrites || f.writes != 0 || string(raw["context"]) != "null" {
				t.Fatalf("lost navigation outcome: %d %s", status, raw)
			}
			if rejected {
				assertGatewayError(t, raw, "INVALID_SELECTION")
			} else if string(raw["success"]) != "true" || raw["result"] == nil {
				t.Fatalf("lost successful selection: %s", raw)
			}
			var detail ErrorDetail
			if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" {
				t.Fatalf("refresh failure: %s %v", raw, err)
			}
			f.repoErr = nil
			if status, observation := navigationGET(t, router); status != 200 || observation.Scene.LocationID != "depot" || store.writes != wantWrites || f.writes != 0 || f.executions != 0 {
				t.Fatalf("GET recovery wrote or replayed: %d %+v", status, observation)
			}
		})
	}
}

func TestDepotSceneActualActivitySkipsDepotReads(t *testing.T) {
	for _, kind := range []string{"sleep", "work", "party", "dungeon", "conflict"} {
		t.Run(kind, func(t *testing.T) {
			activities := []playercontext.Activity{}
			f, store, router := depotSceneRouter(t, playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return activities, nil }))
			f.repoErr = errors.New("ordinary Depot must not be read")
			switch kind {
			case "sleep":
				if err := f.timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
			case "work":
				f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(time.Hour)}}
			case "party", "dungeon":
				activities = []playercontext.Activity{{Kind: kind, ID: "active", Role: "member", Phase: "in_progress", Actions: []string{}}}
			case "conflict":
				activities = []playercontext.Activity{{Kind: "party", ID: "room", Role: "member", Actions: []string{}}, {Kind: "dungeon", ID: "run", Role: "member", Actions: []string{}}}
			}
			wantKind := "activity"
			if kind == "conflict" {
				wantKind = "activity_conflict"
			}
			if status, observation := navigationGET(t, router); status != 200 || observation.Scene.Kind != wantKind || f.reads != 0 || store.writes != 0 || f.writes != 0 || f.char.Money != 100 {
				t.Fatalf("ordinary Depot displaced activity: %d %+v", status, observation)
			}
			if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"depot"}}`, nil); status != 409 || store.writes != 0 || f.reads != 0 {
				t.Fatalf("active entry: %d", status)
			}
			if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"depot_expand"}`, nil); status != 409 || f.reads != 0 || store.writes != 0 || f.writes != 0 {
				t.Fatalf("active expansion: %d %s", status, raw)
			}
		})
	}
}

func TestDepotSceneUnconfigured(t *testing.T) {
	f := newGatewayFixture(t)
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}))
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := navigationGET(t, h.Router()); status != http.StatusNotImplemented || store.writes != 0 {
		t.Fatalf("missing Depot service: %d", status)
	}
}

func TestDepotSceneRequiredReadOwnershipAndContext(t *testing.T) {
	f, store, router := depotSceneRouter(t)
	for _, source := range []string{"character", "depot"} {
		f.repoErr, f.depotCharErr = nil, nil
		if source == "character" {
			f.depotCharErr = errors.New("private character read failed")
		} else {
			f.repoErr = errors.New("private depot read failed")
		}
		if status, _ := navigationGET(t, router); status != 500 || store.writes != 0 || f.writes != 0 {
			t.Fatalf("required %s read: %d", source, status)
		}
	}
	f.repoErr, f.depotCharErr, f.char.JobLevel = depot.ErrNotFound, nil, 7
	status, observation := navigationGET(t, router)
	data := decodeShopScene[DepotSceneData](t, observation)
	if status != 200 || data.Capacity != 40 || data.ItemCount != 0 || data.Items == nil || len(data.Items) != 0 || f.writes != 0 {
		t.Fatalf("missing depot must remain in memory: %d %+v", status, data)
	}
	f.repoErr = nil
	ctx := context.WithValue(context.Background(), struct{}{}, "Depot observation")
	f.expectedContext = ctx
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("request context: %d %s", w.Code, w.Body.String())
	}
	f.expectedContext = nil
	before := f.reads
	for _, tc := range []struct {
		actor, token string
		status       int
	}{{"hero", "", 401}, {"other", "session", 403}, {"missing", "session", 404}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+tc.actor+"/context", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status || f.reads != before || store.writes != 0 || f.writes != 0 {
			t.Fatalf("unauthorized Depot read: %d %s", w.Code, w.Body.String())
		}
	}
	f.queryOwner = "other"
	if status, _ := navigationGET(t, router); status != 403 || f.reads != before || f.writes != 0 || store.writes != 0 {
		t.Fatalf("foreign owned snapshot read Depot: %d", status)
	}
}

func TestDepotSceneMutationsRemainUnconnected(t *testing.T) {
	for _, action := range []string{"depot_deposit", "depot_withdraw"} {
		f, store, router := depotSceneRouter(t)
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":{"item_id":"z"}}`, nil)
		if status != 501 || f.reads != 0 || f.writes != 0 || f.executions != 0 || store.writes != 0 {
			t.Fatalf("unconnected mutation executed or refreshed: %d %s", status, raw)
		}
		assertGatewayError(t, raw, "ACTION_NOT_IMPLEMENTED")
	}
}
