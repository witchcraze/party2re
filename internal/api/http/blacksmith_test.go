package http_test

import (
	"bytes"
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
	applySealFn      func(ctx context.Context, characterID string, sealID int) (blacksmith.Seal, error)
	nameEquipmentFn  func(ctx context.Context, characterID string, target string, customName string) error
	depositWeaponFn  func(ctx context.Context, characterID string) (blacksmith.Deposit, error)
	withdrawWeaponFn func(ctx context.Context, characterID string, slot int) error
	listDepositsFn   func(ctx context.Context, characterID string) ([]blacksmith.Deposit, error)
}

func (s *stubBlacksmithService) ApplySeal(ctx context.Context, characterID string, sealID int) (blacksmith.Seal, error) {
	if s.applySealFn != nil {
		return s.applySealFn(ctx, characterID, sealID)
	}
	return blacksmith.Seals[0], nil
}

func (s *stubBlacksmithService) NameEquipment(ctx context.Context, characterID string, target string, customName string) error {
	if s.nameEquipmentFn != nil {
		return s.nameEquipmentFn(ctx, characterID, target, customName)
	}
	return nil
}

func (s *stubBlacksmithService) DepositWeapon(ctx context.Context, characterID string) (blacksmith.Deposit, error) {
	if s.depositWeaponFn != nil {
		return s.depositWeaponFn(ctx, characterID)
	}
	return blacksmith.Deposit{
		ID:               "dep-1",
		CharacterID:      characterID,
		Slot:             1,
		ItemDefinitionID: "item_sword",
	}, nil
}

func (s *stubBlacksmithService) WithdrawWeapon(ctx context.Context, characterID string, slot int) error {
	if s.withdrawWeaponFn != nil {
		return s.withdrawWeaponFn(ctx, characterID, slot)
	}
	return nil
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

	t.Run("POST /characters/{id}/blacksmith/seal success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"seal_id": 1})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/seal", bytes.NewReader(body))
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
		if resp["seal"] == nil {
			t.Error("expected seal in response")
		}
	})

	t.Run("POST /characters/{id}/blacksmith/seal insufficient crystals returns 400", func(t *testing.T) {
		bsSvc.applySealFn = func(ctx context.Context, characterID string, sealID int) (blacksmith.Seal, error) {
			return blacksmith.Seal{}, blacksmith.ErrInsufficientCrystals
		}
		defer func() { bsSvc.applySealFn = nil }()

		body, _ := json.Marshal(map[string]any{"seal_id": 1})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/seal", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/blacksmith/name success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"target": "weapon", "name": "Excalibur"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/name", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/blacksmith/name validation error returns 400", func(t *testing.T) {
		bsSvc.nameEquipmentFn = func(ctx context.Context, characterID string, target string, customName string) error {
			return blacksmith.ErrNameForbiddenChar
		}
		defer func() { bsSvc.nameEquipmentFn = nil }()

		body, _ := json.Marshal(map[string]string{"target": "weapon", "name": "bad;name"})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/name", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
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

	t.Run("POST /characters/{id}/blacksmith/storage/deposit success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/storage/deposit", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/blacksmith/storage/deposit full returns 400", func(t *testing.T) {
		bsSvc.depositWeaponFn = func(ctx context.Context, characterID string) (blacksmith.Deposit, error) {
			return blacksmith.Deposit{}, blacksmith.ErrStorageFull
		}
		defer func() { bsSvc.depositWeaponFn = nil }()

		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/storage/deposit", nil)
		req.Header.Set("Authorization", "Bearer dummy-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/blacksmith/storage/withdraw success", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"slot": 1})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/storage/withdraw", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/blacksmith/storage/withdraw slot occupied returns 400", func(t *testing.T) {
		bsSvc.withdrawWeaponFn = func(ctx context.Context, characterID string, slot int) error {
			return blacksmith.ErrWeaponSlotOccupied
		}
		defer func() { bsSvc.withdrawWeaponFn = nil }()

		body, _ := json.Marshal(map[string]any{"slot": 1})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/storage/withdraw", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/blacksmith/storage/withdraw not found returns 404", func(t *testing.T) {
		bsSvc.withdrawWeaponFn = func(ctx context.Context, characterID string, slot int) error {
			return blacksmith.ErrDepositNotFound
		}
		defer func() { bsSvc.withdrawWeaponFn = nil }()

		body, _ := json.Marshal(map[string]any{"slot": 2})
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blacksmith/storage/withdraw", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer dummy-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
