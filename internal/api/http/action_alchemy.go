package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// AlchemyService is the Alchemy surface used by the scene, command and entry delivery.
type AlchemyService interface {
	GetStatus(ctx context.Context, characterID string) (alchemy.Synthesis, error)
	GetCompendium(ctx context.Context, characterID string) (alchemy.Compendium, error)
	ViewRecipes(ctx context.Context, characterID string) (alchemy.RecipeListResult, error)
	Synthesize(ctx context.Context, characterID string, recipeID string) (alchemy.SynthesisResult, error)
	Claim(ctx context.Context, characterID string) (alchemy.ClaimResult, error)
}

type alchemySynthesizeRequest struct {
	RecipeID string `json:"recipe_id"`
}

type alchemyRecipesRequest struct{}

// WithAlchemy configures the scene observation, synthesis and recipe-list commands and entry delivery.
func WithAlchemy(service AlchemyService) Option {
	return func(h *Handler) {
		h.alchemy = service
		if service == nil {
			withActionCommand[alchemySynthesizeRequest]("alchemy_synthesize", nil, nil)(h)
			withActionCommand[alchemyRecipesRequest]("alchemy_recipes", nil, nil)(h)
			return
		}
		withActionCommand("alchemy_synthesize", func(ctx context.Context, actor string, p alchemySynthesizeRequest) (any, error) {
			return service.Synthesize(ctx, actor, p.RecipeID)
		}, alchemyActionRejection)(h)
		withActionCommand("alchemy_recipes", func(ctx context.Context, actor string, _ alchemyRecipesRequest) (any, error) {
			return service.ViewRecipes(ctx, actor)
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
