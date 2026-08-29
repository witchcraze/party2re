package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubCasinoService struct {
	getAccountFn           func(ctx context.Context, characterID string) (casino.Account, error)
	exchangeGoldToCoinsFn  func(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	exchangeCoinsToGoldFn  func(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	spinSlotFn             func(ctx context.Context, characterID string, bet int64) (casino.SpinResult, casino.Account, error)
	playHighLowFn          func(ctx context.Context, characterID string, betCoins int64, guess casino.GuessType) (casino.HighLowResult, casino.Account, error)
	playDoppelFn           func(ctx context.Context, characterID string, bet int64, poolSize int, playerMark casino.DoppelMark) (casino.DoppelResult, casino.Account, error)
	startIndianPokerGameFn func(ctx context.Context, characterID string, baseRate int64) (*casino.IndianPokerGame, casino.Account, error)
}

func (s *stubCasinoService) GetAccount(ctx context.Context, characterID string) (casino.Account, error) {
	if s.getAccountFn != nil {
		return s.getAccountFn(ctx, characterID)
	}
	return casino.Account{CharacterID: characterID, Coins: 100, UpdatedAt: time.Now()}, nil
}

func (s *stubCasinoService) ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error) {
	if s.exchangeGoldToCoinsFn != nil {
		return s.exchangeGoldToCoinsFn(ctx, characterID, coins)
	}
	return casino.Account{CharacterID: characterID, Coins: coins, UpdatedAt: time.Now()}, corecharacter.Character{ID: characterID}, nil
}

func (s *stubCasinoService) ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error) {
	if s.exchangeCoinsToGoldFn != nil {
		return s.exchangeCoinsToGoldFn(ctx, characterID, coins)
	}
	return casino.Account{CharacterID: characterID, Coins: 0, UpdatedAt: time.Now()}, corecharacter.Character{ID: characterID, Money: int(coins * 20)}, nil
}

func (s *stubCasinoService) SpinSlot(ctx context.Context, characterID string, bet int64) (casino.SpinResult, casino.Account, error) {
	if s.spinSlotFn != nil {
		return s.spinSlotFn(ctx, characterID, bet)
	}
	return casino.SpinResult{BetCoins: bet, Reels: [3]casino.SlotSymbol{casino.SymbolCherry, casino.SymbolCherry, casino.SymbolCherry}, IsWin: true, Multiplier: 2, PayoutCoins: bet * 2}, casino.Account{CharacterID: characterID, Coins: bet * 2}, nil
}

func (s *stubCasinoService) PlayHighLow(ctx context.Context, characterID string, betCoins int64, guess casino.GuessType) (casino.HighLowResult, casino.Account, error) {
	if s.playHighLowFn != nil {
		return s.playHighLowFn(ctx, characterID, betCoins, guess)
	}
	return casino.HighLowResult{BetCoins: betCoins, Guess: guess, Outcome: casino.OutcomeWin, Multiplier: 2, PayoutCoins: betCoins * 2}, casino.Account{CharacterID: characterID, Coins: betCoins * 2}, nil
}

func (s *stubCasinoService) PlayDoppel(ctx context.Context, characterID string, bet int64, poolSize int, playerMark casino.DoppelMark) (casino.DoppelResult, casino.Account, error) {
	if s.playDoppelFn != nil {
		return s.playDoppelFn(ctx, characterID, bet, poolSize, playerMark)
	}
	return casino.DoppelResult{BetCoins: bet, PlayerMark: playerMark, IsWin: true, Multiplier: poolSize, PayoutCoins: bet * int64(poolSize)}, casino.Account{CharacterID: characterID, Coins: bet * int64(poolSize)}, nil
}

func (s *stubCasinoService) StartIndianPokerGame(ctx context.Context, characterID string, baseRate int64) (*casino.IndianPokerGame, casino.Account, error) {
	if s.startIndianPokerGameFn != nil {
		return s.startIndianPokerGameFn(ctx, characterID, baseRate)
	}
	game, _ := casino.NewIndianPokerGame(baseRate)
	return game, casino.Account{CharacterID: characterID, Coins: 100}, nil
}

func TestCasinoEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Money: 1000}

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
	casService := &stubCasinoService{}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithCasino(casService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/casino - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/casino", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/exchange - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/exchange", `{"direction":"gold_to_coins","coins":10}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/slot - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/slot", `{"bet":10}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/highlow - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/highlow", `{"bet":10,"guess":"HIGH"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/doppel - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/doppel", `{"bet":10,"pool_size":4,"player_mark":"★"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/poker - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/poker", `{"base_rate":10}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
