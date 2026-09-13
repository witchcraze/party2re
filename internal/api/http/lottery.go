package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/lottery"
)

// LotteryService defines the raffle and lottery operations exposed over HTTP.
type LotteryService interface {
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
	PlayRaffle(ctx context.Context, characterID string, raffleType lottery.RaffleType) (lottery.RaffleResult, int, corecharacter.Character, error)

	GetTakarakujiStatus(ctx context.Context) (lottery.TakarakujiStatus, error)
	BuyTakarakujiTicket(ctx context.Context, characterID string) (lottery.TakarakujiPurchaseResult, error)
	GetCharacterTakarakujiTicket(ctx context.Context, characterID string) (*lottery.TakarakujiTicket, []lottery.TakarakujiTicket, error)
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

type playRaffleRequest struct {
	RaffleType string `json:"raffle_type"` // "STANDARD" or "SPECIAL"
}

type playRaffleResponse struct {
	Result           lottery.RaffleResult `json:"result"`
	RemainingTickets int                  `json:"remaining_tickets"`
	Character        characterResponse    `json:"character"`
}

type getCharacterTakarakujiTicketResponse struct {
	CurrentTicket *lottery.TakarakujiTicket  `json:"current_ticket,omitempty"`
	History       []lottery.TakarakujiTicket `json:"history"`
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
			if errors.Is(err, depot.ErrDepotFull) {
				writeError(w, http.StatusConflict, err)
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

func (h *Handler) handleGetTakarakujiStatus(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	status, err := h.lottery.GetTakarakujiStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) handleBuyTakarakujiTicket(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.lottery.BuyTakarakujiTicket(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, lottery.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, lottery.ErrAlreadyPurchased) || errors.Is(err, lottery.ErrSoldOut) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleGetCharacterTakarakujiTicket(w http.ResponseWriter, r *http.Request) {
	if h.lottery == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lottery service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		current, history, err := h.lottery.GetCharacterTakarakujiTicket(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, getCharacterTakarakujiTicketResponse{
			CurrentTicket: current,
			History:       history,
		})
	})
}
