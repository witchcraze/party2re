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
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/gemstore"
)

type stubGemStoreService struct {
	buyGemFn        func(ctx context.Context, characterID, gemID string) (gemstore.BuyResult, error)
	sellGemFn       func(ctx context.Context, characterID, itemID string) (gemstore.SellResult, error)
	sendGemFn       func(ctx context.Context, senderID, recipientID, itemID string) (gemstore.SendResult, error)
	synthesizeGemFn func(ctx context.Context, characterID, recipeID string) (gemstore.SynthesizeResult, error)
	appraiseItemFn  func(ctx context.Context, characterID, itemID string) (gemstore.AppraiseResult, error)
	getCatalogFn    func(level int) []gemstore.Gem
	getRecipesFn    func() []gemstore.Recipe
	getDialogueFn   func() []string
}

func (s *stubGemStoreService) BuyGem(ctx context.Context, characterID, gemID string) (gemstore.BuyResult, error) {
	if s.buyGemFn != nil {
		return s.buyGemFn(ctx, characterID, gemID)
	}
	return gemstore.BuyResult{
		Cost: 300,
		Gem:  gemstore.Gem{ID: gemID, Name: "攻撃の宝珠Ⅰ", Price: 60},
	}, nil
}

func (s *stubGemStoreService) SellGem(ctx context.Context, characterID, itemID string) (gemstore.SellResult, error) {
	if s.sellGemFn != nil {
		return s.sellGemFn(ctx, characterID, itemID)
	}
	return gemstore.SellResult{
		Payout: 30,
		Gem:    gemstore.Gem{ID: "gem_atk_1", Name: "攻撃の宝珠Ⅰ", Price: 60},
	}, nil
}

func (s *stubGemStoreService) SendGem(ctx context.Context, senderID, recipientID, itemID string) (gemstore.SendResult, error) {
	if s.sendGemFn != nil {
		return s.sendGemFn(ctx, senderID, recipientID, itemID)
	}
	return gemstore.SendResult{
		SenderCharacter:    corecharacter.Character{ID: senderID, Name: "Sender"},
		RecipientCharacter: corecharacter.Character{ID: recipientID, Name: "Recipient"},
		Gem:                gemstore.Gem{ID: "gem_atk_1", Name: "攻撃の宝珠Ⅰ"},
	}, nil
}

func (s *stubGemStoreService) SynthesizeGem(ctx context.Context, characterID, recipeID string) (gemstore.SynthesizeResult, error) {
	if s.synthesizeGemFn != nil {
		return s.synthesizeGemFn(ctx, characterID, recipeID)
	}
	return gemstore.SynthesizeResult{
		CreatedGem: gemstore.Gem{ID: "gem_atk_2", Name: "攻撃の宝珠Ⅱ"},
		Recipe:     gemstore.Recipe{ID: recipeID, ResultName: "攻撃の宝珠Ⅱ"},
	}, nil
}

func (s *stubGemStoreService) AppraiseItem(ctx context.Context, characterID, itemID string) (gemstore.AppraiseResult, error) {
	if s.appraiseItemFn != nil {
		return s.appraiseItemFn(ctx, characterID, itemID)
	}
	return gemstore.AppraiseResult{
		IsGem:          true,
		IdentifiedGem:  &gemstore.Gem{ID: "gem_atk_2", Name: "攻撃の宝珠Ⅱ"},
		IdentifiedName: "攻撃の宝珠Ⅱ",
		Message:        "これは… 攻撃の宝珠Ⅱですね",
	}, nil
}

func (s *stubGemStoreService) GetCatalog(level int) []gemstore.Gem {
	if s.getCatalogFn != nil {
		return s.getCatalogFn(level)
	}
	return []gemstore.Gem{
		{ID: "gem_atk_1", Name: "攻撃の宝珠Ⅰ", Price: 60, RequiredLevel: 1},
	}
}

func (s *stubGemStoreService) GetRecipes() []gemstore.Recipe {
	if s.getRecipesFn != nil {
		return s.getRecipesFn()
	}
	return []gemstore.Recipe{
		{ID: "recipe_atk_2", ResultName: "攻撃の宝珠Ⅱ", Material1: "攻撃の宝珠Ⅰ", Material2: "攻撃の宝珠Ⅰ"},
	}
}

func (s *stubGemStoreService) GetDialogue() []string {
	if s.getDialogueFn != nil {
		return s.getDialogueFn()
	}
	return []string{"いらっしゃいませ！"}
}

func TestGemStoreHTTP_Endpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player_1", Username: "hero_user"}
	char := corecharacter.Character{
		ID:       "char_1",
		PlayerID: "player_1",
		Name:     "Hero",
		Level:    10,
		Money:    5000,
	}

	players := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	characters := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "char_1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	gemService := &stubGemStoreService{}

	handler := newTestHandler(
		t,
		players,
		characters,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithGemStore(gemService),
	)

	server := handler.Router()

	// 1. GET /gemstore/catalog
	{
		req := httptest.NewRequest(http.MethodGet, "/gemstore/catalog?level=10", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode JSON: %v", err)
		}
		if body["gems"] == nil {
			t.Errorf("expected gems array in response")
		}
	}

	// 2. GET /gemstore/recipes
	{
		req := httptest.NewRequest(http.MethodGet, "/gemstore/recipes", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 3. GET /gemstore/dialogue
	{
		req := httptest.NewRequest(http.MethodGet, "/gemstore/dialogue", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 4. POST /characters/{id}/gemstore/buy
	{
		payload, _ := json.Marshal(map[string]string{
			"gem_id": "gem_atk_1",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char_1/gemstore/buy", bytes.NewReader(payload))
		req.Header.Set("Authorization", bearerToken("session_1"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 5. POST /characters/{id}/gemstore/sell
	{
		payload, _ := json.Marshal(map[string]string{
			"item_id": "gem_atk_1",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char_1/gemstore/sell", bytes.NewReader(payload))
		req.Header.Set("Authorization", bearerToken("session_1"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 6. POST /characters/{id}/gemstore/send
	{
		payload, _ := json.Marshal(map[string]string{
			"recipient_character_id": "char_2",
			"item_id":                "gem_atk_1",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char_1/gemstore/send", bytes.NewReader(payload))
		req.Header.Set("Authorization", bearerToken("session_1"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 7. POST /characters/{id}/gemstore/synthesize
	{
		payload, _ := json.Marshal(map[string]string{
			"recipe_id": "recipe_atk_2",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char_1/gemstore/synthesize", bytes.NewReader(payload))
		req.Header.Set("Authorization", bearerToken("session_1"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 8. POST /characters/{id}/gemstore/appraise
	{
		payload, _ := json.Marshal(map[string]string{
			"item_id": "orb_1",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char_1/gemstore/appraise", bytes.NewReader(payload))
		req.Header.Set("Authorization", bearerToken("session_1"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// 9. Unauthorized check (missing token)
	{
		payload, _ := json.Marshal(map[string]string{
			"gem_id": "gem_atk_1",
		})
		req := httptest.NewRequest(http.MethodPost, "/characters/char_1/gemstore/buy", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	}
}

// unused import suppression
var _ = coreinventory.Inventory{}
var _ = coreitem.Instance{}
