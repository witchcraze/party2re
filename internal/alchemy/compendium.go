package alchemy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
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

// CompendiumEntry is the observation of one learned combination. The product
// fields (Name, ResultItemDefinitionID, ResultItemName) are empty and omitted
// until the character has claimed that recipe once.
type CompendiumEntry struct {
	RecipeID               string       `json:"recipe_id"`
	Name                   string       `json:"name,omitempty"`
	ResultItemDefinitionID string       `json:"result_item_definition_id,omitempty"`
	ResultItemName         string       `json:"result_item_name,omitempty"`
	Ingredients            []Ingredient `json:"ingredients"`
	IsLearned              bool         `json:"is_learned"`
	IsCrafted              bool         `json:"is_crafted"`
}

// RecipeView is the observation projection of a Recipe for a character. The
// combination (ID, Ingredients) is always visible; the product facts (Name,
// result item and quantity) are omitted until the character has crafted it.
type RecipeView struct {
	ID                     string       `json:"id"`
	Name                   string       `json:"name,omitempty"`
	ResultItemDefinitionID string       `json:"result_item_definition_id,omitempty"`
	ResultQuantity         int          `json:"result_quantity,omitempty"`
	Ingredients            []Ingredient `json:"ingredients"`
}

func viewRecipe(r Recipe, crafted bool) RecipeView {
	v := RecipeView{ID: r.ID, Ingredients: r.Ingredients}
	if crafted {
		v.Name = r.Name
		v.ResultItemDefinitionID = r.ResultItemDefinitionID
		v.ResultQuantity = r.ResultQuantity
	}
	return v
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

	byBase := make(map[string][]Recipe)
	var baseKeys []string

	for _, r := range allRecipes {
		if discoveredMap[r.ID] {
			continue
		}
		if len(r.Ingredients) == 0 {
			continue
		}
		baseID := r.Ingredients[0].DefinitionID
		if len(baseFilter) > 0 {
			match := baseFilter[baseID]
			if !match && s.items != nil {
				if def, err := s.items.FindByID(baseID); err == nil {
					match = baseFilter[def.Name]
				}
			}
			if !match {
				continue
			}
		}
		if len(byBase[baseID]) == 0 {
			baseKeys = append(baseKeys, baseID)
		}
		byBase[baseID] = append(byBase[baseID], r)
	}

	if len(baseKeys) == 0 {
		return Recipe{}, ErrNoRecipesToLearn
	}

	slices.Sort(baseKeys)

	s.rngMu.Lock()
	chosenBase := baseKeys[s.rng.Intn(len(baseKeys))]
	recipesForBase := byBase[chosenBase]
	chosen := recipesForBase[s.rng.Intn(len(recipesForBase))]
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
	entries := make([]CompendiumEntry, 0, len(discovered))
	craftedCount := 0
	learnedCount := len(discovered)

	for _, r := range all {
		d, isLearned := discoveredMap[r.ID]
		if !isLearned {
			continue
		}
		entry := CompendiumEntry{
			RecipeID:    r.ID,
			Ingredients: r.Ingredients,
			IsLearned:   true,
			IsCrafted:   d.IsCrafted,
		}
		if d.IsCrafted {
			craftedCount++
			entry.Name = r.Name
			entry.ResultItemDefinitionID = r.ResultItemDefinitionID
			entry.ResultItemName = r.ResultItemDefinitionID
			if def, err := s.items.FindByID(r.ResultItemDefinitionID); err == nil {
				entry.ResultItemName = def.Name
			}
		}
		entries = append(entries, entry)
	}

	pct := 0
	if len(all) > 0 {
		pct = (craftedCount * 100) / len(all)
	}

	state, err := s.repo.GetSynthesisState(ctx, characterID)
	if err != nil {
		return Compendium{}, fmt.Errorf("get synthesis state: %w", err)
	}

	return Compendium{
		TotalRecipes:         len(all),
		LearnedCount:         learnedCount,
		CraftedCount:         craftedCount,
		CompletionPercentage: pct,
		CompAlc:              state.CompAlc,
		Entries:              entries,
	}, nil
}

// RecipeListResult is the outcome of legacy れしぴ: the compendium and whether
// this view awarded comp_alc.
type RecipeListResult struct {
	Compendium     Compendium `json:"compendium"`
	CompAlcAwarded bool       `json:"comp_alc_awarded"`
}

// ViewRecipes awards comp_alc and its induction on the first view of a fully
// crafted compendium, as legacy れしぴ does; delivery and observation never award.
func (s *Service) ViewRecipes(ctx context.Context, characterID string) (RecipeListResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return RecipeListResult{}, ErrInvalidCharacterID
	}
	var res RecipeListResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// The lock serializes concurrent views so the award happens once.
		state, err := s.repo.GetSynthesisStateForUpdate(txCtx, characterID)
		if err != nil {
			return fmt.Errorf("lock synthesis state: %w", err)
		}
		crafted, err := s.repo.CountCraftedRecipes(txCtx, characterID)
		if err != nil {
			return fmt.Errorf("count crafted recipes: %w", err)
		}
		if crafted >= len(s.recipes.All()) && !state.CompAlc {
			if err := s.repo.SetCompAlcTitle(txCtx, characterID); err != nil {
				return fmt.Errorf("set comp alc title: %w", err)
			}
			if s.legend != nil {
				if err := s.legend.RecordLegend(txCtx, "comp_alc", characterID); err != nil {
					return fmt.Errorf("record legend: %w", err)
				}
			}
			res.CompAlcAwarded = true
		}
		res.Compendium, err = s.GetCompendium(txCtx, characterID)
		return err
	})
	if err != nil {
		return RecipeListResult{}, err
	}
	if res.CompAlcAwarded && s.news != nil {
		s.publishCompletionNews(ctx, characterID)
	}
	return res, nil
}

func (s *Service) publishCompletionNews(ctx context.Context, characterID string) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		slog.WarnContext(ctx, "alchemy completion news skipped", "error_type", reflect.TypeOf(err).String())
		return
	}
	msg := char.Name + "が錬金レシピをコンプリートする！"
	//lint:ignore error-swallow best-effort informational news does not invalidate the committed award
	if err := s.news.PublishNews(ctx, "alchemy", msg, msg, "System", s.now()); err != nil {
		slog.WarnContext(ctx, "alchemy completion news failed", "error_type", reflect.TypeOf(err).String())
	}
}
