package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/blackmarket"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type blackMarketSceneService struct {
	BlackMarketService
	status    blackmarket.Status
	err       error
	denied    bool
	reads     int
	mutations int
}

func (s *blackMarketSceneService) GetStatus(_ context.Context, actor string) (*blackmarket.Status, error) {
	if actor != "hero" {
		return nil, errors.New("wrong black market owner")
	}
	s.reads++
	if s.err != nil {
		return nil, s.err
	}
	status := s.status
	return &status, nil
}

func (s *blackMarketSceneService) SacrificeItem(_ context.Context, actor, id string) (*blackmarket.SacrificeResult, error) {
	if s.denied {
		return nil, blackmarket.ErrAccessDenied
	}
	s.mutations++
	s.status.SacrificeCandidates = slices.DeleteFunc(s.status.SacrificeCandidates, func(c blackmarket.SacrificeCandidate) bool {
		return c.ItemInstanceID == id
	})
	return &blackmarket.SacrificeResult{CharacterID: actor, ItemInstanceID: id}, nil
}

func blackMarketSceneRouter(t *testing.T, service BlackMarketService, jobLevel int) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	f.char.JobLevel = jobLevel
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "blackmarket"}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "oracle", Parent: "town"},
		playercontext.SceneDefinition{ID: "blackmarket", Parent: "oracle", Pageable: true, CanEnter: blackmarket.CheckEligibility}))
	options := []Option{WithPlayerContext(pc), func(h *Handler) { h.homes = f }}
	if service != nil {
		options = append(options, WithBlackMarket(service))
	}
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func blackMarketTestStatus(candidates int) blackmarket.Status {
	status := blackmarket.Status{CharacterID: "hero", LocationName: blackmarket.LocationName, NPCName: blackmarket.NPCName, RarePoints: 3,
		Prizes: []blackmarket.Prize{{ID: "bm_prize_087", Cost: 1}}, UPrizes: []blackmarket.Prize{}, SacrificeCandidates: []blackmarket.SacrificeCandidate{}}
	for i := range candidates {
		status.SacrificeCandidates = append(status.SacrificeCandidates, blackmarket.SacrificeCandidate{ItemInstanceID: fmt.Sprintf("i%d", i), Source: blackmarket.SourceDepot, Quantity: 1, RarePoints: 1})
	}
	return status
}

