package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// BankService defines bank operations exposed over HTTP.
type BankService interface {
	GetState(ctx context.Context, characterID string) (bank.State, error)
	Deposit(ctx context.Context, characterID string, amount int64) (bank.DepositResult, error)
	Withdraw(ctx context.Context, characterID string, amount int64) (bank.WithdrawResult, error)
	InspectNPC() bank.NPCInfo
	TalkNPC() string
}

// WithBank configures the bank service for the Handler.
func WithBank(b BankService) Option {
	return func(h *Handler) {
		h.bank = b
	}
}

type bankDepositRequest struct {
	Amount int64 `json:"amount"`
}

type bankWithdrawRequest struct {
	Amount int64 `json:"amount"`
}

func mapBankHTTPError(w http.ResponseWriter, err error) {
	if errors.Is(err, corecharacter.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if errors.Is(err, bank.ErrInvalidCharacterID) ||
		errors.Is(err, bank.ErrInvalidAmount) ||
		errors.Is(err, bank.ErrInsufficientFunds) ||
		errors.Is(err, bank.ErrInsufficientBalance) ||
		errors.Is(err, bank.ErrDepositLimitExceeded) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func (h *Handler) handleGetBankState(w http.ResponseWriter, r *http.Request) {
	if h.bank == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bank service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		state, err := h.bank.GetState(r.Context(), char.ID)
		if err != nil {
			mapBankHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})
}

func (h *Handler) handleBankDeposit(w http.ResponseWriter, r *http.Request) {
	if h.bank == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bank service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req bankDepositRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}
		res, err := h.bank.Deposit(r.Context(), char.ID, req.Amount)
		if err != nil {
			mapBankHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleBankWithdraw(w http.ResponseWriter, r *http.Request) {
	if h.bank == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bank service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req bankWithdrawRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}
		res, err := h.bank.Withdraw(r.Context(), char.ID, req.Amount)
		if err != nil {
			mapBankHTTPError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleBankInspectNPC(w http.ResponseWriter, r *http.Request) {
	if h.bank == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bank service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		info := h.bank.InspectNPC()
		writeJSON(w, http.StatusOK, info)
	})
}

func (h *Handler) handleBankTalkNPC(w http.ResponseWriter, r *http.Request) {
	if h.bank == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bank service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dialogue := h.bank.TalkNPC()
		writeJSON(w, http.StatusOK, map[string]string{"message": dialogue})
	})
}
