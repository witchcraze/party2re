package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/inn"
)

// InnService defines the inn resting operations exposed over HTTP.
type InnService interface {
	Rest(ctx context.Context, characterID string) (corecharacter.Character, error)
	CalculateFee(level int) int
}

// WithInn configures the inn service for the Handler.
func WithInn(inn InnService) Option {
	return func(h *Handler) {
		h.inn = inn
	}
}

type innRestResponse struct {
	Character characterResponse `json:"character"`
}

func (h *Handler) handleInnRest(w http.ResponseWriter, r *http.Request) {
	if h.inn == nil {
		writeError(w, http.StatusNotImplemented, errors.New("inn service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		updatedChar, err := h.inn.Rest(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, inn.ErrInsufficientFunds) {
				writeError(w, http.StatusUnprocessableEntity, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, innRestResponse{
			Character: toCharacterResponse(updatedChar),
		})
	})
}
