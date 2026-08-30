package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubBlackMarketService struct {
	getStatusFn    func(ctx context.Context, characterID string, now time.Time) (*blackmarket.ShopStatus, error)
	purchaseItemFn func(ctx context.Context, characterID string, itemID string, quantity int, now time.Time) (*blackmarket.PurchaseResult, error)
	sellItemFn     func(ctx context.Context, characterID string, itemInstanceID string, quantity int, now time.Time) (*blackmarket.SaleResult, error)
	talkFn         func(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	rumorsFn       func(ctx context.Context, characterID string, now time.Time) (*blackmarket.RumorsResult, error)
}

func (s *stubBlackMarketService) GetStatus(ctx context.Context, characterID string, now time.Time) (*blackmarket.ShopStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID, now)
	}
	return &blackmarket.ShopStatus{
		CharacterID:  characterID,
		LocationName: blackmarket.LocationName,
		NPCName:      blackmarket.NPCName,
		IsEligible:   true,
		MarketState:  blackmarket.DefaultMarketStates[blackmarket.ConditionQuiet],
		Items:        []blackmarket.ShopItemView{},
	}, nil
}

func (s *stubBlackMarketService) PurchaseItem(ctx context.Context, characterID string, itemID string, quantity int, now time.Time) (*blackmarket.PurchaseResult, error) {
	if s.purchaseItemFn != nil {
		return s.purchaseItemFn(ctx, characterID, itemID, quantity, now)
	}
	return &blackmarket.PurchaseResult{
		CharacterID:         characterID,
		Item:                blackmarket.Item{ID: itemID, Name: "Test Contraband", BasePrice: 1500},
		Quantity:            quantity,
		UnitPrice:           1500,
		TotalPrice:          1500 * quantity,
		RemainingGold:       5000,
		InventoryInstanceID: "inst-1",
		RemainingQuota:      3,
	}, nil
}

func (s *stubBlackMarketService) SellItem(ctx context.Context, characterID string, itemInstanceID string, quantity int, now time.Time) (*blackmarket.SaleResult, error) {
	if s.sellItemFn != nil {
		return s.sellItemFn(ctx, characterID, itemInstanceID, quantity, now)
	}
	return &blackmarket.SaleResult{
		CharacterID:    characterID,
		ItemInstanceID: itemInstanceID,
		ItemName:       "どくばり",
		Quantity:       quantity,
		UnitPrice:      900,
		TotalPayout:    900 * quantity,
		RemainingGold:  5900,
	}, nil
}

func (s *stubBlackMarketService) Talk(ctx context.Context, characterID string) (*blackmarket.TalkResult, error) {
	if s.talkFn != nil {
		return s.talkFn(ctx, characterID)
	}
	return &blackmarket.TalkResult{
		CharacterID: characterID,
		NPCName:     blackmarket.NPCName,
		Dialogue:    "……チッ、誰に聞いてここに来た？",
	}, nil
}

func (s *stubBlackMarketService) Rumors(ctx context.Context, characterID string, now time.Time) (*blackmarket.RumorsResult, error) {
	if s.rumorsFn != nil {
		return s.rumorsFn(ctx, characterID, now)
	}
	return &blackmarket.RumorsResult{
		CharacterID:     characterID,
		NPCName:         blackmarket.NPCName,
		MarketCondition: string(blackmarket.ConditionQuiet),
		Rumor:           "今は衛兵の目も緩んでて相場は落ち着いてるぜ。",
	}, nil
}

func TestBlackMarketEndpoints(t *testing.T) {
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
	bmService := &stubBlackMarketService{}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithBlackMarket(bmService),
	)
	router := h.Router()

	t.Run("GetStatus_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/blackmarket", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), blackmarket.NPCName) {
			t.Errorf("expected body to contain %s, got: %s", blackmarket.NPCName, rec.Body.String())
		}
	})

	t.Run("Purchase_Success", func(t *testing.T) {
		reqBody := `{"item_id":"bm_poison_needle","quantity":2}`
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/purchase", strings.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "bm_poison_needle") {
			t.Errorf("expected response to contain item id, got: %s", rec.Body.String())
		}
	})

	t.Run("Sell_Success", func(t *testing.T) {
		reqBody := `{"item_instance_id":"inst-1","quantity":1}`
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/sell", strings.NewReader(reqBody))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "inst-1") {
			t.Errorf("expected response to contain instance id, got: %s", rec.Body.String())
		}
	})

	t.Run("Talk_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/talk", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), blackmarket.NPCName) {
			t.Errorf("expected NPC name in talk response, got: %s", rec.Body.String())
		}
	})

	t.Run("Rumors_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/rumors", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "market_condition") {
			t.Errorf("expected market_condition in rumors response, got: %s", rec.Body.String())
		}
	})

	t.Run("Unauthorized_NoToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/blackmarket", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("AccessDenied_Error", func(t *testing.T) {
		bmErrService := &stubBlackMarketService{
			getStatusFn: func(_ context.Context, _ string, _ time.Time) (*blackmarket.ShopStatus, error) {
				return nil, blackmarket.ErrAccessDenied
			},
		}
		hErr := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithBlackMarket(bmErrService),
		)
		errRouter := hErr.Router()

		req := httptest.NewRequest(http.MethodGet, "/characters/c1/blackmarket", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		errRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", rec.Code)
		}
	})
}
