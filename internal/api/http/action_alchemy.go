package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// AlchemyService is the Alchemy surface used by the scene, command and entry delivery.
type AlchemyService interface {
	GetStatus(ctx context.Context, characterID string) (alchemy.Synthesis, error)
	GetCompendium(ctx context.Context, characterID string) (alchemy.Compendium, error)
	Synthesize(ctx context.Context, characterID string, recipeID string) (alchemy.SynthesisResult, error)
	Claim(ctx context.Context, characterID string) (alchemy.ClaimResult, error)
}

type alchemySynthesizeRequest struct {
	RecipeID string `json:"recipe_id"`
}

// WithAlchemy configures the scene observation, synthesis command and entry delivery.
func WithAlchemy(service AlchemyService) Option {
	return func(h *Handler) {
		h.alchemy = service
		if service == nil {
			withActionCommand[alchemySynthesizeRequest]("alchemy_synthesize", nil, nil)(h)
			return
		}
		withActionCommand("alchemy_synthesize", func(ctx context.Context, actor string, p alchemySynthesizeRequest) (any, error) {
			return service.Synthesize(ctx, actor, p.RecipeID)
		}, alchemyActionRejection)(h)
	}
}

func alchemyActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, alchemy.ErrInvalidRecipeID):
		return http.StatusBadRequest, ErrorDetail{Code: "ALCHEMY_INVALID_RECIPE", Message: "Recipe ID is invalid."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, alchemy.ErrRecipeNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "ALCHEMY_RECIPE_NOT_FOUND", Message: "Recipe not found."}
	case errors.Is(err, alchemy.ErrRecipeNotLearned):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ALCHEMY_RECIPE_NOT_LEARNED", Message: "Recipe has not been learned."}
	case errors.Is(err, alchemy.ErrInsufficientMaterials):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ALCHEMY_INSUFFICIENT_MATERIALS", Message: "Materials are not in the depot."}
	case errors.Is(err, alchemy.ErrSynthesisInProgress):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "ALCHEMY_SYNTHESIS_IN_PROGRESS", Message: "Another synthesis is in progress."}
	default:
		return 0, ErrorDetail{}
	}
}

// sceneEnterResult keeps the selection fields and adds what entry itself delivered.
type sceneEnterResult struct {
	playercontext.Selection
	AlchemyClaim      *alchemy.ClaimResult `json:"alchemy_claim,omitempty"`
	AlchemyClaimError *ErrorDetail         `json:"alchemy_claim_error,omitempty"`
}

// deliverOnEntry runs after the selection is saved, so a rejected entry never
// claims and a repeated entry cannot deliver twice. GET never reaches it.
func (h *Handler) deliverOnEntry(ctx context.Context, actor string, selection playercontext.Selection) (sceneEnterResult, error) {
	result := sceneEnterResult{Selection: selection}
	if selection.Destination != "alchemy" || h.alchemy == nil {
		return result, nil
	}
	claim, err := h.alchemy.Claim(ctx, actor)
	switch {
	case err == nil:
		result.AlchemyClaim = &claim
	case errors.Is(err, alchemy.ErrNoActiveSynthesis), errors.Is(err, alchemy.ErrSynthesisNotReady):
		// Nothing to deliver: an ordinary entry.
	case errors.Is(err, depot.ErrDepotFull):
		result.AlchemyClaimError = &ErrorDetail{Code: "ALCHEMY_DEPOT_FULL", Message: "Depot is full. Free a slot and enter again to receive the item."}
	default:
		return sceneEnterResult{}, err
	}
	return result, nil
}
