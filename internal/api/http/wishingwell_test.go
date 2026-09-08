package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

type stubWishingWellService struct {
	getStatusFn func(ctx context.Context, characterID string) (wishingwell.WishingWellStatus, error)
	exchangeFn  func(ctx context.Context, req wishingwell.ExchangeRequest) (wishingwell.ExchangeResult, error)
}

func (s *stubWishingWellService) GetStatus(ctx context.Context, characterID string) (wishingwell.WishingWellStatus, error) {
	if s.getStatusFn != nil {
		return s.getStatusFn(ctx, characterID)
	}
	return wishingwell.WishingWellStatus{CharacterID: characterID}, nil
}

func (s *stubWishingWellService) Exchange(ctx context.Context, req wishingwell.ExchangeRequest) (wishingwell.ExchangeResult, error) {
	if s.exchangeFn != nil {
		return s.exchangeFn(ctx, req)
	}
	return wishingwell.ExchangeResult{CharacterID: req.CharacterID}, nil
}

func TestWishingWellEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", SP: 10}

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
	wService := &stubWishingWellService{
		getStatusFn: func(_ context.Context, characterID string) (wishingwell.WishingWellStatus, error) {
			return wishingwell.WishingWellStatus{
				CharacterID:   characterID,
				CharacterName: "Hero",
				SP:            10,
				CanExchange:   true,
			}, nil
		},
		exchangeFn: func(_ context.Context, req wishingwell.ExchangeRequest) (wishingwell.ExchangeResult, error) {
			if req.SP < 1 {
				return wishingwell.ExchangeResult{}, wishingwell.ErrInvalidSPAmount
			}
			if req.SP > 10 {
				return wishingwell.ExchangeResult{}, wishingwell.ErrInsufficientSP
			}
			if req.Stat == "invalid" {
				return wishingwell.ExchangeResult{}, wishingwell.ErrInvalidTargetStat
			}
			return wishingwell.ExchangeResult{
				CharacterID:  req.CharacterID,
				Stat:         corecharacter.SPExchangeMaxHP,
				StatName:     "ＨＰ",
				SPConsumed:   req.SP,
				StatIncrease: req.SP * 2,
				RemainingSP:  10 - req.SP,
				Message:      "SP 5 のかわりに ＨＰ を 10 あたえましょう",
			}, nil
		},
	}

	handler := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithWishingWell(wService),
	)
	router := handler.Router()

	// 1. GET /characters/c1/wishing-well
	t.Run("GET Wishing Well Status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/wishing-well", nil)
		req.Header.Set("Authorization", "Bearer valid_token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"sp":10`) {
			t.Errorf("expected response to contain sp:10, got %s", rec.Body.String())
		}
	})

	// 2. POST /characters/c1/wishing-well/exchange (success)
	t.Run("POST Wishing Well Exchange success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/wishing-well/exchange", strings.NewReader(`{"stat":"mhp","sp":5}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"stat_increase":10`) {
			t.Errorf("expected response to contain stat_increase:10, got %s", rec.Body.String())
		}
	})

	// 3. POST /characters/c1/wishing-well/exchange (insufficient SP -> 400)
	t.Run("POST Wishing Well Exchange insufficient SP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/wishing-well/exchange", strings.NewReader(`{"stat":"mhp","sp":20}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 4. POST /characters/c1/wishing-well/exchange (invalid stat -> 400)
	t.Run("POST Wishing Well Exchange invalid stat", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/wishing-well/exchange", strings.NewReader(`{"stat":"invalid","sp":1}`))
		req.Header.Set("Authorization", "Bearer valid_token")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
