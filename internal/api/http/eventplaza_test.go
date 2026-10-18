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
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/eventplaza"
)

type mockEventPlazaService struct {
	recordPresenceFn           func(ctx context.Context, characterID string) error
	getPlazaStatusFn           func(ctx context.Context) (eventplaza.PlazaStatus, error)
	listAvailableBazaarItemsFn func(ctx context.Context) ([]eventplaza.BazaarItem, int, error)
	purchaseBazaarItemFn       func(ctx context.Context, characterID string, itemID string, quantity int) (eventplaza.BazaarPurchaseResult, error)
}

func (m *mockEventPlazaService) RecordPresence(ctx context.Context, characterID string) error {
	if m.recordPresenceFn != nil {
		return m.recordPresenceFn(ctx, characterID)
	}
	return nil
}

func (m *mockEventPlazaService) GetPlazaStatus(ctx context.Context) (eventplaza.PlazaStatus, error) {
	if m.getPlazaStatusFn != nil {
		return m.getPlazaStatusFn(ctx)
	}
	return eventplaza.PlazaStatus{}, nil
}

func (m *mockEventPlazaService) ListAvailableBazaarItems(ctx context.Context) ([]eventplaza.BazaarItem, int, error) {
	if m.listAvailableBazaarItemsFn != nil {
		return m.listAvailableBazaarItemsFn(ctx)
	}
	return []eventplaza.BazaarItem{}, 0, nil
}

func (m *mockEventPlazaService) PurchaseBazaarItem(ctx context.Context, characterID string, itemID string, quantity int) (eventplaza.BazaarPurchaseResult, error) {
	if m.purchaseBazaarItemFn != nil {
		return m.purchaseBazaarItemFn(ctx, characterID, itemID, quantity)
	}
	return eventplaza.BazaarPurchaseResult{}, nil
}

func createTestHandler(opts ...apihttp.Option) (*apihttp.Handler, error) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}
	otherChar := corecharacter.Character{ID: "char-2", PlayerID: "other-player", Name: "OtherHero"}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	characters := &stubCharacterService{
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

	return apihttp.NewHandler(
		players,
		characters,
		&stubAdventureService{},
		&stubShopService{},
		opts...,
	)
}

func TestHandleGetEventPlaza(t *testing.T) {
	mockSvc := &mockEventPlazaService{
		getPlazaStatusFn: func(ctx context.Context) (eventplaza.PlazaStatus, error) {
			return eventplaza.PlazaStatus{
				ActiveParticipants:     7,
				MerchantSelectionCount: 25,
				MerchantTier:           2,
				MerchantTierName:       "Silver Traveling Merchant (熟練の行商人バザー)",
				NextTierThreshold:      30,
			}, nil
		},
	}

	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/eventplaza", nil)
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res eventplaza.PlazaStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.ActiveParticipants != 7 || res.MerchantSelectionCount != 25 || res.MerchantTier != 2 {
		t.Errorf("unexpected plaza status: %+v", res)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"active_banquets", "active_banquets_count"} {
		if _, exists := fields[name]; exists {
			t.Errorf("retired field %q remains in plaza status", name)
		}
	}
}

func TestHandleGetEventPlazaMerchantItems(t *testing.T) {
	mockSvc := &mockEventPlazaService{
		listAvailableBazaarItemsFn: func(ctx context.Context) ([]eventplaza.BazaarItem, int, error) {
			return []eventplaza.BazaarItem{
				{
					ID:           "bazaar_herb_extract",
					Name:         "名薬草のエキス",
					Price:        500,
					TierRequired: 1,
				},
			}, 1, nil
		},
	}

	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/eventplaza/merchant/items", nil)
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res struct {
		MerchantTier int                     `json:"merchant_tier"`
		Items        []eventplaza.BazaarItem `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.MerchantTier != 1 || len(res.Items) != 1 {
		t.Errorf("unexpected response: %+v", res)
	}
}

func TestHandlePostEventPlazaMerchantPurchase(t *testing.T) {
	mockSvc := &mockEventPlazaService{
		purchaseBazaarItemFn: func(ctx context.Context, characterID string, itemID string, quantity int) (eventplaza.BazaarPurchaseResult, error) {
			if itemID == "locked_item" {
				return eventplaza.BazaarPurchaseResult{}, eventplaza.ErrItemTierLocked
			}
			if itemID == "helper_item" {
				return eventplaza.BazaarPurchaseResult{}, eventplaza.ErrItemUnavailable
			}
			if itemID == "depot_full_item" {
				return eventplaza.BazaarPurchaseResult{}, eventplaza.ErrDepotFull
			}
			return eventplaza.BazaarPurchaseResult{
				CharacterID:         characterID,
				Quantity:            quantity,
				TotalPrice:          4800,
				RemainingGold:       5000,
				InventoryInstanceID: "inst-123",
				TransferredToDepot:  false,
			}, nil
		},
	}

	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// 1. Success with valid auth
	body, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
		"item_id":      "item-072",
		"quantity":     2,
	})
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-session")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Locked item -> 400
	lockedBody, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
		"item_id":      "locked_item",
		"quantity":     1,
	})
	reqLocked := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", bytes.NewReader(lockedBody))
	reqLocked.Header.Set("Content-Type", "application/json")
	reqLocked.Header.Set("Authorization", "Bearer valid-session")
	recLocked := httptest.NewRecorder()
	handler.Router().ServeHTTP(recLocked, reqLocked)

	if recLocked.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", recLocked.Code)
	}

	// 3. Helper quest unavailable item -> 409
	helperBody, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
		"item_id":      "helper_item",
		"quantity":     1,
	})
	reqHelper := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", bytes.NewReader(helperBody))
	reqHelper.Header.Set("Content-Type", "application/json")
	reqHelper.Header.Set("Authorization", "Bearer valid-session")
	recHelper := httptest.NewRecorder()
	handler.Router().ServeHTTP(recHelper, reqHelper)

	if recHelper.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", recHelper.Code)
	}

	// 4. Depot full -> 409
	depotBody, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
		"item_id":      "depot_full_item",
		"quantity":     1,
	})
	reqDepot := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", bytes.NewReader(depotBody))
	reqDepot.Header.Set("Content-Type", "application/json")
	reqDepot.Header.Set("Authorization", "Bearer valid-session")
	recDepot := httptest.NewRecorder()
	handler.Router().ServeHTTP(recDepot, reqDepot)

	if recDepot.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", recDepot.Code)
	}
}

func TestHandlePostEventPlazaPresence(t *testing.T) {
	recordedID := ""
	mockSvc := &mockEventPlazaService{
		recordPresenceFn: func(ctx context.Context, characterID string) error {
			recordedID = characterID
			return nil
		},
	}
	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/presence", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-session")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if recordedID != "char-1" {
		t.Errorf("expected recorded presence for char-1, got %s", recordedID)
	}
}

func TestEventPlaza_MerchantPurchase_Unauthenticated_Returns401(t *testing.T) {
	mockSvc := &mockEventPlazaService{}
	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
		"item_id":      "item-072",
		"quantity":     1,
	})
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", rec.Code)
	}
}

func TestEventPlaza_MerchantPurchase_ForbiddenCharacter_Returns403(t *testing.T) {
	mockSvc := &mockEventPlazaService{}
	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"character_id": "char-2", // belongs to other-player
		"item_id":      "item-072",
		"quantity":     1,
	})
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-session")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden, got %d", rec.Code)
	}
}
