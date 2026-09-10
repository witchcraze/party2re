package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/shop"
)

func TestShopEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10, Money: 50000}

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
	sService := &stubShopService{
		getCatalogFn: func(_ context.Context, shopType shop.ShopType, _ string) (shop.ShopCatalog, error) {
			if shopType != shop.ShopTypeWeapon && shopType != shop.ShopTypeArmor && shopType != shop.ShopTypeItem {
				return shop.ShopCatalog{}, shop.ErrInvalidShopType
			}
			return shop.ShopCatalog{
				ShopType: shopType,
				Title:    "武器屋",
				NPCName:  "ブッキー",
				Items: []shop.CatalogItem{
					{
						ID:          "weapon-01",
						Name:        "Club",
						BasePrice:   10,
						RetailPrice: 20,
					},
				},
			}, nil
		},
		batchPurchaseFn: func(_ context.Context, characterID string, _ shop.ShopType, items []shop.BatchPurchaseItemRequest) (shop.BatchPurchaseResult, error) {
			total := 0
			count := 0
			for _, it := range items {
				total += 20 * it.Quantity
				count += it.Quantity
			}
			return shop.BatchPurchaseResult{
				Character:  corecharacter.Character{ID: characterID},
				Purchased:  make([]coreitem.Instance, count),
				TotalPrice: total,
				NPCMessage: "batch delivered to depot",
			}, nil
		},
		inspectNPCFn: func(_ context.Context, shopType shop.ShopType, _ string) (shop.NPCInspectResult, error) {
			dialogue, hint := shop.GetInspectDialogue(shopType)
			return shop.NPCInspectResult{
				ShopType:       shopType,
				Dialogue:       dialogue,
				SecretShopHint: hint,
			}, nil
		},
		talkNPCFn: func(_ context.Context, shopType shop.ShopType) (string, error) {
			words := shop.GetShopWords(shopType)
			return words[0], nil
		},
		discoverSecretShopFn: func(_ context.Context, _ string) (bool, string, error) {
			return true, "秘密の店を見つけました！", nil
		},
		purchaseFn: func(_ context.Context, characterID, itemID string, qty int) (shop.PurchaseResult, error) {
			return shop.PurchaseResult{
				Character:          char,
				ItemInstance:       coreitem.Instance{ID: "inst-1", DefinitionID: itemID, Quantity: qty},
				TotalPrice:         20 * qty,
				TransferredToDepot: true,
				NPCMessage:         "delivered to depot",
			}, nil
		},
	}

	h := newTestHandler(t, pService, cService, &stubAdventureService{}, sService)
	router := h.Router()

	t.Run("GET /characters/{id}/shop/{type} success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/shop/weapon", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp shop.ShopCatalog
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ShopType != shop.ShopTypeWeapon {
			t.Errorf("expected weapon, got %s", resp.ShopType)
		}
		if len(resp.Items) != 1 || resp.Items[0].RetailPrice != 20 {
			t.Errorf("unexpected catalog items: %+v", resp.Items)
		}
	})

	t.Run("GET /characters/{id}/shop/{type} invalid type returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/shop/unknown", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /characters/{id}/shop/batch-purchase success", func(t *testing.T) {
		body := `{"shop_type":"weapon","items":[{"item_definition_id":"weapon-01","quantity":3}]}`
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/batch-purchase", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["total_price"] != float64(60) {
			t.Errorf("expected total_price 60, got %v", resp["total_price"])
		}
		if resp["purchased_count"] != float64(3) {
			t.Errorf("expected purchased_count 3, got %v", resp["purchased_count"])
		}
	})

	t.Run("POST /characters/{id}/shop/{type}/inspect success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/item/inspect", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["dialogue"] == "" {
			t.Error("expected dialogue to be non-empty")
		}
		if resp["secret_shop_hint"] != shop.SecretShopHint {
			t.Errorf("expected secret shop hint, got %v", resp["secret_shop_hint"])
		}
	})

	t.Run("POST /characters/{id}/shop/{type}/talk success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/weapon/talk", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["dialogue"] == "" {
			t.Error("expected dialogue to be non-empty")
		}
	})

	t.Run("POST /characters/{id}/shop/discover-secret success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/discover-secret", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["unlocked"] != true {
			t.Errorf("expected unlocked=true, got %v", resp["unlocked"])
		}
	})

	t.Run("POST /shop/purchase returns depot and npc message fields", func(t *testing.T) {
		body := `{"character_id":"c1","item_definition_id":"weapon-01","quantity":1}`
		req := httptest.NewRequest(http.MethodPost, "/shop/purchase", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["transferred_to_depot"] != true {
			t.Errorf("expected transferred_to_depot=true, got %v", resp["transferred_to_depot"])
		}
		if resp["npc_message"] != "delivered to depot" {
			t.Errorf("expected npc_message, got %v", resp["npc_message"])
		}
	})
}
