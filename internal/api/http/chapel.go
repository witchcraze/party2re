package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/chapel"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// ChapelService defines the chapel blessings and donations operations exposed over HTTP.
type ChapelService interface {
	GetBlessing(ctx context.Context, characterID string) (chapel.CharacterBlessing, error)
	SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error)
	Donate(ctx context.Context, characterID string, goldAmount int) (chapel.CharacterBlessing, error)
}

// WithChapel configures the chapel service for the Handler.
func WithChapel(c ChapelService) Option {
	return func(h *Handler) {
		h.chapel = c
	}
}

type getChapelResponse struct {
	Blessing chapel.CharacterBlessing `json:"blessing"`
}

type prayChapelRequest struct {
	Blessing string `json:"blessing"`
}

type donateChapelRequest struct {
	Amount int `json:"amount"`
}

type chapelBlessingResponse struct {
	Blessing chapel.CharacterBlessing `json:"blessing"`
}

func (h *Handler) handleGetChapel(w http.ResponseWriter, r *http.Request) {
	if h.chapel == nil {
		writeError(w, http.StatusNotImplemented, errors.New("chapel service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		blessing, err := h.chapel.GetBlessing(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, getChapelResponse{
			Blessing: blessing,
		})
	})
}

func (h *Handler) handleChapelPray(w http.ResponseWriter, r *http.Request) {
	if h.chapel == nil {
		writeError(w, http.StatusNotImplemented, errors.New("chapel service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req prayChapelRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		blessingType := chapel.BlessingType(req.Blessing)
		blessing, err := h.chapel.SelectBlessing(r.Context(), char.ID, blessingType)
		if err != nil {
			if errors.Is(err, chapel.ErrInvalidBlessing) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, chapelBlessingResponse{
			Blessing: blessing,
		})
	})
}

func (h *Handler) handleChapelDonate(w http.ResponseWriter, r *http.Request) {
	if h.chapel == nil {
		writeError(w, http.StatusNotImplemented, errors.New("chapel service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req donateChapelRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		if req.Amount <= 0 {
			writeError(w, http.StatusBadRequest, chapel.ErrInvalidDonation)
			return
		}

		blessing, err := h.chapel.Donate(r.Context(), char.ID, req.Amount)
		if err != nil {
			if errors.Is(err, chapel.ErrInsufficientGold) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			if errors.Is(err, chapel.ErrInvalidDonation) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, chapelBlessingResponse{
			Blessing: blessing,
		})
	})
}
