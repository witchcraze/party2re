package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
)

type stubDepotService struct {
	getDepotFn     func(ctx context.Context, characterID string) (depot.Depot, error)
	depositItemFn  func(ctx context.Context, characterID, itemID string) (depot.Depot, error)
	withdrawItemFn func(ctx context.Context, characterID, itemID string) (depot.Depot, error)
	sellItemFn     func(ctx context.Context, characterID, itemID string) (depot.Depot, int, error)
	sellItemsFn    func(ctx context.Context, characterID string, itemIDs []string) (depot.Depot, int, error)
	sortItemsFn    func(ctx context.Context, characterID string) (depot.Depot, error)
	expandFn       func(ctx context.Context, characterID string) (depot.Depot, error)
	sendMoneyFn    func(ctx context.Context, fromID, toID string, amount int) (depot.Depot, error)
	sendItemFn     func(ctx context.Context, fromID, toID, itemID string) (depot.Depot, error)
}

func (s *stubDepotService) GetDepot(ctx context.Context, characterID string) (depot.Depot, error) {
	if s.getDepotFn != nil {
		return s.getDepotFn(ctx, characterID)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 50}, nil
}

func (s *stubDepotService) DepositItem(ctx context.Context, characterID, itemID string) (depot.Depot, error) {
	if s.depositItemFn != nil {
		return s.depositItemFn(ctx, characterID, itemID)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 50}, nil
}

func (s *stubDepotService) WithdrawItem(ctx context.Context, characterID, itemID string) (depot.Depot, error) {
	if s.withdrawItemFn != nil {
		return s.withdrawItemFn(ctx, characterID, itemID)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 50}, nil
}

func (s *stubDepotService) SellItem(ctx context.Context, characterID, itemID string) (depot.Depot, int, error) {
	if s.sellItemFn != nil {
		return s.sellItemFn(ctx, characterID, itemID)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 50}, 250, nil
}

func (s *stubDepotService) SellItems(ctx context.Context, characterID string, itemIDs []string) (depot.Depot, int, error) {
	if s.sellItemsFn != nil {
		return s.sellItemsFn(ctx, characterID, itemIDs)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 50}, 500, nil
}

func (s *stubDepotService) SortItems(ctx context.Context, characterID string) (depot.Depot, error) {
	if s.sortItemsFn != nil {
		return s.sortItemsFn(ctx, characterID)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 50}, nil
}

func (s *stubDepotService) Expand(ctx context.Context, characterID string) (depot.Depot, error) {
	if s.expandFn != nil {
		return s.expandFn(ctx, characterID)
	}
	return depot.Depot{CharacterID: characterID, Capacity: 55, ExDepot: 1}, nil
}

func (s *stubDepotService) SendMoney(ctx context.Context, fromID, toID string, amount int) (depot.Depot, error) {
	if s.sendMoneyFn != nil {
		return s.sendMoneyFn(ctx, fromID, toID, amount)
	}
	return depot.Depot{CharacterID: fromID, Capacity: 50}, nil
}

func (s *stubDepotService) SendItem(ctx context.Context, fromID, toID, itemID string) (depot.Depot, error) {
	if s.sendItemFn != nil {
		return s.sendItemFn(ctx, fromID, toID, itemID)
	}
	return depot.Depot{CharacterID: fromID, Capacity: 50}, nil
}

func TestDepotEndpoints(t *testing.T) {
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
	depotSvc := &stubDepotService{}

	h, err := apihttp.NewHandler(pService, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithDepot(depotSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("GET /characters/{id}/depot success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/depot", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["character_id"] != "c1" {
			t.Errorf("expected character_id c1, got %v", resp["character_id"])
		}
	})

	t.Run("POST /characters/{id}/depot/deposit success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"item_id": "item-1"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/deposit", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/depot/deposit full error", func(t *testing.T) {
		depotSvc.depositItemFn = func(ctx context.Context, characterID, itemID string) (depot.Depot, error) {
			return depot.Depot{}, depot.ErrDepotFull
		}
		defer func() { depotSvc.depositItemFn = nil }()

		body, _ := json.Marshal(map[string]string{"item_id": "item-1"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/deposit", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/depot/withdraw success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"item_id": "item-1"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/withdraw", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/depot/sell success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"item_id": "item-1"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/sell", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if int(resp["gold_earned"].(float64)) != 250 {
			t.Errorf("expected 250 gold_earned, got %v", resp["gold_earned"])
		}
	})

	t.Run("POST /characters/{id}/depot/sell-batch success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"item_ids": []string{"item-1", "item-2"}})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/sell-batch", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/depot/sort success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/sort", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/depot/expand success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/expand", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if int(resp["capacity"].(float64)) != 55 {
			t.Errorf("expected 55 capacity, got %v", resp["capacity"])
		}
	})

	t.Run("POST /characters/{id}/depot/send-money success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"recipient_character_id": "c2", "amount": 1000})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/send-money", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/depot/send-item success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"recipient_character_id": "c2", "item_id": "item-1"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/depot/send-item", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Unauthenticated returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/depot", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("Forbidden character returns 403", func(t *testing.T) {
		pServiceOther := &stubPlayerService{
			authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p-other", Username: "other"}),
		}
		hOther, _ := apihttp.NewHandler(pServiceOther, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithDepot(depotSvc))
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/depot", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		hOther.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", rec.Code)
		}
	})
}
