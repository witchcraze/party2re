package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/tavern"
)

// TavernService defines the tavern operations exposed over HTTP.
type TavernService interface {
	GetMenu() []tavern.MenuItem
	GetStatus(ctx context.Context, characterID string) (tavern.TavernStatus, error)
	OrderMeal(ctx context.Context, characterID string, itemID string) (tavern.OrderResult, error)
	ReserveDelivery(ctx context.Context, characterID string, itemID string) (tavern.DeliveryReservation, error)
	GetDelivery(ctx context.Context, characterID string) (tavern.DeliveryReservation, error)
	CancelDelivery(ctx context.Context, characterID string) error
	ClaimDelivery(ctx context.Context, characterID string) (tavern.OrderResult, error)
	Talk(ctx context.Context, characterID string) (tavern.TalkResult, error)
	ResetFullness(ctx context.Context, characterID string) error
}

// WithTavern configures the TavernService for the HTTP handler.
func WithTavern(service TavernService) Option {
	return func(h *Handler) {
		h.tavern = service
	}
}

type tavernItemRequest struct {
	ItemID string `json:"item_id"`
}

func (h *Handler) handleGetTavernMenu(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	items := h.tavern.GetMenu()
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *Handler) handleGetCharacterTavernStatus(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.tavern.GetStatus(r.Context(), char.ID)
		if err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleTavernOrder(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req tavernItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.ItemID == "" {
			writeError(w, http.StatusBadRequest, errors.New("item_id is required"))
			return
		}

		result, err := h.tavern.OrderMeal(r.Context(), char.ID, req.ItemID)
		if err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleTavernReserveDelivery(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req tavernItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.ItemID == "" {
			writeError(w, http.StatusBadRequest, errors.New("item_id is required"))
			return
		}

		reservation, err := h.tavern.ReserveDelivery(r.Context(), char.ID, req.ItemID)
		if err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, reservation)
	})
}

func (h *Handler) handleGetTavernDelivery(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		delivery, err := h.tavern.GetDelivery(r.Context(), char.ID)
		if err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, delivery)
	})
}

func (h *Handler) handleTavernCancelDelivery(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		if err := h.tavern.CancelDelivery(r.Context(), char.ID); err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "delivery reservation cancelled successfully",
		})
	})
}

func (h *Handler) handleTavernClaimDelivery(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.tavern.ClaimDelivery(r.Context(), char.ID)
		if err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleTavernTalk(w http.ResponseWriter, r *http.Request) {
	if h.tavern == nil {
		writeError(w, http.StatusNotImplemented, errors.New("tavern service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		talk, err := h.tavern.Talk(r.Context(), char.ID)
		if err != nil {
			h.writeTavernError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, talk)
	})
}

func (h *Handler) writeTavernError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tavern.ErrInvalidCharacterID):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, tavern.ErrCharacterNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, tavern.ErrMenuItemNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, tavern.ErrInsufficientFunds):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, tavern.ErrAlreadyFull):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, tavern.ErrNoActiveDelivery):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