func TestBlackMarketSceneObservesOwnedFactsWithoutMutation(t *testing.T) {
	service := &blackMarketSceneService{status: blackMarketTestStatus(2)}
	store, router := blackMarketSceneRouter(t, service, blackmarket.MinAccessJobLevel)
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.LocationID != "blackmarket" || observation.Scene.Title != "闇市場" || observation.Scene.Support.Observation != "details" {
		t.Fatalf("projection: %d %+v", status, observation)
	}
	data := decodeShopScene[BlackMarketSceneData](t, observation)
	if data.Parent != "oracle" || !reflect.DeepEqual(data.Status, service.status) || data.CandidateCount != 2 || data.Page.Next != nil {
		t.Fatalf("lost facts: %+v", data)
	}
	for _, action := range []string{"blackmarket_sacrifice", "blackmarket_trade", "scene_back"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("missing connected choice %s: %+v", action, observation.AvailableActions)
		}
	}
	if store.writes != 0 || service.mutations != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, service.mutations)
	}

	store.selection.Destination = "town"
	status, town := navigationGET(t, router)
	// Legacy reaches the black market only through the Oracle shop (goods.cgi ＠やみいちば).
	if status != 200 || slices.ContainsFunc(decodeShopScene[TownSceneData](t, town).Destinations, func(d SceneDestination) bool { return d.ID == "blackmarket" }) {
		t.Fatalf("black market listed in town: %d %+v", status, town)
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"blackmarket"}}`, nil)
	var refreshed PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
		t.Fatal(err)
	}
	if status != 200 || !reflect.DeepEqual(refreshed.Scene, observation.Scene) || store.writes != 1 {
		t.Fatalf("entry differs: %d %s", status, raw)
	}
}

func TestBlackMarketScenePagesCandidates(t *testing.T) {
	service := &blackMarketSceneService{status: blackMarketTestStatus(21)}
	store, router := blackMarketSceneRouter(t, service, blackmarket.MinAccessJobLevel)
	status, first := navigationGET(t, router)
	data := decodeShopScene[BlackMarketSceneData](t, first)
	if status != 200 || len(data.SacrificeCandidates) != 20 || data.CandidateCount != 21 || data.Page.Next == nil || data.Page.Next.Offset != 20 {
		t.Fatalf("first page: %d %+v", status, data)
	}
	if !slices.ContainsFunc(first.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" }) {
		t.Fatalf("missing paging choice: %+v", first.AvailableActions)
	}
	store.selection.Offset, store.selection.Limit = 20, 20
	status, second := navigationGET(t, router)
	data = decodeShopScene[BlackMarketSceneData](t, second)
	if status != 200 || len(data.SacrificeCandidates) != 1 || data.SacrificeCandidates[0].ItemInstanceID != "i20" || data.Page.Next != nil {
		t.Fatalf("second page: %d %+v", status, data)
	}
}

func TestBlackMarketSceneQualificationAndSafeBack(t *testing.T) {
	service := &blackMarketSceneService{status: blackMarketTestStatus(0), denied: true}
	store, router := blackMarketSceneRouter(t, service, blackmarket.MinAccessJobLevel-1)
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "selection_unavailable" || service.reads != 0 {
		t.Fatalf("unqualified observation: %d %+v", status, observation)
	}
	if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_back" }) {
		t.Fatalf("missing safe back: %+v", observation.AvailableActions)
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"blackmarket_sacrifice","params":{"item_instance_id":"i"}}`, nil)
	if status != 409 || service.mutations != 0 {
		t.Fatalf("unqualified command: %d %s", status, raw)
	}
	assertGatewayError(t, raw, "ACTION_UNAVAILABLE")

	store.selection.Destination = "town"
	if status, town := navigationGET(t, router); status != 200 || slices.ContainsFunc(decodeShopScene[TownSceneData](t, town).Destinations, func(d SceneDestination) bool { return d.ID == "blackmarket" }) {
		t.Fatalf("unqualified discovery: %d %+v", status, town)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"blackmarket"}}`, nil)
	if status < 400 || store.selection.Destination != "town" {
		t.Fatalf("unqualified entry: %d %s", status, raw)
	}

	// A qualification lost between the navigation read and the service read stays unavailable.
	service.err = fmt.Errorf("wrapped: %w", blackmarket.ErrAccessDenied)
	store2, qualified := blackMarketSceneRouter(t, service, blackmarket.MinAccessJobLevel)
	if status, lost := navigationGET(t, qualified); status != 200 || lost.Scene.Kind != "selection_unavailable" || store2.writes != 0 {
		t.Fatalf("lost qualification: %d %+v", status, lost)
	}
}

func TestBlackMarketSceneRequiredReadFailureAndGETRecovery(t *testing.T) {
	service := &blackMarketSceneService{status: blackMarketTestStatus(1), err: errors.New("private points read failed")}
	store, router := blackMarketSceneRouter(t, service, blackmarket.MinAccessJobLevel)
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"blackmarket_sacrifice","params":{"item_instance_id":"i0"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || service.mutations != 1 {
		t.Fatalf("lost command outcome: %d %s", status, raw)
	}
	var detail ErrorDetail
	if err := json.Unmarshal(raw["context_error"], &detail); err != nil || detail.Code != "CONTEXT_REFRESH_FAILED" {
		t.Fatalf("refresh failure: %s %v", raw, err)
	}
	service.err = nil
	if status, _ := navigationGET(t, router); status != 200 || service.mutations != 1 || store.writes != 0 {
		t.Fatalf("GET recovery replayed: %d", status)
	}
}

func TestBlackMarketSceneUnconfigured(t *testing.T) {
	store, router := blackMarketSceneRouter(t, nil, blackmarket.MinAccessJobLevel)
	if status, _ := navigationGET(t, router); status != http.StatusNotImplemented || store.writes != 0 {
		t.Fatalf("missing service: %d", status)
	}
}

func TestBlackMarketSceneEquippedCandidateAndSacrificeRefresh(t *testing.T) {
	statusFixture := blackMarketTestStatus(0)
	statusFixture.SacrificeCandidates = []blackmarket.SacrificeCandidate{
		{ItemInstanceID: "eq-1", ItemDefinitionID: "item-eq", ItemName: "Equipped Rare", Source: blackmarket.SourceInventory, Equipped: true, Quantity: 1, RarePoints: 1},
		{ItemInstanceID: "dep-1", ItemDefinitionID: "item-dep", ItemName: "Depot Rare", Source: blackmarket.SourceDepot, Equipped: false, Quantity: 1, RarePoints: 1},
	}
	service := &blackMarketSceneService{status: statusFixture}
	_, router := blackMarketSceneRouter(t, service, blackmarket.MinAccessJobLevel)

	httpStatus, observation := navigationGET(t, router)
	data := decodeShopScene[BlackMarketSceneData](t, observation)
	if httpStatus != 200 || len(data.SacrificeCandidates) != 2 || !data.SacrificeCandidates[0].Equipped || data.SacrificeCandidates[0].ItemInstanceID != "eq-1" {
		t.Fatalf("unexpected initial candidates: %d %+v", httpStatus, data.SacrificeCandidates)
	}

	httpStatus, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"blackmarket_sacrifice","params":{"item_instance_id":"eq-1"}}`, nil)
	if httpStatus != 200 || string(raw["success"]) != "true" || service.mutations != 1 {
		t.Fatalf("sacrifice action: %d %s", httpStatus, raw)
	}
	var refreshed struct {
		Scene struct {
			Data BlackMarketSceneData `json:"data"`
		} `json:"scene"`
	}
	if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
		t.Fatalf("decode refreshed context: %v (%s)", err, raw["context"])
	}
	if len(refreshed.Scene.Data.SacrificeCandidates) != 1 || refreshed.Scene.Data.SacrificeCandidates[0].ItemInstanceID != "dep-1" {
		t.Fatalf("expected sacrificed candidate removed from scene context, got %+v", refreshed.Scene.Data.SacrificeCandidates)
	}
}
