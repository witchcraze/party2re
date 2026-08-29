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
	getRaffleTicketsFn      func(ctx context.Context, characterID string) (int, error)
	listLotteryTicketsFn    func(ctx context.Context, characterID string, roundID int) ([]lottery.LotteryTicket, error)
	buyRaffleTicketsFn      func(ctx context.Context, characterID string, count int) (int, corecharacter.Character, error)
	playRaffleFn            func(ctx context.Context, characterID string, raffleType lottery.RaffleType) (lottery.RaffleResult, int, corecharacter.Character, error)
	purchaseLotteryTicketFn func(ctx context.Context, characterID string, roundID int, number string) (lottery.LotteryTicket, corecharacter.Character, error)
	claimLotteryTicketFn    func(ctx context.Context, characterID, ticketID string) (lottery.LotteryTicket, corecharacter.Character, error)
}

func (s *stubLotteryService) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	if s.getRaffleTicketsFn != nil {
		return s.getRaffleTicketsFn(ctx, characterID)
	}
	return 0, nil
}

func (s *stubLotteryService) ListLotteryTickets(ctx context.Context, characterID string, roundID int) ([]lottery.LotteryTicket, error) {
	if s.listLotteryTicketsFn != nil {
		return s.listLotteryTicketsFn(ctx, characterID, roundID)
	}
	return nil, nil
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

func (s *stubLotteryService) PurchaseLotteryTicket(ctx context.Context, characterID string, roundID int, number string) (lottery.LotteryTicket, corecharacter.Character, error) {
	if s.purchaseLotteryTicketFn != nil {
		return s.purchaseLotteryTicketFn(ctx, characterID, roundID, number)
	}
	return lottery.LotteryTicket{ID: "t1", CharacterID: characterID, RoundID: roundID, TicketNumber: number, PurchasedAt: time.Now()}, corecharacter.Character{ID: characterID}, nil
}

func (s *stubLotteryService) ClaimLotteryTicket(ctx context.Context, characterID, ticketID string) (lottery.LotteryTicket, corecharacter.Character, error) {
	if s.claimLotteryTicketFn != nil {
		return s.claimLotteryTicketFn(ctx, characterID, ticketID)
	}
	return lottery.LotteryTicket{ID: ticketID, CharacterID: characterID, Claimed: true}, corecharacter.Character{ID: characterID}, nil
}

func TestLotteryEndpoints(t *testing.T) {
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

	t.Run("POST /characters/{id}/lottery/buy-ticket - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/lottery/buy-ticket", `{"round_id":1,"number":"1234"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/lottery/claim - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/lottery/claim", `{"ticket_id":"t1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
