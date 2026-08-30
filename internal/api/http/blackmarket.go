package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// BlackMarketService defines the black market underground trade operations exposed over HTTP.
type BlackMarketService interface {
	GetStatus(ctx context.Context, characterID string, now time.Time) (*blackmarket.ShopStatus, error)
	PurchaseItem(ctx context.Context, characterID string, itemID string, quantity int, now time.Time) (*blackmarket.PurchaseResult, error)
	SellItem(ctx context.Context, characterID string, itemInstanceID string, quantity int, now time.Time) (*blackmarket.SaleResult, error)
	Talk(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	Rumors(ctx context.Context, characterID string, now time.Time) (*blackmarket.RumorsResult, error)
}

// WithBlackMarket configures the BlackMarketService for the HTTP handler.
func WithBlackMarket(service BlackMarketService) Option {
	return func(h *Handler) {
		h.blackmarket = service
	}
}

type blackMarketPurchaseRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

type blackMarketSellRequest struct {
	ItemInstanceID string `json:"item_instance_id"`
	Quantity       int    `json:"quantity"`
}

func (h *Handler) handleGetBlackMarketStatus(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.blackmarket.GetStatus(r.Context(), char.ID, time.Now())
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleBlackMarketPurchase(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req blackMarketPurchaseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.Quantity <= 0 {
			req.Quantity = 1
		}

		result, err := h.blackmarket.PurchaseItem(r.Context(), char.ID, req.ItemID, req.Quantity, time.Now())
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleBlackMarketSell(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req blackMarketSellRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		if req.Quantity <= 0 {
			req.Quantity = 1
		}

		result, err := h.blackmarket.SellItem(r.Context(), char.ID, req.ItemInstanceID, req.Quantity, time.Now())
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleBlackMarketTalk(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.blackmarket.Talk(r.Context(), char.ID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleBlackMarketRumors(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.blackmarket.Rumors(r.Context(), char.ID, time.Now())
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) writeBlackMarketError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blackmarket.ErrAccessDenied):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, blackmarket.ErrCharacterNotFound), errors.Is(err, blackmarket.ErrItemNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, blackmarket.ErrInsufficientFunds),
		errors.Is(err, blackmarket.ErrInvalidQuantity),
		errors.Is(err, blackmarket.ErrDailyLimitExceeded),
		errors.Is(err, blackmarket.ErrUnownedItem),
		errors.Is(err, blackmarket.ErrPriceOverflow):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
