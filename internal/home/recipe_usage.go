package home

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrNoRecipesToLearn         = errors.New("no more recipes can be learned from this book")
	ErrRecipeLearnerUnavailable = errors.New("recipe learner service is not available")
)

// LearnedRecipe represents recipe combination discovery metadata.
type LearnedRecipe struct {
	BaseName     string
	MaterialName string
}

// RecipeLearner provides character recipe discovery matching allowed base material pools.
type RecipeLearner interface {
	LearnRecipe(ctx context.Context, characterID string, pool []string) (LearnedRecipe, error)
}

// SetRecipeLearner registers a cross-domain recipe learner service.
func (s *Service) SetRecipeLearner(l RecipeLearner) {
	s.recipeLearner = l
}

// WithRecipeLearner configures a RecipeLearner for the Service.
func WithRecipeLearner(l RecipeLearner) ServiceOption {
	return func(s *Service) {
		s.recipeLearner = l
	}
}

// basicRecipePool defines the 34 base items learnable via 基本錬金レシピ (Item 127).
var basicRecipePool = []string{
	"item-001", "item-002", "item-003", "item-007", "item-072", "item-079", "item-024", "item-025",
	"item-036", "item-131", "item-012", "item-118", "weapon-01", "weapon-03", "weapon-06", "weapon-43",
	"weapon-14", "weapon-45", "weapon-18", "weapon-08", "weapon-64", "weapon-09", "weapon-11", "weapon-47",
	"weapon-07", "armor-01", "armor-02", "armor-04", "armor-07", "armor-11", "armor-16", "armor-13",
	"armor-17", "armor-18",
}

// advancedRecipePool defines the 30 base items learnable via 応用錬金レシピ (Item 128).
var advancedRecipePool = []string{
	"item-006", "item-011", "item-022", "item-021", "item-073", "item-078", "item-081", "item-082",
	"item-106", "item-026", "item-119", "item-115", "item-114", "weapon-15", "weapon-46", "weapon-19",
	"weapon-21", "weapon-13", "item-090", "weapon-34", "weapon-57", "weapon-65", "weapon-20", "weapon-48",
	"armor-19", "armor-14", "armor-43", "armor-21", "armor-08", "armor-38",
}

func isRecipeScrollItem(name string) bool {
	switch name {
	case "基本錬金レシピ", "応用錬金レシピ", "神の錬金レシピ":
		return true
	default:
		return false
	}
}

func (s *Service) applyRecipeScrollConsumable(ctx context.Context, char *corecharacter.Character, itemName string) (string, error) {
	if s.recipeLearner == nil {
		return "", ErrRecipeLearnerUnavailable
	}

	var pool []string
	switch itemName {
	case "基本錬金レシピ":
		pool = basicRecipePool
	case "応用錬金レシピ":
		pool = advancedRecipePool
	case "神の錬金レシピ":
		pool = nil
	default:
		return "", fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, itemName)
	}

	res, err := s.recipeLearner.LearnRecipe(ctx, char.ID, pool)
	if err != nil {
		if errors.Is(err, ErrNoRecipesToLearn) {
			return "この錬金レシピからこれ以上習得できる錬金方法はないようだ…", nil
		}
		return "", err
	}

	return fmt.Sprintf("%s は錬金レシピを読んだ！【%s × %s ＝ ？？？】の錬金方法を習得した！", char.Name, res.BaseName, res.MaterialName), nil
}
