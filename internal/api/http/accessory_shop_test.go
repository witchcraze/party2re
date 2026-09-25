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

func TestAccessoryShopHTTPEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10, Money: 50000, JobLevel: 100}

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
			if shopType != shop.ShopTypeAccessory {
				return shop.ShopCatalog{}, shop.ErrInvalidShopType
			}
			return shop.ShopCatalog{
				ShopType: shopType,
				Title:    "アクセサリー屋",
				NPCName:  "@ミラ",
				Items: []shop.CatalogItem{
					{
						ID:          "item-147",
						Name:        "伝国の懐刀",
						BasePrice:   50,
						RetailPrice: 500,
					},
				},
			}, nil
		},
		talkNPCFn: func(_ context.Context, shopType shop.ShopType) (string, error) {
			if shopType != shop.ShopTypeAccessory {
				return "", shop.ErrInvalidShopType
			}
			return "ここはアクセサリー屋。ここでしか手に入らないアイテムばっかりよ", nil
		},
		inspectNPCFn: func(_ context.Context, shopType shop.ShopType, _ string) (shop.NPCInspectResult, error) {
			if shopType != shop.ShopTypeAccessory {
				return shop.NPCInspectResult{}, shop.ErrInvalidShopType
			}
			return shop.NPCInspectResult{
				ShopType: shopType,
				NPCName:  "@ミラ",
				Dialogue: "なにか私についてる？",
			}, nil
		},
		purchaseInShopFn: func(_ context.Context, characterID string, shopType shop.ShopType, itemDefinitionID string, quantity int) (shop.PurchaseResult, error) {
			inst, _ := coreitem.NewInstance(itemDefinitionID, quantity)
			return shop.PurchaseResult{
				Character:          corecharacter.Character{ID: characterID, Money: 49500},
				ItemInstance:       inst,
				TotalPrice:         500,
				TransferredToDepot: false,
			}, nil
		},
		sellFn: func(_ context.Context, characterID string, itemInstanceID string, quantity int) (shop.SaleResult, error) {
			return shop.SaleResult{
				Character:    corecharacter.Character{ID: characterID, Money: 50025},
				SoldInstance: coreitem.Instance{ID: itemInstanceID, Quantity: quantity},
				TotalPayout:  25,
			}, nil
		},
		synthesizeFn: func(_ context.Context, characterID string, recipeTarget string) (shop.SynthesisResult, error) {
			recipe, err := shop.FindSynthesisRecipe(recipeTarget)
			if err != nil {
				return shop.SynthesisResult{}, err
			}
			inst, _ := coreitem.NewInstance(recipe.ProductDefinitionID, 1)
			return shop.SynthesisResult{
				Recipe:      recipe,
				Success:     true,
				UsedHiyaku:  true,
				CreatedItem: &inst,
			}, nil
		},
	}

	h := newTestHandler(t, pService, cService, &stubAdventureService{}, sService)
	router := h.Router()

	t.Run("GET /characters/c1/shop/accessory", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/shop/accessory", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var catalog shop.ShopCatalog
		if err := json.Unmarshal(rr.Body.Bytes(), &catalog); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if catalog.Title != "アクセサリー屋" || catalog.NPCName != "@ミラ" {
			t.Errorf("unexpected catalog meta: %+v", catalog)
		}
	})

	t.Run("POST /characters/c1/shop/accessory/talk", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/accessory/talk", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("POST /characters/c1/shop/accessory/inspect", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/accessory/inspect", nil)
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
		if resp["dialogue"] != "なにか私についてる？" {
			t.Errorf("dialogue = %v, want なにか私についてる？", resp["dialogue"])
		}
	})

	t.Run("POST /characters/c1/shop/accessory/buy (envelope compliant)", func(t *testing.T) {
		body := `{"item_definition_id":"item-147","quantity":1}`
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/accessory/buy", bytes.NewBufferString(body))
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

		// Verify envelope pattern: { "data": ..., "message": ... }
		if _, ok := resp["data"]; !ok {
			t.Fatalf("missing 'data' field in envelope response: %s", rr.Body.String())
		}
		if msg, ok := resp["message"]; !ok || msg != "伝国の懐刀ね。はい" {
			t.Errorf("unexpected envelope message: %v", resp["message"])
		}
	})

	t.Run("POST /characters/c1/shop/accessory/sell (envelope compliant)", func(t *testing.T) {
		body := `{"item_instance_id":"inst-1","quantity":1}`
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/accessory/sell", bytes.NewBufferString(body))
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
		if _, ok := resp["data"]; !ok {
			t.Fatalf("missing 'data' field in envelope response: %s", rr.Body.String())
		}
	})

	t.Run("POST /characters/c1/shop/accessory/synthesize (envelope compliant)", func(t *testing.T) {
		body := `{"recipe_target":"命の宝珠"}`
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/shop/accessory/synthesize", bytes.NewBufferString(body))
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
		data, ok := resp["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("missing or invalid 'data' in envelope response: %s", rr.Body.String())
		}
		if data["success"] != true || data["used_hiyaku"] != true {
			t.Errorf("unexpected synthesis data: %+v", data)
		}
	})

	t.Run("GET /characters/c1/shop/accessory/recipes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/shop/accessory/recipes", nil)
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
		data, ok := resp["data"].([]interface{})
		if !ok || len(data) != 48 {
			t.Fatalf("expected 48 recipes in data, got %d", len(data))
		}
	})
}
