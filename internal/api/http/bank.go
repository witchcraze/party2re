package http

import (
	"context"
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
