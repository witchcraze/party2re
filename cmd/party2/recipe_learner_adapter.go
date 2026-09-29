package main

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/home"
)

type recipeLearnerAdapter struct {
	alchemy *alchemy.Service
	catalog *item.Catalog
}

func (a *recipeLearnerAdapter) LearnRecipe(ctx context.Context, characterID string, allowedBases []string) (home.LearnedRecipeInfo, error) {
	if a.alchemy == nil {
		return home.LearnedRecipeInfo{}, errors.New("alchemy service not available")
	}

	recipe, err := a.alchemy.LearnRecipe(ctx, characterID, allowedBases)
	if err != nil {
		if errors.Is(err, alchemy.ErrNoRecipesToLearn) {
			return home.LearnedRecipeInfo{}, home.ErrNoRecipesToLearn
		}
		return home.LearnedRecipeInfo{}, err
	}

	var ing1Name, ing2Name string
	if len(recipe.Ingredients) > 0 {
		def1, err := a.catalog.FindByID(recipe.Ingredients[0].DefinitionID)
		if err == nil {
			ing1Name = def1.Name
		} else {
			ing1Name = recipe.Ingredients[0].DefinitionID
		}
	}
	if len(recipe.Ingredients) > 1 {
		def2, err := a.catalog.FindByID(recipe.Ingredients[1].DefinitionID)
		if err == nil {
			ing2Name = def2.Name
		} else {
			ing2Name = recipe.Ingredients[1].DefinitionID
		}
	} else if len(recipe.Ingredients) == 1 {
		ing2Name = ing1Name
	}

	return home.LearnedRecipeInfo{
		ID:                     recipe.ID,
		Name:                   recipe.Name,
		ResultItemDefinitionID: recipe.ResultItemDefinitionID,
		Ingredient1Name:        ing1Name,
		Ingredient2Name:        ing2Name,
	}, nil
}
