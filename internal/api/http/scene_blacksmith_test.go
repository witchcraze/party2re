package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type blacksmithSceneService struct {
	BlacksmithService
	observation blacksmith.Observation
	err         error
	reads       int
	mutations   int
}

func (s *blacksmithSceneService) Observe(_ context.Context, actor string) (blacksmith.Observation, error) {
	if actor != "hero" {
		return blacksmith.Observation{}, errors.New("wrong blacksmith owner")
	}
	s.reads++
	return s.observation, s.err
}

func (s *blacksmithSceneService) ApplySeal(context.Context, string, int) (blacksmith.Seal, error) {
	s.mutations++
	return blacksmith.Seals[0], nil
}

func blacksmithSceneRouter(t *testing.T, service BlacksmithService) (*gatewayFixture, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "blacksmith"}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "blacksmith", Parent: "town"}))
	options := []Option{WithPlayerContext(pc), func(h *Handler) { h.homes = f }}
	if service != nil {
		options = append(options, WithBlacksmith(service))
	}
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

func TestBlacksmithSceneObservesOwnedFactsWithoutMutation(t *testing.T) {
	service := &blacksmithSceneService{observation: blacksmith.Observation{
		Crystal: 70, WeaponSealID: 2, AvailableSealIDs: []int{1, 2},
		Weapon:   &blacksmith.EquippedItem{ItemDefinitionID: "weapon-01", Name: "名剣", CustomName: "名剣"},
		Deposits: []blacksmith.StoredWeapon{{Deposit: blacksmith.Deposit{ID: "d", CharacterID: "hero", Slot: 2, ItemDefinitionID: "weapon-02", SealID: 1, CreatedAt: time.Unix(1, 0).UTC()}, Name: "カタログ名"}},
	}}
	_, store, router := blacksmithSceneRouter(t, service)
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.LocationID != "blacksmith" || observation.Scene.Support.Observation != "details" {
		t.Fatalf("projection: %d %+v", status, observation)
	}
	data := decodeShopScene[BlacksmithSceneData](t, observation)
	if data.Parent != "town" || !reflect.DeepEqual(data.Observation, service.observation) || !reflect.DeepEqual(data.Seals, blacksmith.Seals) || data.StorageCapacity != 3 {
		t.Fatalf("lost facts: %+v", data)
	}
	for _, action := range []string{"blacksmith_seal", "blacksmith_name", "blacksmith_deposit", "blacksmith_withdraw", "scene_back"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("missing connected choice %s: %+v", action, observation.AvailableActions)
		}
	}
	if store.writes != 0 || service.mutations != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, service.mutations)
	}

	store.selection.Destination = "town"
	status, town := navigationGET(t, router)
	if status != 200 || !slices.ContainsFunc(decodeShopScene[TownSceneData](t, town).Destinations, func(d SceneDestination) bool {
		return d.ID == "blacksmith" && d.Supported && d.EnterParams.Destination == "blacksmith"
	}) {
		t.Fatalf("missing town entry: %d %+v", status, town)
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"blacksmith"}}`, nil)
	var refreshed PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
		t.Fatal(err)
	}
	if status != 200 || !reflect.DeepEqual(refreshed.Scene, observation.Scene) || store.writes != 1 {
		t.Fatalf("entry differs: %d %s", status, raw)
	}
}

func TestBlacksmithSceneRequiredReadFailureAndGETRecovery(t *testing.T) {
	service := &blacksmithSceneService{err: errors.New("private storage read failed")}
	f, store, router := blacksmithSceneRouter(t, service)
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
	// A confirmed command outcome survives a failed refresh; only GET recovers.
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"blacksmith_seal","params":{"seal_id":1}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || service.mutations != 1 {
		t.Fatalf("lost command outcome: %d %s", status, raw)
	}
	var detail ErrorDetail
	if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" {
		t.Fatalf("refresh failure: %s %v", raw, err)
	}
	service.err = nil
	if status, _ := navigationGET(t, router); status != 200 || service.mutations != 1 || store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET recovery replayed: %d", status)
	}
}

func TestBlacksmithSceneUnconfigured(t *testing.T) {
	_, store, router := blacksmithSceneRouter(t, nil)
	if status, _ := navigationGET(t, router); status != http.StatusNotImplemented || store.writes != 0 {
		t.Fatalf("missing service: %d", status)
	}
}

func TestRetiredBlacksmithReadsDoNotObserve(t *testing.T) {
	service := &blacksmithSceneService{}
	_, _, router := blacksmithSceneRouter(t, service)
	for _, path := range []string{"/blacksmith/seals", "/characters/hero/blacksmith/storage"} {
		r := httptest.NewRequest(http.MethodGet, path, strings.NewReader(""))
		r.Header.Set("Authorization", "Bearer session")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound || service.reads != 0 {
			t.Fatalf("retired %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}
