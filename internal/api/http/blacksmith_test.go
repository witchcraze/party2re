package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubBlacksmithService struct {
	apihttp.BlacksmithService
	listDepositsFn func(context.Context, string) ([]blacksmith.Deposit, error)
}

func (s *stubBlacksmithService) ListDeposits(ctx context.Context, characterID string) ([]blacksmith.Deposit, error) {
	if s.listDepositsFn != nil {
		return s.listDepositsFn(ctx, characterID)
	}
	return []blacksmith.Deposit{
		{
			ID:               "dep-1",
			CharacterID:      characterID,
			Slot:             1,
			ItemDefinitionID: "item_sword",
		},
	}, nil
}

func TestBlacksmithEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10}

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
	bsSvc := &stubBlacksmithService{}

	h, err := apihttp.NewHandler(
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithBlacksmith(bsSvc),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("GET /blacksmith/seals returns 12 seals", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/blacksmith/seals", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Seals []blacksmith.Seal `json:"seals"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Seals) != 12 {
			t.Fatalf("expected 12 seals, got %d", len(resp.Seals))
		}
	})

	t.Run("GET /characters/{id}/blacksmith/storage success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/blacksmith/storage", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Deposits []blacksmith.Deposit `json:"deposits"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(resp.Deposits) != 1 {
			t.Fatalf("expected 1 deposit, got %d", len(resp.Deposits))
		}
	})

}
