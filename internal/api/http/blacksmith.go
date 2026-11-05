package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/blacksmith"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// BlacksmithService defines the blacksmith operations exposed over HTTP.
type BlacksmithService interface {
	ApplySeal(ctx context.Context, characterID string, sealID int) (blacksmith.Seal, error)
	NameEquipment(ctx context.Context, characterID string, target string, customName string) error
	DepositWeapon(ctx context.Context, characterID string) (blacksmith.Deposit, error)
	WithdrawWeapon(ctx context.Context, characterID string, slot int) error
	ListDeposits(ctx context.Context, characterID string) ([]blacksmith.Deposit, error)
}

func (h *Handler) handleGetBlacksmithSeals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"seals": blacksmith.Seals,
	})
}

func (h *Handler) handleGetBlacksmithStorage(w http.ResponseWriter, r *http.Request) {
	if h.blacksmith == nil {
		writeError(w, http.StatusNotImplemented, errors.New("blacksmith service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		deposits, err := h.blacksmith.ListDeposits(r.Context(), char.ID)
		if err != nil {
			h.writeBlacksmithError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"deposits": deposits,
		})
	})
}

func (h *Handler) writeBlacksmithError(w http.ResponseWriter, err error) {
	status, _ := blacksmithActionRejection(err)
	if status == 0 {
		status = http.StatusInternalServerError
	}
	writeError(w, status, err)
}
