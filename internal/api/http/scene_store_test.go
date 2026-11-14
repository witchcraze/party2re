package http

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
)

type storeSceneService struct {
	StoreService
	stores           map[string]store.Store
	townErr, getErr  error
	reads, mutations int
}

func (s *storeSceneService) GetTownStores(_ context.Context, townID string) ([]store.Store, error) {
	s.reads++
	var rows []store.Store
	for _, st := range s.stores {
		if st.TownID == townID {
			rows = append(rows, st)
		}
	}
	return rows, s.townErr
}

func (s *storeSceneService) GetStore(_ context.Context, id string) (*store.StoreDetails, error) {
	s.reads++
	if s.getErr != nil {
		return nil, s.getErr
	}
	st, ok := s.stores[id]
	if !ok {
		return nil, store.ErrStoreNotFound
	}
	return &store.StoreDetails{Store: st, OwnerName: "Owner", Sales: []store.Sale{}, Interiors: []store.Interior{}, MaxSales: 10}, nil
}

func (s *storeSceneService) BuyItem(context.Context, string, string) error {
	s.mutations++
	return nil
}

func storeSceneRouter(t *testing.T, service *storeSceneService, selection playercontext.Selection) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	nav := &gatewayNavigationStore{selection: selection}
	scenes := []playercontext.SceneDefinition{{ID: "town", Pageable: true}}
	for _, id := range []string{"town1", "town2", "town3", "town4"} {
		scenes = append(scenes, playercontext.SceneDefinition{ID: id, Parent: "town", SubjectKind: "store",
			SubjectAvailable: func(context.Context, string, string) (bool, error) { return true, nil },
			SubjectOwned: func(_ context.Context, actor, target string) (bool, error) {
				return service.stores[target].CharacterID == actor, nil
			}})
	}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(nav, scenes...))
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc), WithStore(service), func(h *Handler) { h.homes = f })
	if err != nil {
		t.Fatal(err)
	}
	return nav, h.Router()
}

func actionNames(actions []ContextAction) []string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Action
	}
	return names
}

func newStoreSceneService() *storeSceneService {
	exp := time.Now().Add(time.Hour)
	return &storeSceneService{stores: map[string]store.Store{
		"mine":  {ID: "mine", CharacterID: "hero", TownID: "town1", StoreName: "@Heroの店@", ExpiresAt: exp},
		"peer":  {ID: "peer", CharacterID: "peer", TownID: "town1", StoreName: "@Peerの店@", ExpiresAt: exp},
		"other": {ID: "other", CharacterID: "peer2", TownID: "town2", StoreName: "@Peer2の店@", ExpiresAt: exp},
	}}
}

func TestTownHubListsLegacyTowns(t *testing.T) {
	_, router := storeSceneRouter(t, newStoreSceneService(), playercontext.Selection{Destination: "town"})
	status, observation := navigationGET(t, router)
	data := decodeShopScene[TownSceneData](t, observation)
	var titles []string
	for _, d := range data.Destinations {
		titles = append(titles, d.Title)
	}
	if status != 200 || !slices.Equal(titles, []string{"メケメケ村", "キノコ町", "スライム町", "ガイア国"}) {
		t.Fatalf("towns: %d %v", status, titles)
	}
}

