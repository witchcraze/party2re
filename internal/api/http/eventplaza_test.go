package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/eventplaza"
)

type mockEventPlazaService struct {
	getPlazaStatusFn           func(ctx context.Context) (eventplaza.PlazaStatus, error)
	listAvailableBazaarItemsFn func(ctx context.Context) ([]eventplaza.BazaarItem, int, error)
	purchaseBazaarItemFn       func(ctx context.Context, characterID string, itemID string, quantity int) (eventplaza.BazaarPurchaseResult, error)
	listActiveBanquetsFn       func(ctx context.Context) ([]eventplaza.CelebrationBanquet, error)
	toastBanquetFn             func(ctx context.Context, banquetID string, characterID string) (eventplaza.BanquetToastResult, error)
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

func (m *mockEventPlazaService) ListActiveBanquets(ctx context.Context) ([]eventplaza.CelebrationBanquet, error) {
	if m.listActiveBanquetsFn != nil {
		return m.listActiveBanquetsFn(ctx)
	}
	return []eventplaza.CelebrationBanquet{}, nil
}

func (m *mockEventPlazaService) ToastBanquet(ctx context.Context, banquetID string, characterID string) (eventplaza.BanquetToastResult, error) {
	if m.toastBanquetFn != nil {
		return m.toastBanquetFn(ctx, banquetID, characterID)
	}
	return eventplaza.BanquetToastResult{}, nil
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
				ActiveParticipants:  25,
				MerchantTier:        2,
				MerchantTierName:    "Silver Traveling Merchant (熟練の行商人バザー)",
				NextTierThreshold:   30,
				ActiveBanquetsCount: 1,
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
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.ActiveParticipants != 25 || res.MerchantTier != 2 {
		t.Errorf("unexpected plaza status: %+v", res)
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
			return eventplaza.BazaarPurchaseResult{
				CharacterID:         characterID,
				Quantity:            quantity,
				TotalPrice:          1000,
				RemainingGold:       5000,
				InventoryInstanceID: "inst-123",
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
		"item_id":      "bazaar_herb_extract",
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
}

func TestEventPlaza_MerchantPurchase_Unauthenticated_Returns401(t *testing.T) {
	mockSvc := &mockEventPlazaService{}
	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"character_id": "char-1",
		"item_id":      "bazaar_herb_extract",
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
		"item_id":      "bazaar_herb_extract",
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

func TestHandleGetEventPlazaBanquets(t *testing.T) {
	now := time.Now().UTC()
	mockSvc := &mockEventPlazaService{
		listActiveBanquetsFn: func(ctx context.Context) ([]eventplaza.CelebrationBanquet, error) {
			return []eventplaza.CelebrationBanquet{
				{
					ID:                  "banquet-1",
					BossName:            "Flame Sovereign",
					SlayerCharacterName: "Hero",
					Tier:                1,
					ToastCount:          5,
					CelebratedAt:        now,
					ExpiresAt:           now.Add(24 * time.Hour),
				},
			}, nil
		},
	}

	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/eventplaza/banquets", nil)
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var res struct {
		Banquets []eventplaza.CelebrationBanquet `json:"banquets"`
		Total    int                             `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Total != 1 || res.Banquets[0].BossName != "Flame Sovereign" {
		t.Errorf("unexpected banquets response: %+v", res)
	}
}

func TestHandlePostEventPlazaBanquetToast(t *testing.T) {
	mockSvc := &mockEventPlazaService{
		toastBanquetFn: func(ctx context.Context, banquetID string, characterID string) (eventplaza.BanquetToastResult, error) {
			if banquetID == "already_toasted" {
				return eventplaza.BanquetToastResult{}, eventplaza.ErrAlreadyToasted
			}
			if banquetID == "expired" {
				return eventplaza.BanquetToastResult{}, eventplaza.ErrBanquetExpired
			}
			if banquetID == "not_found" {
				return eventplaza.BanquetToastResult{}, eventplaza.ErrBanquetNotFound
			}
			return eventplaza.BanquetToastResult{
				BanquetID:            banquetID,
				CharacterID:          characterID,
				GoldAwarded:          300,
				CurrentCharacterGold: 1300,
				ToastCount:           6,
				Message:              "乾杯しました！",
			}, nil
		},
	}

	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// 1. Success with valid auth
	body, _ := json.Marshal(map[string]any{"character_id": "char-1"})
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/banquet-1/toast", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-session")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Already toasted -> 409 Conflict
	reqConflict := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/already_toasted/toast", bytes.NewReader(body))
	reqConflict.Header.Set("Content-Type", "application/json")
	reqConflict.Header.Set("Authorization", "Bearer valid-session")
	recConflict := httptest.NewRecorder()
	handler.Router().ServeHTTP(recConflict, reqConflict)
	if recConflict.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", recConflict.Code)
	}

	// 3. Expired -> 410 Gone
	reqExpired := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/expired/toast", bytes.NewReader(body))
	reqExpired.Header.Set("Content-Type", "application/json")
	reqExpired.Header.Set("Authorization", "Bearer valid-session")
	recExpired := httptest.NewRecorder()
	handler.Router().ServeHTTP(recExpired, reqExpired)
	if recExpired.Code != http.StatusGone {
		t.Errorf("expected status 410, got %d", recExpired.Code)
	}

	// 4. Not Found -> 404
	reqNotFound := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/not_found/toast", bytes.NewReader(body))
	reqNotFound.Header.Set("Content-Type", "application/json")
	reqNotFound.Header.Set("Authorization", "Bearer valid-session")
	recNotFound := httptest.NewRecorder()
	handler.Router().ServeHTTP(recNotFound, reqNotFound)
	if recNotFound.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", recNotFound.Code)
	}
}

func TestEventPlaza_BanquetToast_Unauthenticated_Returns401(t *testing.T) {
	mockSvc := &mockEventPlazaService{}
	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"character_id": "char-1"})
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/banquet-1/toast", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", rec.Code)
	}
}

func TestEventPlaza_BanquetToast_ForbiddenCharacter_Returns403(t *testing.T) {
	mockSvc := &mockEventPlazaService{}
	handler, err := createTestHandler(apihttp.WithEventPlaza(mockSvc))
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"character_id": "char-2"}) // belongs to other-player
	req := httptest.NewRequest(http.MethodPost, "/eventplaza/banquets/banquet-1/toast", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-session")
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden, got %d", rec.Code)
	}
}
