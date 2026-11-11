package http

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type gemStoreSceneService struct {
	GemStoreService
	box          gemstore.GemBox
	boxErr       error
	catalogLevel int
	mutations    int
	recipes      []gemstore.Recipe
}

func (s *gemStoreSceneService) GetGemBox(_ context.Context, actor string) (gemstore.GemBox, error) {
	if actor != "hero" {
		return gemstore.GemBox{}, errors.New("wrong gem-box owner")
	}
	return s.box, s.boxErr
}

func (s *gemStoreSceneService) GetCatalog(jobLevel int) []gemstore.Gem {
	s.catalogLevel = jobLevel
	return []gemstore.Gem{{ID: "gem_atk_1", Price: 60}}
}

func (s *gemStoreSceneService) GetRecipes() []gemstore.Recipe {
	return s.recipes
}
func (s *gemStoreSceneService) BuyGem(context.Context, string, string) (gemstore.BuyResult, error) {
	s.mutations++
	return gemstore.BuyResult{}, nil
}

func gemStoreSceneRouter(t *testing.T, service GemStoreService, destination string) (*gatewayFixture, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "gemstore", Parent: "town"}))
	options := []Option{WithPlayerContext(pc), func(h *Handler) { h.homes = f }}
	if service != nil {
		options = append(options, WithGemStore(service))
	}
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, options...)
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

func TestGemStoreSceneObservesOwnedFactsWithoutWrites(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	service := &gemStoreSceneService{box: gemstore.GemBox{CharacterID: "hero", Capacity: 15, Items: []coreitem.Instance{{ID: "owned-gem", DefinitionID: "gem_atk_1", Quantity: 1}}}}
	service.recipes = catalog.AllRecipes()
	f, store, router := gemStoreSceneRouter(t, service, "gemstore")
	f.char.JobLevel = 30
	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.Kind != "facility" || observation.Scene.Title != "宝石店" || observation.Scene.Support.Observation != "details" {
		t.Fatalf("projection: %d %+v", status, observation)
	}
	data := decodeShopScene[GemStoreSceneData](t, observation)
	if data.Parent != "town" || data.GemBox.Capacity != 15 || len(data.GemBox.Items) != 1 || data.GemBox.Items[0].ID != "owned-gem" ||
		len(data.Catalog) != 1 || len(data.Recipes) != 56 || service.catalogLevel != 30 {
		t.Fatalf("lost facts: %+v (catalog level %d)", data, service.catalogLevel)
	}
	items, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range data.Recipes {
		gem, ok := catalog.FindGemByID(recipe.ResultID)
		if !ok || recipe.ResultName != gem.Name {
			t.Fatalf("missing result identity/name: %+v", recipe)
		}
		for _, material := range [][2]string{{recipe.Material1, recipe.Material1Name}, {recipe.Material2, recipe.Material2Name}} {
			if gem, ok := catalog.FindGemByID(material[0]); ok {
				if material[1] != gem.Name {
					t.Fatalf("missing gem name: %+v", recipe)
				}
			} else if item, err := items.FindByID(material[0]); err != nil || material[1] != item.Name {
				t.Fatalf("missing item name: %+v", recipe)
			}
		}
	}
	for _, action := range []string{"gemstore_buy", "gemstore_sell", "gemstore_send", "gemstore_synthesize", "gemstore_appraise", "gemstore_sort", "scene_back"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("missing connected choice %s: %+v", action, observation.AvailableActions)
		}
	}
	if store.writes != 0 || service.mutations != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d %d", store.writes, service.mutations, f.executions)
	}
	store.selection.Destination = "town"
	if status, town := navigationGET(t, router); status != 200 || !slices.ContainsFunc(decodeShopScene[TownSceneData](t, town).Destinations, func(d SceneDestination) bool {
		return d.ID == "gemstore" && d.Supported && d.EnterParams.Destination == "gemstore"
	}) {
		t.Fatalf("missing town entry: %d %+v", status, town)
	}
}

func TestGemStoreSceneRequiredReadFailure(t *testing.T) {
	_, store, router := gemStoreSceneRouter(t, &gemStoreSceneService{boxErr: errors.New("private gem-box failure")}, "gemstore")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil || store.writes != 0 {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
}

func TestGemStoreSceneUnconfigured(t *testing.T) {
	_, store, router := gemStoreSceneRouter(t, nil, "gemstore")
	if status, _ := navigationGET(t, router); status != http.StatusNotImplemented || store.writes != 0 {
		t.Fatalf("missing service: %d", status)
	}
}
