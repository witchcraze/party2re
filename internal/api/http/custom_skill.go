package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

type CustomSkillService interface {
	GetCustomSkill(ctx context.Context, characterID string) (*custom_skill.CustomSkill, error)
	SetCustomSkill(ctx context.Context, characterID, name, comment string, gems [3]string) (*custom_skill.CustomSkill, error)
}

func WithCustomSkill(skills CustomSkillService) Option {
	return func(h *Handler) {
		h.customSkills = skills
	}
}

type customSkillRequest struct {
	Name    string    `json:"name"`
	Comment string    `json:"comment"`
	Gems    [3]string `json:"gems"`
	Gem1    string    `json:"gem1,omitempty"`
	Gem2    string    `json:"gem2,omitempty"`
	Gem3    string    `json:"gem3,omitempty"`
}

func (h *Handler) handleGetCustomSkills(w http.ResponseWriter, r *http.Request) {
	if h.customSkills == nil {
		writeError(w, http.StatusNotImplemented, errors.New("custom skill service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		skill, err := h.customSkills.GetCustomSkill(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"custom_skill": skill})
	})
}

func (h *Handler) handleSetCustomSkill(w http.ResponseWriter, r *http.Request) {
	if h.customSkills == nil {
		writeError(w, http.StatusNotImplemented, errors.New("custom skill service not configured"))
		return
	}
	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req customSkillRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		gems := req.Gems
		if req.Gem1 != "" {
			gems[0] = req.Gem1
		}
		if req.Gem2 != "" {
			gems[1] = req.Gem2
		}
		if req.Gem3 != "" {
			gems[2] = req.Gem3
		}
		skill, err := h.customSkills.SetCustomSkill(r.Context(), char.ID, req.Name, req.Comment, gems)
		if err != nil {
			switch {
			case errors.Is(err, custom_skill.ErrInvalidSkillName),
				errors.Is(err, custom_skill.ErrInvalidSkillComment),
				errors.Is(err, custom_skill.ErrTooManyGemSlots),
				errors.Is(err, custom_skill.ErrCMPTooHigh),
				errors.Is(err, custom_skill.ErrGemNotOwned),
				errors.Is(err, custom_skill.ErrGemNotFound):
				writeError(w, http.StatusBadRequest, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"custom_skill": skill})
	})
}
