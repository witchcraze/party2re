package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/blackmarket"
)

// BlackMarketService defines the black market observation, commands and retained NPC reads.
type BlackMarketService interface {
	GetStatus(ctx context.Context, characterID string) (*blackmarket.Status, error)
	SacrificeItem(ctx context.Context, characterID string, itemInstanceID string) (*blackmarket.SacrificeResult, error)
	TradePrize(ctx context.Context, characterID string, prizeID string) (*blackmarket.TradeResult, error)
}

func (h *Handler) writeBlackMarketError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blackmarket.ErrAccessDenied):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, blackmarket.ErrCharacterNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
