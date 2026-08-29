package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/lottery"
)

// LotteryService defines the raffle and lottery operations exposed over HTTP.
type LotteryService interface {
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
	ListLotteryTickets(ctx context.Context, characterID string, roundID int) ([]lottery.LotteryTicket, error)
	BuyRaffleTickets(ctx context.Context, characterID string, count int) (int, corecharacter.Character, error)
	PlayRaffle(ctx context.Context, characterID string, raffleType lottery.RaffleType) (lottery.RaffleResult, int, corecharacter.Character, error)
	PurchaseLotteryTicket(ctx context.Context, characterID string, roundID int, number string) (lottery.LotteryTicket, corecharacter.Character, error)
	ClaimLotteryTicket(ctx context.Context, characterID, ticketID string) (lottery.LotteryTicket, corecharacter.Character, error)
}

// WithLottery configures the lottery service for the Handler.
func WithLottery(l LotteryService) Option {
	return func(h *Handler) {
		h.lottery = l
	}
}

type getLotteryTicketsResponse struct {
	Tickets int `json:"tickets"`
}

type buyRaffleRequest struct {
	Count int `json:"count"`
}

type buyRaffleResponse struct {
	Tickets   int               `json:"tickets"`
	Character characterResponse `json:"character"`
}

type playRaffleRequest struct {
	RaffleType string `json:"raffle_type"` // "STANDARD" or "SPECIAL"
}

type playRaffleResponse struct {
	Result           lottery.RaffleResult `json:"result"`
	RemainingTickets int                  `json:"remaining_tickets"`
	Character        characterResponse    `json:"character"`
}

type buyLotteryTicketRequest struct {
	RoundID int    `json:"round_id"`
	Number  string `json:"number"`
}

type buyLotteryTicketResponse struct {
	Ticket    lottery.LotteryTicket `json:"ticket"`
	Character characterResponse     `json:"character"`
}

type claimLotteryTicketRequest struct {
	TicketID string `json:"ticket_id"`
}

type claimLotteryTicketResponse struct {
	Ticket    lottery.LotteryTicket `json:"ticket"`
	Character characterResponse     `json:"character"`
}

func (h *Handler) handleGetLotteryTickets(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		tickets, err := h.lottery.GetRaffleTickets(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, getLotteryTicketsResponse{
			Tickets: tickets,
		})
	})
}

func (h *Handler) handleBuyRaffleTickets(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req buyRaffleRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Count <= 0 {
			writeError(w, http.StatusBadRequest, lottery.ErrInvalidAmount)
			return
		}

		tickets, updatedChar, err := h.lottery.BuyRaffleTickets(r.Context(), char.ID, req.Count)
		if err != nil {
			if errors.Is(err, lottery.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, lottery.ErrInvalidAmount) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, buyRaffleResponse{
			Tickets:   tickets,
			Character: toCharacterResponse(updatedChar),
		})
	})
}

func (h *Handler) handlePlayRaffle(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req playRaffleRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		raffleType := lottery.RaffleType(req.RaffleType)
		if raffleType != lottery.RaffleStandard && raffleType != lottery.RaffleSpecial {
			raffleType = lottery.RaffleStandard
		}

		res, remaining, updatedChar, err := h.lottery.PlayRaffle(r.Context(), char.ID, raffleType)
		if err != nil {
			if errors.Is(err, lottery.ErrInsufficientTickets) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, playRaffleResponse{
			Result:           res,
			RemainingTickets: remaining,
			Character:        toCharacterResponse(updatedChar),
		})
	})
}

func (h *Handler) handleBuyLotteryTicket(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req buyLotteryTicketRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.RoundID <= 0 || req.Number == "" {
			writeError(w, http.StatusBadRequest, errors.New("round_id and 4-digit number are required"))
			return
		}

		ticket, updatedChar, err := h.lottery.PurchaseLotteryTicket(r.Context(), char.ID, req.RoundID, req.Number)
		if err != nil {
			if errors.Is(err, lottery.ErrInvalidTicketNumber) || errors.Is(err, lottery.ErrInvalidAmount) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, lottery.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, buyLotteryTicketResponse{
			Ticket:    ticket,
			Character: toCharacterResponse(updatedChar),
		})
	})
}

func (h *Handler) handleClaimLotteryTicket(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req claimLotteryTicketRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.TicketID == "" {
			writeError(w, http.StatusBadRequest, errors.New("ticket_id is required"))
			return
		}

		ticket, updatedChar, err := h.lottery.ClaimLotteryTicket(r.Context(), char.ID, req.TicketID)
		if err != nil {
			if errors.Is(err, lottery.ErrTicketNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, lottery.ErrTicketAlreadyClaimed) || errors.Is(err, lottery.ErrDrawingNotSettled) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, claimLotteryTicketResponse{
			Ticket:    ticket,
			Character: toCharacterResponse(updatedChar),
		})
	})
}
