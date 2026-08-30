package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/secretshop"
)

// SecretShopService defines the secret underground shop operations exposed over HTTP.
type SecretShopService interface {
	GetShopStatus(ctx context.Context, characterID string) (*secretshop.ShopStatus, error)
	Talk(ctx context.Context, characterID string) (string, error)
	Inspect(ctx context.Context, characterID string) (string, error)
	PuffPuff(ctx context.Context, characterID string) (*secretshop.PuffPuffResult, error)
	PurchaseItem(ctx context.Context, characterID string, itemID string, quantity int) (*secretshop.PurchaseResult, error)
}

// WithSecretShop configures the SecretShopService for the HTTP handler.
func WithSecretShop(service SecretShopService) Option {
	return func(h *Handler) {
		h.secretshop = service
	}
}

type secretShopPurchaseRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

type secretShopDialogueResponse struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Message     string `json:"message"`
}

func (h *Handler) handleGetSecretShop(w http.ResponseWriter, r *http.Request) {
	if h.secretshop == nil {
		writeError(w, http.StatusNotImplemented, errors.New("secret shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.secretshop.GetShopStatus(r.Context(), char.ID)
		if err != nil {
			h.writeSecretShopError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleSecretShopTalk(w http.ResponseWriter, r *http.Request) {
	if h.secretshop == nil {
		writeError(w, http.StatusNotImplemented, errors.New("secret shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		dialogue, err := h.secretshop.Talk(r.Context(), char.ID)
		if err != nil {
			h.writeSecretShopError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, secretShopDialogueResponse{
			CharacterID: char.ID,
			NPCName:     secretshop.NPCName,
			Message:     dialogue,
		})
	})
}

func (h *Handler) handleSecretShopInspect(w http.ResponseWriter, r *http.Request) {
	if h.secretshop == nil {
		writeError(w, http.StatusNotImplemented, errors.New("secret shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		description, err := h.secretshop.Inspect(r.Context(), char.ID)
		if err != nil {
			h.writeSecretShopError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, secretShopDialogueResponse{
			CharacterID: char.ID,
			NPCName:     secretshop.NPCName,
			Message:     description,
		})
	})
}

func (h *Handler) handleSecretShopPuffPuff(w http.ResponseWriter, r *http.Request) {
	if h.secretshop == nil {
		writeError(w, http.StatusNotImplemented, errors.New("secret shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.secretshop.PuffPuff(r.Context(), char.ID)
		if err != nil {
			h.writeSecretShopError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleSecretShopPurchase(w http.ResponseWriter, r *http.Request) {
	if h.secretshop == nil {
		writeError(w, http.StatusNotImplemented, errors.New("secret shop service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req secretShopPurchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.Quantity <= 0 {
			req.Quantity = 1
		}

		result, err := h.secretshop.PurchaseItem(r.Context(), char.ID, req.ItemID, req.Quantity)
		if err != nil {
			h.writeSecretShopError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) writeSecretShopError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, secretshop.ErrAccessDenied):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, secretshop.ErrCharacterNotFound), errors.Is(err, secretshop.ErrItemNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, secretshop.ErrInsufficientFunds),
		errors.Is(err, secretshop.ErrInvalidQuantity),
		errors.Is(err, secretshop.ErrPriceOverflow):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
