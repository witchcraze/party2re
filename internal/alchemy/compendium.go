package alchemy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoRecipesToLearn = errors.New("no more recipes can be learned from this book")
)

type DiscoveredRecipe struct {
	RecipeID     string     `json:"recipe_id"`
	IsCrafted    bool       `json:"is_crafted"`
	DiscoveredAt time.Time  `json:"discovered_at"`
	CraftedAt    *time.Time `json:"crafted_at,omitempty"`
}

type CompendiumEntry struct {
	RecipeID               string       `json:"recipe_id"`
	Name                   string       `json:"name"`
	ResultItemDefinitionID string       `json:"result_item_definition_id"`
	ResultItemName         string       `json:"result_item_name"`
	Ingredients            []Ingredient `json:"ingredients"`
	IsLearned              bool         `json:"is_learned"`
	IsCrafted              bool         `json:"is_crafted"`
}

type Compendium struct {
	TotalRecipes         int               `json:"total_recipes"`
	LearnedCount         int               `json:"learned_count"`
	CraftedCount         int               `json:"crafted_count"`
	CompletionPercentage int               `json:"completion_percentage"`
	CompAlc              bool              `json:"comp_alc"`
	Entries              []CompendiumEntry `json:"entries"`
}

func (s *Service) UnlockRecipe(ctx context.Context, characterID string, recipeID string) error {
	characterID = strings.TrimSpace(characterID)
	recipeID = strings.TrimSpace(recipeID)
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if recipeID == "" {
		return ErrInvalidRecipeID
	}

	if _, err := s.recipes.FindByID(recipeID); err != nil {
		return err
	}
	return s.repo.SaveDiscoveredRecipe(ctx, characterID, recipeID, false)
}

func (s *Service) LearnRecipe(ctx context.Context, characterID string, allowedBases []string) (Recipe, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Recipe{}, ErrInvalidCharacterID
	}

	allRecipes := s.recipes.All()
	discovered, err := s.repo.GetDiscoveredRecipes(ctx, characterID)
	if err != nil {
		return Recipe{}, err
	}
	discoveredMap := make(map[string]bool, len(discovered))
	for _, d := range discovered {
		discoveredMap[d.RecipeID] = true
	}

	baseFilter := make(map[string]bool)
	for _, b := range allowedBases {
		b = strings.TrimSpace(b)
		if b != "" {
			baseFilter[b] = true
		}
	}

	var candidates []Recipe
	for _, r := range allRecipes {
		if discoveredMap[r.ID] {
			continue
		}
		if len(baseFilter) > 0 {
			match := false
			for _, ing := range r.Ingredients {
				if baseFilter[ing.DefinitionID] {
					match = true
					break
				}
				if def, err := s.items.FindByID(ing.DefinitionID); err == nil {
					if baseFilter[def.Name] {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}
		candidates = append(candidates, r)
	}

	if len(candidates) == 0 {
		return Recipe{}, ErrNoRecipesToLearn
	}

	s.rngMu.Lock()
	chosen := candidates[s.rng.Intn(len(candidates))]
	s.rngMu.Unlock()

	if err := s.repo.SaveDiscoveredRecipe(ctx, characterID, chosen.ID, false); err != nil {
		return Recipe{}, fmt.Errorf("save discovered recipe: %w", err)
	}

	return chosen, nil
}

func (s *Service) GetCompendium(ctx context.Context, characterID string) (Compendium, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Compendium{}, ErrInvalidCharacterID
	}

	discovered, err := s.repo.GetDiscoveredRecipes(ctx, characterID)
	if err != nil {
		return Compendium{}, err
	}
	discoveredMap := make(map[string]DiscoveredRecipe, len(discovered))
	for _, d := range discovered {
		discoveredMap[d.RecipeID] = d
	}

	all := s.recipes.All()
	entries := make([]CompendiumEntry, 0, len(all))
	craftedCount := 0
	learnedCount := len(discovered)

	for _, r := range all {
		d, isLearned := discoveredMap[r.ID]
		isCrafted := isLearned && d.IsCrafted
		if isCrafted {
			craftedCount++
		}

		resultItemName := "？？？"
		if isCrafted {
			if def, err := s.items.FindByID(r.ResultItemDefinitionID); err == nil {
				resultItemName = def.Name
			} else {
				resultItemName = r.ResultItemDefinitionID
			}
		}

		entries = append(entries, CompendiumEntry{
			RecipeID:               r.ID,
			Name:                   r.Name,
			ResultItemDefinitionID: r.ResultItemDefinitionID,
			ResultItemName:         resultItemName,
			Ingredients:            r.Ingredients,
			IsLearned:              isLearned,
			IsCrafted:              isCrafted,
		})
	}

	pct := 0
	if len(all) > 0 {
		pct = (craftedCount * 100) / len(all)
	}

	state, _ := s.repo.GetSynthesisState(ctx, characterID)

	return Compendium{
		TotalRecipes:         len(all),
		LearnedCount:         learnedCount,
		CraftedCount:         craftedCount,
		CompletionPercentage: pct,
		CompAlc:              state.CompAlc,
		Entries:              entries,
	}, nil
}
