package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/witchcraze/party2re/internal/altar"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// AltarService defines the operations exposed by the Altar of Rebirth over HTTP.
type AltarService interface {
	GetStatus(ctx context.Context, characterID string) (altar.AltarStatus, error)
	Pray(ctx context.Context, characterID string) (altar.PrayResult, error)
	Wish(ctx context.Context, characterID string, itemID string) (altar.WishResult, error)
	OfferOrb(ctx context.Context, characterID string, orb rune) (altar.OfferResult, error)
}

// WithAltar configures the altar service for the Handler.
func WithAltar(a AltarService) Option {
	return func(h *Handler) {
		h.altar = a
	}
}

type altarWishRequest struct {
	ItemID string `json:"item_id"`
}

type altarOfferRequest struct {
	Orb string `json:"orb"`
}

func parseOrbRune(s string) (rune, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "s", "silver", "シルバー", "シルバーオーブ":
		return corecharacter.OrbSilver, nil
	case "r", "red", "レッド", "レッドオーブ":
		return corecharacter.OrbRed, nil
	case "b", "blue", "ブルー", "ブルーオーブ":
		return corecharacter.OrbBlue, nil
	case "g", "green", "グリーン", "グリーンオーブ":
		return corecharacter.OrbGreen, nil
	case "y", "yellow", "イエロー", "イエローオーブ":
		return corecharacter.OrbYellow, nil
	case "p", "purple", "パープル", "パープルオーブ":
		return corecharacter.OrbPurple, nil
	default:
		if len([]rune(s)) == 1 {
			r := []rune(s)[0]
			if corecharacter.IsValidOrbRune(r) {
				return r, nil
			}
		}
		return 0, fmt.Errorf("%w: %s", altar.ErrInvalidOrbRune, s)
	}
}

func (h *Handler) handleGetAltar(w http.ResponseWriter, r *http.Request) {
	if h.altar == nil {
		writeError(w, http.StatusNotImplemented, errors.New("altar service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		status, err := h.altar.GetStatus(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, status)
	})
}

func (h *Handler) handleAltarPray(w http.ResponseWriter, r *http.Request) {
	if h.altar == nil {
		writeError(w, http.StatusNotImplemented, errors.New("altar service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.altar.Pray(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, altar.ErrInsufficientOrbs) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleAltarWish(w http.ResponseWriter, r *http.Request) {
	if h.altar == nil {
		writeError(w, http.StatusNotImplemented, errors.New("altar service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req altarWishRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		result, err := h.altar.Wish(r.Context(), char.ID, req.ItemID)
		if err != nil {
			if errors.Is(err, altar.ErrRamiaNotAwakened) || errors.Is(err, altar.ErrInvalidWishItem) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}

func (h *Handler) handleAltarOffer(w http.ResponseWriter, r *http.Request) {
	if h.altar == nil {
		writeError(w, http.StatusNotImplemented, errors.New("altar service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req altarOfferRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		orbRune, err := parseOrbRune(req.Orb)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		result, err := h.altar.OfferOrb(r.Context(), char.ID, orbRune)
		if err != nil {
			if errors.Is(err, altar.ErrOrbAlreadyOffered) {
				writeJSON(w, http.StatusConflict, result)
				return
			}
			if errors.Is(err, altar.ErrInvalidOrbRune) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, corecharacter.ErrNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, result)
	})
}
