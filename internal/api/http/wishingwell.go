package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

// WishingWellService defines the Wishing Well operations exposed over HTTP.
type WishingWellService interface {
	GetStatus(ctx context.Context, characterID string) (wishingwell.WishingWellStatus, error)
	Exchange(ctx context.Context, req wishingwell.ExchangeRequest) (wishingwell.ExchangeResult, error)
}

// WithWishingWell configures the wishing well service for the Handler.
func WithWishingWell(w WishingWellService) Option {
	return func(h *Handler) {
		h.wishingWell = w
	}
}

type wishingWellExchangeRequest struct {
	Stat string `json:"stat"`
	SP   int    `json:"sp"`
}

func (h *Handler) handleGetWishingWell(w http.ResponseWriter, r *http.Request) {
	if h.wishingWell == nil {
		writeError(w, http.StatusNotImplemented, errors.New("wishing well service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.wishingWell.GetStatus(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleWishingWellExchange(w http.ResponseWriter, r *http.Request) {
	if h.wishingWell == nil {
		writeError(w, http.StatusNotImplemented, errors.New("wishing well service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req wishingWellExchangeRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		res, err := h.wishingWell.Exchange(r.Context(), wishingwell.ExchangeRequest{
			CharacterID: char.ID,
			Stat:        req.Stat,
			SP:          req.SP,
		})
		if err != nil {
			if errors.Is(err, wishingwell.ErrInvalidSPAmount) ||
				errors.Is(err, wishingwell.ErrInsufficientSP) ||
				errors.Is(err, wishingwell.ErrJobMemoryActive) ||
				errors.Is(err, wishingwell.ErrOverLevelRestricted) ||
				errors.Is(err, wishingwell.ErrInvalidTargetStat) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, res)
	})
}
