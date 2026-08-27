package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/medal"
)

type mockMedalService struct {
	getRewardsFn func() []medal.Reward
	claimFn      func(ctx context.Context, charID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error)
}

func (m *mockMedalService) GetRewards() []medal.Reward {
	if m.getRewardsFn != nil {
		return m.getRewardsFn()
	}
	return []medal.Reward{
		{Cost: 3, ItemID: "armor-32"},
		{Cost: 10, ItemID: "weapon-32"},
	}
}

func (m *mockMedalService) Claim(ctx context.Context, charID string, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
	if m.claimFn != nil {
		return m.claimFn(ctx, charID, itemID)
	}
	inv, _ := coreinventory.New(charID)
	return corecharacter.Character{ID: charID, SmallMedals: 2}, inv, nil
}

func TestMedalEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "MedalHero", SmallMedals: 5}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "other-player", Name: "OtherHero", SmallMedals: 5}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char, nil
			}
			if id == "char-2" {
				return otherChar, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	adv := &stubAdventureService{}
	shopSvc := &stubShopService{}
	medalSvc := &mockMedalService{}

	handler, err := apihttp.NewHandler(
		players,
		chars,
		adv,
		shopSvc,
		apihttp.WithMedal(medalSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	router := handler.Router()

	t.Run("GET /medals/rewards", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/medals/rewards", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var rewards []medal.Reward
		if err := json.Unmarshal(rec.Body.Bytes(), &rewards); err != nil {
			t.Fatalf("failed to unmarshal rewards: %v", err)
		}
		if len(rewards) != 2 {
			t.Errorf("expected 2 rewards, got %d", len(rewards))
		}
	})

	t.Run("POST /medals/claim - unauthorized", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1","item_id":"armor-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - forbidden character ownership", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-2","item_id":"armor-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - insufficient medals", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimFn: func(ctx context.Context, charID, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
				return corecharacter.Character{}, coreinventory.Inventory{}, medal.ErrInsufficientMedals
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		payload := []byte(`{"character_id":"char-1","item_id":"weapon-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - reward not found", func(t *testing.T) {
		mockSvc := &mockMedalService{
			claimFn: func(ctx context.Context, charID, itemID string) (corecharacter.Character, coreinventory.Inventory, error) {
				return corecharacter.Character{}, coreinventory.Inventory{}, medal.ErrRewardNotFound
			},
		}
		h, _ := apihttp.NewHandler(players, chars, adv, shopSvc, apihttp.WithMedal(mockSvc))
		r := h.Router()

		payload := []byte(`{"character_id":"char-1","item_id":"non-existent"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("POST /medals/claim - success", func(t *testing.T) {
		payload := []byte(`{"character_id":"char-1","item_id":"armor-32"}`)
		req := httptest.NewRequest(http.MethodPost, "/medals/claim", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-session")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})
}
