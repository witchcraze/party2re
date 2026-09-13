package http

import (
	"context"
	"encoding/json"
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

// WithBlacksmith configures the BlacksmithService for the HTTP handler.
func WithBlacksmith(service BlacksmithService) Option {
	return func(h *Handler) {
		h.blacksmith = service
	}
}

type applySealRequest struct {
	SealID int `json:"seal_id"`
}

type nameEquipmentRequest struct {
	Target string `json:"target"`
	Name   string `json:"name"`
}

type withdrawWeaponRequest struct {
	Slot int `json:"slot"`
}

func (h *Handler) handleGetBlacksmithSeals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"seals": blacksmith.Seals,
	})
}

func (h *Handler) handleApplyBlacksmithSeal(w http.ResponseWriter, r *http.Request) {
	if h.blacksmith == nil {
		writeError(w, http.StatusNotImplemented, errors.New("blacksmith service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req applySealRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		seal, err := h.blacksmith.ApplySeal(r.Context(), char.ID, req.SealID)
		if err != nil {
			h.writeBlacksmithError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"seal":    seal,
			"message": "よっしゃ！武器に【" + seal.Name + "】を刻印したぜ！",
		})
	})
}

func (h *Handler) handleNameEquipment(w http.ResponseWriter, r *http.Request) {
	if h.blacksmith == nil {
		writeError(w, http.StatusNotImplemented, errors.New("blacksmith service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req nameEquipmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if err := h.blacksmith.NameEquipment(r.Context(), char.ID, req.Target, req.Name); err != nil {
			h.writeBlacksmithError(w, err)
			return
		}

		targetLabel := "武器"
		if req.Target == "armor" || req.Target == "arm" || req.Target == "防具" {
			targetLabel = "防具"
		}
		msg := targetLabel + "を" + req.Name + "と名づけたぜ"
		if req.Name == "" {
			msg = targetLabel + "の名前を元に戻したぜ"
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": msg,
		})
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

func (h *Handler) handleDepositBlacksmithWeapon(w http.ResponseWriter, r *http.Request) {
	if h.blacksmith == nil {
		writeError(w, http.StatusNotImplemented, errors.New("blacksmith service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dep, err := h.blacksmith.DepositWeapon(r.Context(), char.ID)
		if err != nil {
			h.writeBlacksmithError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"deposit": dep,
			"message": "武器をお預かっておくぜ",
		})
	})
}

func (h *Handler) handleWithdrawBlacksmithWeapon(w http.ResponseWriter, r *http.Request) {
	if h.blacksmith == nil {
		writeError(w, http.StatusNotImplemented, errors.New("blacksmith service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req withdrawWeaponRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if err := h.blacksmith.WithdrawWeapon(r.Context(), char.ID, req.Slot); err != nil {
			h.writeBlacksmithError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "武器を返すぜ",
		})
	})
}

func (h *Handler) writeBlacksmithError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blacksmith.ErrDepositNotFound), errors.Is(err, corecharacter.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, blacksmith.ErrInvalidCharacterID),
		errors.Is(err, blacksmith.ErrNoWeaponEquipped),
		errors.Is(err, blacksmith.ErrNoArmorEquipped),
		errors.Is(err, blacksmith.ErrInvalidSeal),
		errors.Is(err, blacksmith.ErrSealNotApplicable),
		errors.Is(err, blacksmith.ErrInsufficientCrystals),
		errors.Is(err, blacksmith.ErrStorageFull),
		errors.Is(err, blacksmith.ErrDuplicateStoredName),
		errors.Is(err, blacksmith.ErrWeaponSlotOccupied),
		errors.Is(err, blacksmith.ErrInvalidSlot),
		errors.Is(err, blacksmith.ErrInvalidTarget),
		errors.Is(err, blacksmith.ErrNameWhitespace),
		errors.Is(err, blacksmith.ErrNameForbiddenChar),
		errors.Is(err, blacksmith.ErrNameAtSymbol),
		errors.Is(err, blacksmith.ErrNameTooLong),
		errors.Is(err, blacksmith.ErrEmptyName):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
