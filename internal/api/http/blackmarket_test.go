package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubBlackMarketService struct {
	getStatusFn     func(ctx context.Context, characterID string) (*blackmarket.Status, error)
	talkFn          func(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	inspectFn       func(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	sacrificeItemFn func(ctx context.Context, characterID string, itemInstanceID string) (*blackmarket.SacrificeResult, error)
	tradePrizeFn    func(ctx context.Context, characterID string, prizeID string) (*blackmarket.TradeResult, error)
}

func (s *stubBlackMarketService) GetStatus(ctx context.Context, characterID string) (*blackmarket.Status, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return &blackmarket.Status{
		CharacterID:  characterID,
		LocationName: blackmarket.LocationName,
		NPCName:      blackmarket.NPCName,
		RarePoints:   10,
		URarePoints:  2,
		Prizes:       []blackmarket.Prize{},
		UPrizes:      []blackmarket.Prize{},
	}, nil
}

func (s *stubBlackMarketService) Talk(ctx context.Context, characterID string) (*blackmarket.TalkResult, error) {
	if s.talkFn != nil {
		return s.talkFn(ctx, characterID)
	}
	return &blackmarket.TalkResult{
		CharacterID: characterID,
		NPCName:     blackmarket.NPCName,
	}, nil
}

func (s *stubBlackMarketService) Inspect(ctx context.Context, characterID string) (*blackmarket.TalkResult, error) {
	if s.inspectFn != nil {
		return s.inspectFn(ctx, characterID)
	}
	return &blackmarket.TalkResult{
		CharacterID: characterID,
		NPCName:     blackmarket.NPCName,
	}, nil
}

func (s *stubBlackMarketService) SacrificeItem(ctx context.Context, characterID string, itemInstanceID string) (*blackmarket.SacrificeResult, error) {
	if s.sacrificeItemFn != nil {
		return s.sacrificeItemFn(ctx, characterID, itemInstanceID)
	}
	return &blackmarket.SacrificeResult{
		CharacterID:       characterID,
		ItemInstanceID:    itemInstanceID,
		ItemDefinitionID:  "weapon-29",
		ItemName:          "はやぶさの剣",
		RarePointsGained:  1,
		URarePointsGained: 0,
		TotalRarePoints:   11,
		TotalURarePoints:  2,
		Message:           "…はやぶさの剣…か…。レアだな…。いいだろう…。お前のレアポイントを加算しておこう…",
	}, nil
}

func (s *stubBlackMarketService) TradePrize(ctx context.Context, characterID string, prizeID string) (*blackmarket.TradeResult, error) {
	if s.tradePrizeFn != nil {
		return s.tradePrizeFn(ctx, characterID, prizeID)
	}
	return &blackmarket.TradeResult{
		CharacterID:      characterID,
		PrizeID:          prizeID,
		ItemDefinitionID: "item-087",
		ItemName:         "時の砂",
		DepotInstanceID:  "inst-prize-1",
		Cost:             1,
		IsURare:          false,
		RemainingRare:    9,
		RemainingURare:   2,
		Message:          "取引成立だ…。時の砂 はお前の預かり所に送っておいた…",
	}, nil
}

func TestBlackMarketEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Shadow Hero", Level: 1}

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

	handler := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithBlackMarket(bmService),
	)
	router := handler.Router()

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
		if !strings.Contains(rec.Body.String(), `"dialogue"`) {
			t.Errorf("expected dialogue in talk response, got: %s", rec.Body.String())
		}
	})

	t.Run("Inspect_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/inspect", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "お前の魂で取引したいのか？") {
			t.Errorf("expected inspect dialogue in response, got: %s", rec.Body.String())
		}
	})

	t.Run("Unauthorized_NoToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/talk", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rec.Code)
		}
	})

	t.Run("Talk_AccessDenied", func(t *testing.T) {
		denied := newTestHandler(t, pService, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithBlackMarket(&stubBlackMarketService{
			talkFn: func(context.Context, string) (*blackmarket.TalkResult, error) {
				return nil, blackmarket.ErrAccessDenied
			},
		}))
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/blackmarket/talk", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		denied.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", rec.Code)
		}
	})

	t.Run("Retired_Routes", func(t *testing.T) {
		for _, route := range []struct{ method, path string }{
			{http.MethodGet, "/characters/c1/blackmarket"},
			{http.MethodGet, "/characters/c1/blackmarket/points"},
			{http.MethodPost, "/characters/c1/blackmarket/sacrifice"},
			{http.MethodPost, "/characters/c1/blackmarket/trade"},
		} {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"item_instance_id":"i","prize_id":"p"}`))
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s still reachable: %d", route.method, route.path, rec.Code)
			}
		}
	})
}