func TestTownDistrictObservesStoresAndBuildTerms(t *testing.T) {
	service := newStoreSceneService()
	nav, router := storeSceneRouter(t, service, playercontext.Selection{Destination: "town1"})
	status, observation := navigationGET(t, router)
	data := decodeShopScene[TownDistrictSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "メケメケ村" || observation.Scene.Support.Observation != "details" ||
		len(data.Stores) != 2 || data.Town.ID != "town1" || data.StoreTerms.Price != store.StorePrice || len(data.StoreTerms.HouseStyles) != 4 {
		t.Fatalf("district: %d %+v %+v", status, observation.Scene, data)
	}
	names := actionNames(observation.AvailableActions)
	for _, want := range []string{"store_build", "scene_select", "scene_back"} {
		if !slices.Contains(names, want) {
			t.Fatalf("missing %s: %v", want, names)
		}
	}
	for _, a := range observation.AvailableActions {
		if a.Action == "store_build" && a.ParamsTemplate["town_id"] != "town1" {
			t.Fatalf("build template: %+v", a)
		}
		if a.Action == "scene_select" && a.ParamsTemplate["target_kind"] != "store" {
			t.Fatalf("select template: %+v", a)
		}
	}
	if slices.Contains(names, "store_buy") || service.mutations != 0 || nav.writes != 0 {
		t.Fatalf("district exposed store controls or wrote: %v %d %d", names, service.mutations, nav.writes)
	}
}

func TestStoreSceneOwnerAndVisitorCommands(t *testing.T) {
	for _, tc := range []struct {
		store string
		want  []string
		deny  []string
	}{
		{"mine", storeOwnerCommands, storeVisitorCommands},
		{"peer", storeVisitorCommands, storeOwnerCommands},
	} {
		t.Run(tc.store, func(t *testing.T) {
			service := newStoreSceneService()
			_, router := storeSceneRouter(t, service, playercontext.Selection{Destination: "town1", Subject: playercontext.Subject{Kind: "store", ID: tc.store}})
			status, observation := navigationGET(t, router)
			data := decodeShopScene[StoreSceneData](t, observation)
			if status != 200 || observation.Scene.Kind != "subject" || observation.Scene.Title != service.stores[tc.store].StoreName ||
				data.Parent != "town1" || data.IsOwner != (tc.store == "mine") || data.Details.Sales == nil {
				t.Fatalf("store: %d %+v %+v", status, observation.Scene, data)
			}
			names := actionNames(observation.AvailableActions)
			for _, want := range append(slices.Clone(tc.want), "scene_back") {
				if !slices.Contains(names, want) {
					t.Fatalf("missing %s: %v", want, names)
				}
			}
			for _, deny := range append(slices.Clone(tc.deny), "store_build", "scene_select") {
				if slices.Contains(names, deny) {
					t.Fatalf("unexpected %s: %v", deny, names)
				}
			}
			if service.mutations != 0 {
				t.Fatal("GET mutated")
			}
		})
	}
}

// Owner commands act on the actor's own store whatever store is selected, so
// they are admitted only while the actor's own store is the selected one.
func TestStoreCommandsFollowTheSelectedStoreOwnership(t *testing.T) {
	for _, tc := range []struct {
		store, body string
		status      int
	}{
		{"peer", `{"action":"store_rename","params":{"store_name":"x"}}`, 409},
		{"peer", `{"action":"store_buy","params":{"sale_id":"s"}}`, 200},
		{"mine", `{"action":"store_buy","params":{"sale_id":"s"}}`, 409},
	} {
		service := newStoreSceneService()
		_, router := storeSceneRouter(t, service, playercontext.Selection{Destination: "town1", Subject: playercontext.Subject{Kind: "store", ID: tc.store}})
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", tc.body, nil)
		if status != tc.status || (service.mutations != 0) != (tc.status == 200) {
			t.Fatalf("%s %s: %d %s (mutations %d)", tc.store, tc.body, status, raw, service.mutations)
		}
		if tc.status == 409 {
			assertGatewayError(t, raw, "ACTION_UNAVAILABLE")
		}
	}
}

func TestStoreSceneUnavailableAndRequiredReads(t *testing.T) {
	for name, tc := range map[string]struct {
		selection playercontext.Selection
		mutate    func(*storeSceneService)
		status    int
		kind      string
	}{
		"other town":   {playercontext.Selection{Destination: "town1", Subject: playercontext.Subject{Kind: "store", ID: "other"}}, nil, 200, "selection_unavailable"},
		"missing":      {playercontext.Selection{Destination: "town1", Subject: playercontext.Subject{Kind: "store", ID: "gone"}}, nil, 200, "selection_unavailable"},
		"expired":      {playercontext.Selection{Destination: "town1", Subject: playercontext.Subject{Kind: "store", ID: "mine"}}, func(s *storeSceneService) { s.getErr = store.ErrStoreExpired }, 200, "selection_unavailable"},
		"store read":   {playercontext.Selection{Destination: "town1", Subject: playercontext.Subject{Kind: "store", ID: "mine"}}, func(s *storeSceneService) { s.getErr = errors.New("private read") }, 500, ""},
		"stores read":  {playercontext.Selection{Destination: "town1"}, func(s *storeSceneService) { s.townErr = errors.New("private read") }, 500, ""},
		"unconfigured": {playercontext.Selection{Destination: "town1"}, nil, 501, ""},
	} {
		t.Run(name, func(t *testing.T) {
			service := newStoreSceneService()
			if tc.mutate != nil {
				tc.mutate(service)
			}
			var router http.Handler
			if name == "unconfigured" {
				f := newGatewayFixture(t)
				pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(&gatewayNavigationStore{selection: tc.selection},
					playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "town1", Parent: "town"}))
				h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc), func(h *Handler) { h.homes = f })
				if err != nil {
					t.Fatal(err)
				}
				router = h.Router()
			} else {
				_, router = storeSceneRouter(t, service, tc.selection)
			}
			status, observation := navigationGET(t, router)
			if status != tc.status || (tc.kind != "" && observation.Scene.Kind != tc.kind) {
				t.Fatalf("%d %+v", status, observation.Scene)
			}
		})
	}
}
