package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/secretshop"
)

type stubSecretShopService struct {
	getStatusFn    func(ctx context.Context, characterID string) (*secretshop.ShopStatus, error)
	talkFn         func(ctx context.Context, characterID string) (string, error)
	inspectFn      func(ctx context.Context, characterID string) (string, error)
	puffPuffFn     func(ctx context.Context, characterID string) (*secretshop.PuffPuffResult, error)
	purchaseItemFn func(ctx context.Context, characterID string, itemID string, quantity int) (*secretshop.PurchaseResult, error)
}

func (s *stubSecretShopService) GetShopStatus(ctx context.Context, characterID string) (*secretshop.ShopStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return &secretshop.ShopStatus{
		CharacterID:  characterID,
		LocationName: secretshop.LocationName,
		NPCName:      secretshop.NPCName,
		IsEligible:   true,
		Items:        []secretshop.Item{},
	}, nil
}

func (s *stubSecretShopService) Talk(ctx context.Context, characterID string) (string, error) {
	if s.talkFn != nil {
		return s.talkFn(ctx, characterID)
	}
	return "メェ〜", nil
}

func (s *stubSecretShopService) Inspect(ctx context.Context, characterID string) (string, error) {
	if s.inspectFn != nil {
		return s.inspectFn(ctx, characterID)
	}
	return secretshop.InspectDialogue, nil
}

func (s *stubSecretShopService) PuffPuff(ctx context.Context, characterID string) (*secretshop.PuffPuffResult, error) {
	if s.puffPuffFn != nil {
		return s.puffPuffFn(ctx, characterID)
	}
	return &secretshop.PuffPuffResult{
		CharacterID: characterID,
		NPCName:     secretshop.NPCName,
		Message:     secretshop.PuffPuffDialogue,
	}, nil
}

func (s *stubSecretShopService) PurchaseItem(ctx context.Context, characterID string, itemID string, quantity int) (*secretshop.PurchaseResult, error) {
	if s.purchaseItemFn != nil {
		return s.purchaseItemFn(ctx, characterID, itemID, quantity)
	}
	return &secretshop.PurchaseResult{
		CharacterID:         characterID,
		Item:                secretshop.Item{ID: itemID, Name: "Test Item", Price: 1000},
		Quantity:            quantity,
		TotalPrice:          1000 * quantity,
		RemainingGold:       5000,
		InventoryInstanceID: "inst-1",
	}, nil
}

func TestSecretShopEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 20, Money: 50000}

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
	ssService := &stubSecretShopService{}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithSecretShop(ssService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/secretshop success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/secretshop", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("GET /characters/{id}/secretshop forbidden access denied", func(t *testing.T) {
		ssServiceErr := &stubSecretShopService{
			getStatusFn: func(_ context.Context, _ string) (*secretshop.ShopStatus, error) {
				return nil, secretshop.ErrAccessDenied
			},
		}
		hErr := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithSecretShop(ssServiceErr),
		)
		errRouter := hErr.Router()

		req := httptest.NewRequest(http.MethodGet, "/characters/c1/secretshop", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		errRouter.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /characters/{id}/secretshop/talk", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/secretshop/talk", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /characters/{id}/secretshop/inspect", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/secretshop/inspect", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /characters/{id}/secretshop/puffpuff", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/secretshop/puffpuff", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	for _, tc := range []struct {
		name    string
		toDepot bool
		message string
	}{
		{"inventory", false, "Test Itemメェ〜。持ってけメェ〜"},
		{"depot", true, "Test ItemはHeroメェ〜の預かり所の方に投げましたメェ〜"},
	} {
		t.Run("POST /characters/{id}/secretshop/purchase "+tc.name, func(t *testing.T) {
			ssService.purchaseItemFn = func(ctx context.Context, characterID, itemID string, quantity int) (*secretshop.PurchaseResult, error) {
				result, err := (&stubSecretShopService{}).PurchaseItem(ctx, characterID, itemID, quantity)
				result.TransferredToDepot = tc.toDepot
				return result, err
			}
			t.Cleanup(func() { ssService.purchaseItemFn = nil })
			req := jsonRequest(t, http.MethodPost, "/characters/c1/secretshop/purchase", `{"item_id":"secret_item_herbal_root","quantity":1}`)
			req.Header.Set("Authorization", "Bearer valid-token")
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
			}
			var response struct {
				secretshop.PurchaseResult
				NPCMessage string `json:"npc_message"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.NPCMessage != tc.message {
				t.Fatalf("npc_message = %q; want %q", response.NPCMessage, tc.message)
			}
			if response.CharacterID != "c1" || response.Item.Name != "Test Item" || response.Quantity != 1 ||
				response.TotalPrice != 1000 || response.RemainingGold != 5000 || response.InventoryInstanceID != "inst-1" ||
				response.TransferredToDepot != tc.toDepot {
				t.Fatalf("unexpected purchase result: %+v", response.PurchaseResult)
			}
		})
	}

	t.Run("POST /characters/{id}/secretshop/purchase insufficient funds", func(t *testing.T) {
		ssServiceErr := &stubSecretShopService{
			purchaseItemFn: func(_ context.Context, _, _ string, _ int) (*secretshop.PurchaseResult, error) {
				return nil, secretshop.ErrInsufficientFunds
			},
		}
		hErr := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithSecretShop(ssServiceErr),
		)
		errRouter := hErr.Router()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/secretshop/purchase", `{"item_id":"secret_item_herbal_root","quantity":1}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		errRouter.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d: %s", rr.Code, rr.Body.String())
		}
	})
}
