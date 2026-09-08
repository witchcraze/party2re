package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/witchcraze/party2re/internal/chapel"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// ChapelService defines the chapel blessings operations exposed over HTTP.
type ChapelService interface {
	GetBlessing(ctx context.Context, characterID string) (chapel.CharacterBlessing, error)
	GetStatus(ctx context.Context, characterID, characterName string) (chapel.ChapelStatus, error)
	SelectBlessing(ctx context.Context, characterID string, blessing chapel.BlessingType) (chapel.CharacterBlessing, error)
	ClearBlessing(ctx context.Context, characterID string) error
}

// WithChapel configures the chapel service for the Handler.
func WithChapel(c ChapelService) Option {
	return func(h *Handler) {
		h.chapel = c
	}
}

type getChapelResponse struct {
	Blessing chapel.CharacterBlessing `json:"blessing"`
	Status   chapel.ChapelStatus      `json:"status"`
}

type prayChapelRequest struct {
	Blessing string `json:"blessing"`
}

type chapelBlessingResponse struct {
	Blessing chapel.CharacterBlessing `json:"blessing"`
	Message  string                   `json:"message,omitempty"`
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

		status, err := h.chapel.GetStatus(r.Context(), char.ID, char.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, getChapelResponse{
			Blessing: blessing,
			Status:   status,
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

		blessingType, err := chapel.ParseBlessingType(req.Blessing)
		if err != nil || blessingType == chapel.BlessingNone {
			writeError(w, http.StatusBadRequest, chapel.ErrInvalidBlessing)
			return
		}

		blessing, err := h.chapel.SelectBlessing(r.Context(), char.ID, blessingType)
		if err != nil {
			if errors.Is(err, chapel.ErrInvalidBlessing) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, chapel.ErrAlreadyPrayed) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		var blessingName string
		for _, info := range chapel.AvailableBlessings {
			if info.Type == blessing.ActiveBlessing {
				blessingName = info.Name
				break
			}
		}

		writeJSON(w, http.StatusOK, chapelBlessingResponse{
			Blessing: blessing,
			Message:  fmt.Sprintf("%sは「%s」と祈るのですね…", char.Name, blessingName),
		})
	})
}
