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

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type alchemySceneService struct {
	AlchemyService
	status             alchemy.Synthesis
	compendium         alchemy.Compendium
	statusErr, readErr error
	claimErr           error
	claims             int
}

func (s *alchemySceneService) GetStatus(_ context.Context, actor string) (alchemy.Synthesis, error) {
	if actor != "hero" {
		return alchemy.Synthesis{}, errors.New("wrong alchemy owner")
	}
	return s.status, s.statusErr
}

func (s *alchemySceneService) GetCompendium(_ context.Context, actor string) (alchemy.Compendium, error) {
	if actor != "hero" {
		return alchemy.Compendium{}, errors.New("wrong alchemy owner")
	}
	return s.compendium, s.readErr
}

func (s *alchemySceneService) Claim(_ context.Context, actor string) (alchemy.ClaimResult, error) {
	if actor != "hero" {
		return alchemy.ClaimResult{}, errors.New("wrong alchemy owner")
	}
	s.claims++
	if s.claimErr != nil {
		return alchemy.ClaimResult{}, s.claimErr
	}
	return alchemy.ClaimResult{TotalCrafts: 1, Message: "delivered"}, nil
}

func alchemyTestService(entries int) *alchemySceneService {
	s := &alchemySceneService{status: alchemy.Synthesis{CharacterID: "hero", State: alchemy.StateNone},
		compendium: alchemy.Compendium{TotalRecipes: 114, LearnedCount: entries, Entries: []alchemy.CompendiumEntry{}}}
	for i := range entries {
		s.compendium.Entries = append(s.compendium.Entries, alchemy.CompendiumEntry{RecipeID: fmt.Sprintf("r%d", i), Ingredients: []alchemy.Ingredient{}, IsLearned: true})
	}
	return s
}

func alchemySceneRouter(t *testing.T, service AlchemyService, destination string) (*gatewayFixture, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "alchemy", Parent: "town", Pageable: true}))
	options := []Option{WithPlayerContext(pc), func(h *Handler) { h.homes = f }}
	if service != nil {
		options = append(options, WithAlchemy(service))
	}
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

func TestAlchemySceneObservesOwnedFactsWithoutClaiming(t *testing.T) {
	service := alchemyTestService(1)
	// Even a completed synthesis stays unclaimed on GET.
	service.status.State, service.status.RecipeID = alchemy.StateCompleted, "r0"
	_, store, router := alchemySceneRouter(t, service, "alchemy")
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.Title != "錬金場" || observation.Scene.Support.Observation != "details" {
		t.Fatalf("projection: %d %+v", status, observation)
	}
	data := decodeShopScene[AlchemySceneData](t, observation)
	if data.Parent != "town" || !reflect.DeepEqual(data.Status, service.status) || data.Compendium.TotalRecipes != 114 || data.Page.Next != nil {
		t.Fatalf("lost facts: %+v", data)
	}
	for _, action := range []string{"alchemy_synthesize", "scene_back"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("missing connected choice %s: %+v", action, observation.AvailableActions)
		}
	}
	if store.writes != 0 || service.claims != 0 {
		t.Fatalf("GET wrote or claimed: %d %d", store.writes, service.claims)
	}
	store.selection.Destination = "town"
	if status, town := navigationGET(t, router); status != 200 || !slices.ContainsFunc(decodeShopScene[TownSceneData](t, town).Destinations, func(d SceneDestination) bool {
		return d.ID == "alchemy" && d.Supported && d.EnterParams.Destination == "alchemy"
	}) {
		t.Fatalf("missing town entry: %d %+v", status, town)
	}
}

func TestAlchemySceneHidesUncraftedProduct(t *testing.T) {
	service := alchemyTestService(0)
	service.compendium.Entries = []alchemy.CompendiumEntry{
		{RecipeID: "known", Name: "上薬草の合成", ResultItemDefinitionID: "item-002", ResultItemName: "上薬草", Ingredients: []alchemy.Ingredient{}, IsLearned: true, IsCrafted: true},
		{RecipeID: "unknown", Ingredients: []alchemy.Ingredient{}, IsLearned: true},
	}
	_, _, router := alchemySceneRouter(t, service, "alchemy")
	_, observation := navigationGET(t, router)
	data := decodeShopScene[struct {
		Compendium struct {
			Entries []map[string]any `json:"entries"`
		} `json:"compendium"`
	}](t, observation)
	if len(data.Compendium.Entries) != 2 {
		t.Fatalf("entries: %+v", data)
	}
	for _, key := range []string{"name", "result_item_definition_id", "result_item_name"} {
		if _, ok := data.Compendium.Entries[0][key]; !ok {
			t.Errorf("crafted entry must expose %q", key)
		}
		if _, ok := data.Compendium.Entries[1][key]; ok {
			t.Errorf("uncrafted entry must not expose %q", key)
		}
	}
}

func TestAlchemyScenePagesEntries(t *testing.T) {
	_, store, router := alchemySceneRouter(t, alchemyTestService(21), "alchemy")
	status, first := navigationGET(t, router)
	data := decodeShopScene[AlchemySceneData](t, first)
	if status != 200 || len(data.Compendium.Entries) != 20 || data.Compendium.LearnedCount != 21 || data.Page.Next == nil || data.Page.Next.Offset != 20 {
		t.Fatalf("first page: %d %+v", status, data)
	}
	if !slices.ContainsFunc(first.AvailableActions, func(a ContextAction) bool { return a.Action == "scene_page" }) {
		t.Fatalf("missing paging choice: %+v", first.AvailableActions)
	}
	store.selection.Offset, store.selection.Limit = 20, 20
	status, second := navigationGET(t, router)
	data = decodeShopScene[AlchemySceneData](t, second)
	if status != 200 || len(data.Compendium.Entries) != 1 || data.Compendium.Entries[0].RecipeID != "r20" || data.Page.Next != nil {
		t.Fatalf("second page: %d %+v", status, data)
	}
}

