package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
)

// AlchemyService defines the alchemy operations exposed over HTTP.
type AlchemyService interface {
	GetStatus(ctx context.Context, characterID string) (alchemy.Synthesis, error)
	GetCompendium(ctx context.Context, characterID string) (alchemy.Compendium, error)
	Synthesize(ctx context.Context, characterID string, recipeID string) (alchemy.SynthesisResult, error)
	Claim(ctx context.Context, characterID string) (alchemy.ClaimResult, error)
	LearnRecipe(ctx context.Context, characterID string, pool []string) (alchemy.Recipe, error)
}

// WithAlchemy configures the AlchemyService for the HTTP handler.
func WithAlchemy(service AlchemyService) Option {
	return func(h *Handler) {
		h.alchemy = service
	}
}

type alchemySynthesizeRequest struct {
	RecipeID string `json:"recipe_id"`
}

type alchemyLearnRequest struct {
	Pool []string `json:"pool,omitempty"`
}

type characterAlchemyResponse struct {
	Status     alchemy.Synthesis  `json:"status"`
	Compendium alchemy.Compendium `json:"compendium"`
}

func (h *Handler) handleGetCharacterAlchemy(w http.ResponseWriter, r *http.Request) {
	if h.alchemy == nil {
		writeError(w, http.StatusNotImplemented, errors.New("alchemy service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		st, err := h.alchemy.GetStatus(r.Context(), char.ID)
		if err != nil {
			h.writeAlchemyError(w, err)
			return
		}
		comp, err := h.alchemy.GetCompendium(r.Context(), char.ID)
		if err != nil {
			h.writeAlchemyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, characterAlchemyResponse{
			Status:     st,
			Compendium: comp,
		})
	})
}

func (h *Handler) handleAlchemySynthesize(w http.ResponseWriter, r *http.Request) {
	if h.alchemy == nil {
		writeError(w, http.StatusNotImplemented, errors.New("alchemy service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req alchemySynthesizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request payload"))
			return
		}

		res, err := h.alchemy.Synthesize(r.Context(), char.ID, req.RecipeID)
		if err != nil {
			h.writeAlchemyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleAlchemyClaim(w http.ResponseWriter, r *http.Request) {
	if h.alchemy == nil {
		writeError(w, http.StatusNotImplemented, errors.New("alchemy service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.alchemy.Claim(r.Context(), char.ID)
		if err != nil {
			h.writeAlchemyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleAlchemyLearn(w http.ResponseWriter, r *http.Request) {
	if h.alchemy == nil {
		writeError(w, http.StatusNotImplemented, errors.New("alchemy service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req alchemyLearnRequest
		if r.Body != nil && r.ContentLength > 0 {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		res, err := h.alchemy.LearnRecipe(r.Context(), char.ID, req.Pool)
		if err != nil {
			h.writeAlchemyError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) writeAlchemyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, alchemy.ErrInvalidCharacterID), errors.Is(err, alchemy.ErrInvalidRecipeID):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, alchemy.ErrRecipeNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, alchemy.ErrInsufficientMaterials),
		errors.Is(err, alchemy.ErrRecipeNotLearned),
		errors.Is(err, alchemy.ErrSynthesisInProgress),
		errors.Is(err, alchemy.ErrNoActiveSynthesis),
		errors.Is(err, alchemy.ErrSynthesisNotReady),
		errors.Is(err, alchemy.ErrNoRecipesToLearn):
		writeError(w, http.StatusUnprocessableEntity, err)
	case errors.Is(err, depot.ErrDepotFull):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
