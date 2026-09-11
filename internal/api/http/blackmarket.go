package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// BlackMarketService defines the black market underground trade operations exposed over HTTP.
type BlackMarketService interface {
	GetStatus(ctx context.Context, characterID string) (*blackmarket.Status, error)
	Talk(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	Inspect(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	SacrificeItem(ctx context.Context, characterID string, itemInstanceID string) (*blackmarket.SacrificeResult, error)
	TradePrize(ctx context.Context, characterID string, prizeID string) (*blackmarket.TradeResult, error)
}

// WithBlackMarket configures the BlackMarketService for the HTTP handler.
func WithBlackMarket(service BlackMarketService) Option {
	return func(h *Handler) {
		h.blackmarket = service
	}
}

type blackMarketSacrificeRequest struct {
	ItemInstanceID string `json:"item_instance_id"`
}

type blackMarketTradeRequest struct {
	PrizeID string `json:"prize_id"`
}

func (h *Handler) handleGetBlackMarketStatus(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.blackmarket.GetStatus(r.Context(), char.ID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleGetBlackMarketPoints(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.blackmarket.GetStatus(r.Context(), char.ID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
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

func (h *Handler) handleBlackMarketInspect(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.blackmarket.Inspect(r.Context(), char.ID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleBlackMarketSacrifice(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req blackMarketSacrificeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		result, err := h.blackmarket.SacrificeItem(r.Context(), char.ID, req.ItemInstanceID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleBlackMarketTrade(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req blackMarketTradeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		result, err := h.blackmarket.TradePrize(r.Context(), char.ID, req.PrizeID)
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
	case errors.Is(err, blackmarket.ErrCharacterNotFound),
		errors.Is(err, blackmarket.ErrPrizeNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, blackmarket.ErrUnownedItem),
		errors.Is(err, blackmarket.ErrNotSacrificeEligible),
		errors.Is(err, blackmarket.ErrInsufficientRarePoints),
		errors.Is(err, blackmarket.ErrInsufficientURarePoints),
		errors.Is(err, blackmarket.ErrDepotFull):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, blackmarket.ErrDepotNotConfigured):
		writeError(w, http.StatusInternalServerError, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