func TestAlchemySceneRequiredReadFailures(t *testing.T) {
	for name, set := range map[string]func(*alchemySceneService){
		"status":     func(s *alchemySceneService) { s.statusErr = errors.New("private status failure") },
		"compendium": func(s *alchemySceneService) { s.readErr = errors.New("private compendium failure") },
	} {
		t.Run(name, func(t *testing.T) {
			service := alchemyTestService(1)
			set(service)
			_, _, router := alchemySceneRouter(t, service, "alchemy")
			if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
				t.Fatalf("partial observation: %d %+v", status, observation)
			}
		})
	}
}

func TestAlchemySceneUnconfigured(t *testing.T) {
	_, store, router := alchemySceneRouter(t, nil, "alchemy")
	if status, _ := navigationGET(t, router); status != http.StatusNotImplemented || store.writes != 0 {
		t.Fatalf("missing service: %d", status)
	}
}

func enterScene(t *testing.T, router http.Handler, destination string) (int, map[string]json.RawMessage) {
	t.Helper()
	return gatewayRequest(t, router, "hero", "session", "application/json", fmt.Sprintf(`{"action":"scene_enter","params":{"destination":%q}}`, destination), nil)
}

func TestAlchemyEntryDelivery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		claimErr  error
		claimed   bool
		errorCode string
	}{
		{name: "completed", claimed: true},
		{name: "nothing started", claimErr: alchemy.ErrNoActiveSynthesis},
		{name: "unfinished", claimErr: fmt.Errorf("wrapped: %w", alchemy.ErrSynthesisNotReady)},
		{name: "full depot keeps entry", claimErr: fmt.Errorf("wrapped: %w", depot.ErrDepotFull), errorCode: "ALCHEMY_DEPOT_FULL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := alchemyTestService(0)
			service.claimErr = tc.claimErr
			_, store, router := alchemySceneRouter(t, service, "town")
			status, got := enterScene(t, router, "alchemy")
			if status != 200 || service.claims != 1 || store.selection.Destination != "alchemy" {
				t.Fatalf("entry: %d %s", status, got)
			}
			var result sceneEnterResult
			if err := json.Unmarshal(got["result"], &result); err != nil {
				t.Fatal(err)
			}
			if result.Destination != "alchemy" || (result.AlchemyClaim != nil) != tc.claimed {
				t.Fatalf("result: %s", got["result"])
			}
			if tc.claimed && result.AlchemyClaim.Message != "delivered" {
				t.Fatalf("lost claim facts: %s", got["result"])
			}
			if (result.AlchemyClaimError == nil) != (tc.errorCode == "") || (tc.errorCode != "" && result.AlchemyClaimError.Code != tc.errorCode) {
				t.Fatalf("claim rejection: %s", got["result"])
			}
			assertGatewayContext(t, got)
		})
	}
}

func TestAlchemyEntryUnexpectedClaimFailureIsUnknown(t *testing.T) {
	service := alchemyTestService(0)
	service.claimErr = errors.New("private claim failure")
	_, _, router := alchemySceneRouter(t, service, "town")
	status, got := enterScene(t, router, "alchemy")
	assertGatewayError(t, got, "EXECUTION_FAILED")
	if _, exists := got["context"]; status != 500 || exists || strings.Contains(fmt.Sprint(got), "private") {
		t.Fatalf("unknown outcome: %d %s", status, got)
	}
	if status, _ := navigationGET(t, router); status != 200 || service.claims != 1 {
		t.Fatalf("GET recovery claimed: %d %d", status, service.claims)
	}
}

func TestAlchemyEntryOnlyClaimsForAcceptedAlchemyEntry(t *testing.T) {
	service := alchemyTestService(0)
	_, _, router := alchemySceneRouter(t, service, "town")
	if status, got := enterScene(t, router, "bank"); status >= 400 && status != 501 || service.claims != 0 {
		t.Fatalf("other destination claimed: %d %s %d", status, got, service.claims)
	}

	f, _, busy := alchemySceneRouter(t, service, "town")
	f.actions = []scheduling.ScheduledAction{{ID: "work", ActorID: "hero", State: scheduling.StateProcessing, ActionType: "training"}}
	if status, got := enterScene(t, busy, "alchemy"); status < 400 || service.claims != 0 {
		t.Fatalf("rejected entry claimed: %d %s %d", status, got, service.claims)
	}

	store := &gatewayNavigationStore{saveErr: errors.New("private selection failure"), selection: playercontext.Selection{Destination: "town"}}
	g := newGatewayFixture(t)
	pc := playercontext.NewService(g, g, timer.NewService(nil), playercontext.WithNavigation(store, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "alchemy", Parent: "town"}))
	h, err := NewHandler(g, g, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithPlayerContext(pc), WithAlchemy(service), func(h *Handler) { h.homes = g })
	if err != nil {
		t.Fatal(err)
	}
	if status, got := enterScene(t, h.Router(), "alchemy"); status != 500 || service.claims != 0 {
		t.Fatalf("failed selection claimed: %d %s %d", status, got, service.claims)
	}
}
