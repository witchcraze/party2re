package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/lottery"
)

type stubLotteryService struct {
	getRaffleTicketsFn             func(ctx context.Context, characterID string) (int, error)
	buyRaffleTicketsFn             func(ctx context.Context, characterID string, count int) (int, corecharacter.Character, error)
	playRaffleFn                   func(ctx context.Context, characterID string, raffleType lottery.RaffleType) (lottery.RaffleResult, int, corecharacter.Character, error)
	getTakarakujiStatusFn          func(ctx context.Context) (lottery.TakarakujiStatus, error)
	buyTakarakujiTicketFn          func(ctx context.Context, characterID string) (lottery.TakarakujiPurchaseResult, error)
	getCharacterTakarakujiTicketFn func(ctx context.Context, characterID string) (*lottery.TakarakujiTicket, []lottery.TakarakujiTicket, error)
}

func (s *stubLotteryService) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	if s.getRaffleTicketsFn != nil {
		return s.getRaffleTicketsFn(ctx, characterID)
	}
	return 0, nil
}

func (s *stubLotteryService) BuyRaffleTickets(ctx context.Context, characterID string, count int) (int, corecharacter.Character, error) {
	if s.buyRaffleTicketsFn != nil {
		return s.buyRaffleTicketsFn(ctx, characterID, count)
	}
	return count, corecharacter.Character{ID: characterID}, nil
}

func (s *stubLotteryService) PlayRaffle(ctx context.Context, characterID string, raffleType lottery.RaffleType) (lottery.RaffleResult, int, corecharacter.Character, error) {
	if s.playRaffleFn != nil {
		return s.playRaffleFn(ctx, characterID, raffleType)
	}
	return lottery.RaffleResult{Prize: lottery.RafflePrize{Tier: lottery.PrizeTierMiss, Name: "Pocket Tissue"}}, 0, corecharacter.Character{ID: characterID}, nil
}

func (s *stubLotteryService) GetTakarakujiStatus(ctx context.Context) (lottery.TakarakujiStatus, error) {
	if s.getTakarakujiStatusFn != nil {
		return s.getTakarakujiStatusFn(ctx)
	}
	return lottery.TakarakujiStatus{
		RoundID:          1,
		Title:            "宝くじ屋",
		NPCName:          "@クラゲ",
		TicketPrice:      30000,
		MaxTickets:       20,
		SoldCount:        5,
		RemainingTickets: 15,
		DrawDate:         time.Now().Add(5 * 24 * time.Hour),
	}, nil
}

func (s *stubLotteryService) BuyTakarakujiTicket(ctx context.Context, characterID string) (lottery.TakarakujiPurchaseResult, error) {
	if s.buyTakarakujiTicketFn != nil {
		return s.buyTakarakujiTicketFn(ctx, characterID)
	}
	return lottery.TakarakujiPurchaseResult{
		Ticket:        lottery.TakarakujiTicket{ID: "t-1", RoundID: 1, CharacterID: characterID},
		RemainingGold: 70000,
		NPCMessage:    "ありがとー。当たってたら賞品が届くからね",
	}, nil
}

func (s *stubLotteryService) GetCharacterTakarakujiTicket(ctx context.Context, characterID string) (*lottery.TakarakujiTicket, []lottery.TakarakujiTicket, error) {
	if s.getCharacterTakarakujiTicketFn != nil {
		return s.getCharacterTakarakujiTicketFn(ctx, characterID)
	}
	tkt := lottery.TakarakujiTicket{ID: "t-1", RoundID: 1, CharacterID: characterID}
	return &tkt, []lottery.TakarakujiTicket{tkt}, nil
}

func TestLotteryEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Money: 100000}

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
	lService := &stubLotteryService{
		getRaffleTicketsFn: func(_ context.Context, characterID string) (int, error) {
			return 10, nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithLottery(lService),
	)
	router := h.Router()

	t.Run("GET /lottery/takarakuji - public success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/lottery/takarakuji", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/lottery/takarakuji/buy - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/lottery/takarakuji/buy", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/lottery/takarakuji/buy - already purchased conflict", func(t *testing.T) {
		hConflict := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithLottery(&stubLotteryService{
				buyTakarakujiTicketFn: func(ctx context.Context, characterID string) (lottery.TakarakujiPurchaseResult, error) {
					return lottery.TakarakujiPurchaseResult{}, lottery.ErrAlreadyPurchased
				},
			}),
		)
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/lottery/takarakuji/buy", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		hConflict.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/lottery/takarakuji/buy - sold out conflict", func(t *testing.T) {
		hSoldOut := newTestHandler(
			t,
			pService,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithLottery(&stubLotteryService{
				buyTakarakujiTicketFn: func(ctx context.Context, characterID string) (lottery.TakarakujiPurchaseResult, error) {
					return lottery.TakarakujiPurchaseResult{}, lottery.ErrSoldOut
				},
			}),
		)
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/lottery/takarakuji/buy", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		hSoldOut.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /characters/{id}/lottery/takarakuji/ticket - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/lottery/takarakuji/ticket", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Fictional routes removed - 404", func(t *testing.T) {
		req1 := jsonRequest(t, http.MethodPost, "/characters/c1/lottery/buy-ticket", `{"round_id":1,"number":"1234"}`)
		req1.Header.Set("Authorization", "Bearer valid-token")
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for buy-ticket, got %d", rec1.Code)
		}

		req2 := jsonRequest(t, http.MethodPost, "/characters/c1/lottery/claim", `{"ticket_id":"t1"}`)
		req2.Header.Set("Authorization", "Bearer valid-token")
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for claim, got %d", rec2.Code)
		}
	})

	t.Run("GET /characters/{id}/lottery/tickets - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/lottery/tickets", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/lottery/buy-raffle - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/lottery/buy-raffle", `{"count":5}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/lottery/raffle - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/lottery/raffle", `{"raffle_type":"STANDARD"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
