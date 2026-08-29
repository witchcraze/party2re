package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/collection"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubCollectionService struct {
	getMonsterBookFn    func(ctx context.Context, characterID string) ([]collection.MonsterBookEntry, collection.CompletionProgress, error)
	getItemCollectionFn func(ctx context.Context, characterID, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error)
}

func (s *stubCollectionService) GetMonsterBook(ctx context.Context, characterID string) ([]collection.MonsterBookEntry, collection.CompletionProgress, error) {
	if s.getMonsterBookFn != nil {
		return s.getMonsterBookFn(ctx, characterID)
	}
	return nil, collection.CompletionProgress{}, nil
}

func (s *stubCollectionService) GetItemCollection(ctx context.Context, characterID, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
	if s.getItemCollectionFn != nil {
		return s.getItemCollectionFn(ctx, characterID, category)
	}
	return nil, collection.CompletionProgress{}, nil
}

func TestCollectionEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	colService := &stubCollectionService{
		getMonsterBookFn: func(_ context.Context, characterID string) ([]collection.MonsterBookEntry, collection.CompletionProgress, error) {
			return []collection.MonsterBookEntry{
				{MonsterID: "m1", MonsterName: "Slime", DefeatedCount: 5},
			}, collection.CompletionProgress{DiscoveredCount: 1, TotalCatalogCount: 10, CompletionPercentage: 10.0}, nil
		},
		getItemCollectionFn: func(_ context.Context, characterID, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
			return []collection.ItemCollectionEntry{
				{ItemID: "i1", ItemName: "Herb", Category: "consumable"},
			}, collection.CompletionProgress{DiscoveredCount: 1, TotalCatalogCount: 5, CompletionPercentage: 20.0}, nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithCollection(colService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/collections/monsters - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/collections/monsters", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /characters/{id}/collections/items - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/collections/items?category=consumable", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
