package alchemy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
)

var (
	ErrInvalidCharacterID    = errors.New("invalid character ID")
	ErrInvalidRecipeID       = errors.New("invalid recipe ID")
	ErrInsufficientMaterials = errors.New("insufficient ingredients in depot")
	ErrRecipeNotLearned      = errors.New("recipe has not been learned")
	ErrSynthesisInProgress   = errors.New("another synthesis is already in progress")
	ErrNoActiveSynthesis     = errors.New("no active synthesis to claim")
	ErrSynthesisNotReady     = errors.New("synthesis is not yet complete")
)

type SynthesisState string

const (
	StateNone      SynthesisState = "none"
	StateOngoing   SynthesisState = "ongoing"
	StateCompleted SynthesisState = "completed"
)

type Synthesis struct {
	CharacterID string         `json:"character_id"`
	RecipeID    string         `json:"recipe_id"`
	State       SynthesisState `json:"state"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	MaturesAt   *time.Time     `json:"matures_at,omitempty"`
	TotalCrafts int            `json:"total_crafts"`
	CompAlc     bool           `json:"comp_alc"`
}

type SynthesisResult struct {
	Recipe    Recipe         `json:"recipe"`
	State     SynthesisState `json:"state"`
	StartedAt time.Time      `json:"started_at"`
	MaturesAt time.Time      `json:"matures_at"`
	Message   string         `json:"message"`
}

type ClaimResult struct {
	Recipe         Recipe            `json:"recipe"`
	CreatedItem    coreitem.Instance `json:"created_item"`
	TotalCrafts    int               `json:"total_crafts"`
	CompAlcAwarded bool              `json:"comp_alc_awarded"`
	Message        string            `json:"message"`
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
}

type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, d depot.Depot) error
}

type Repository interface {
	GetSynthesisState(ctx context.Context, characterID string) (Synthesis, error)
	GetSynthesisStateForUpdate(ctx context.Context, characterID string) (Synthesis, error)
	SaveSynthesisState(ctx context.Context, s Synthesis) error
	CompleteOngoingSynthesis(ctx context.Context, characterID string) error
	GetDiscoveredRecipes(ctx context.Context, characterID string) ([]DiscoveredRecipe, error)
	SaveDiscoveredRecipe(ctx context.Context, characterID, recipeID string, isCrafted bool) error
	MarkRecipeCrafted(ctx context.Context, characterID, recipeID string) error
	CountCraftedRecipes(ctx context.Context, characterID string) (int, error)
	SetCompAlcTitle(ctx context.Context, characterID string) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type SynthesisHook func(ctx context.Context, characterID string, recipeID string) error

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithSynthesisHook(hook SynthesisHook) Option {
	return func(s *Service) {
		s.synthesisHook = hook
	}
}

func WithNowFunc(fn func() time.Time) Option {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

type Service struct {
	characters    CharacterRepository
	depots        DepotRepository
	repo          Repository
	txProvider    TransactionProvider
	recipes       *RecipeCatalog
	items         coreitem.DefinitionProvider
	synthesisHook SynthesisHook
	nowFunc       func() time.Time
	rngMu         sync.Mutex
	rng           *rand.Rand
}

func NewService(
	characters CharacterRepository,
	depots DepotRepository,
	repo Repository,
	recipes *RecipeCatalog,
	items coreitem.DefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if characters == nil || depots == nil || repo == nil || recipes == nil || items == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{
		characters: characters,
		depots:     depots,
		repo:       repo,
		recipes:    recipes,
		items:      items,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) SetSynthesisHook(hook SynthesisHook) {
	s.synthesisHook = hook
}

func (s *Service) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) Synthesize(ctx context.Context, characterID string, recipeID string) (SynthesisResult, error) {
	characterID = strings.TrimSpace(characterID)
	recipeID = strings.TrimSpace(recipeID)
	if characterID == "" {
		return SynthesisResult{}, ErrInvalidCharacterID
	}
	if recipeID == "" {
		return SynthesisResult{}, ErrInvalidRecipeID
	}

	recipe, err := s.recipes.FindByID(recipeID)
	if err != nil {
		return SynthesisResult{}, err
	}

	var res SynthesisResult
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Lock Character
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 5: Lock Depot
		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 8: Lock Synthesis State
		state, err := s.repo.GetSynthesisStateForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		now := s.now()
		if state.State == StateOngoing {
			if state.MaturesAt != nil && !now.Before(*state.MaturesAt) {
				state.State = StateCompleted
			}
		}
		if state.State == StateOngoing || state.State == StateCompleted {
			return ErrSynthesisInProgress
		}

		// Verify recipe is learned
		discovered, err := s.repo.GetDiscoveredRecipes(txCtx, characterID)
		if err != nil {
			return err
		}
		var targetDiscovered *DiscoveredRecipe
		for i := range discovered {
			if discovered[i].RecipeID == recipeID {
				targetDiscovered = &discovered[i]
				break
			}
		}
		if targetDiscovered == nil {
			return ErrRecipeNotLearned
		}

		// Verify materials in Depot
		for _, ing := range recipe.Ingredients {
			if dep.Quantity(ing.DefinitionID) < ing.Quantity {
				return ErrInsufficientMaterials
			}
		}

		// Consume materials from Depot
		for _, ing := range recipe.Ingredients {
			needed := ing.Quantity
			for _, inst := range dep.Items {
				if inst.DefinitionID == ing.DefinitionID && inst.Quantity > 0 {
					toTake := inst.Quantity
					if toTake > needed {
						toTake = needed
					}
					if _, err := dep.Consume(inst.ID, toTake); err != nil {
						return err
					}
					needed -= toTake
					if needed <= 0 {
						break
					}
				}
			}
		}

		if err := s.depots.Save(txCtx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}

		maturesAt := timer.NextMidnightJST(now)
		state.RecipeID = recipeID
		state.State = StateOngoing
		state.StartedAt = &now
		state.MaturesAt = &maturesAt

		if err := s.repo.SaveSynthesisState(txCtx, state); err != nil {
			return fmt.Errorf("save synthesis state: %w", err)
		}

		var msg string
		if targetDiscovered.IsCrafted {
			resItem, _ := s.items.FindByID(recipe.ResultItemDefinitionID)
			msg = fmt.Sprintf("ふむ、この組み合わせなら %s ができるぞ！完成する頃にまた来るがよい", resItem.Name)
		} else {
			msg = "おお、錬金可能なようじゃ！何が出来るか楽しみじゃな！一晩たてば完成するじゃろう。完成する頃にまた来るがよい"
		}

		res = SynthesisResult{
			Recipe:    recipe,
			State:     StateOngoing,
			StartedAt: now,
			MaturesAt: maturesAt,
			Message:   msg,
		}
		_ = char
		return nil
	})
	if err != nil {
		return SynthesisResult{}, err
	}
	return res, nil
}

func (s *Service) GetStatus(ctx context.Context, characterID string) (Synthesis, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Synthesis{}, ErrInvalidCharacterID
	}
	st, err := s.repo.GetSynthesisState(ctx, characterID)
	if err != nil {
		return Synthesis{}, err
	}
	now := s.now()
	if st.State == StateOngoing && st.MaturesAt != nil && !now.Before(*st.MaturesAt) {
		st.State = StateCompleted
	}
	return st, nil
}

func (s *Service) CompleteOngoingSynthesis(ctx context.Context, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	return s.runInTx(ctx, func(txCtx context.Context) error {
		return s.repo.CompleteOngoingSynthesis(txCtx, characterID)
	})
}

func (s *Service) Claim(ctx context.Context, characterID string) (ClaimResult, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ClaimResult{}, ErrInvalidCharacterID
	}

	var res ClaimResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Lock Character
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 5: Lock Depot
		dep, err := s.depots.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Rank 8: Lock Synthesis State
		state, err := s.repo.GetSynthesisStateForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		now := s.now()
		if state.State == StateOngoing {
			if state.MaturesAt != nil && !now.Before(*state.MaturesAt) {
				state.State = StateCompleted
			} else {
				return ErrSynthesisNotReady
			}
		}
		if state.State != StateCompleted || state.RecipeID == "" {
			return ErrNoActiveSynthesis
		}

		recipe, err := s.recipes.FindByID(state.RecipeID)
		if err != nil {
			return err
		}

		createdInstance, err := coreitem.NewInstance(recipe.ResultItemDefinitionID, recipe.ResultQuantity)
		if err != nil {
			return fmt.Errorf("create item instance: %w", err)
		}

		if err := dep.AddItem(createdInstance); err != nil {
			return err
		}

		if err := s.depots.Save(txCtx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}

		if err := s.repo.MarkRecipeCrafted(txCtx, characterID, recipe.ID); err != nil {
			return fmt.Errorf("mark recipe crafted: %w", err)
		}

		state.TotalCrafts++

		// Check 100% compendium title
		compAlcAwarded := false
		craftedCount, err := s.repo.CountCraftedRecipes(txCtx, characterID)
		if err == nil && craftedCount >= len(s.recipes.All()) && !state.CompAlc {
			state.CompAlc = true
			_ = s.repo.SetCompAlcTitle(txCtx, characterID)
			compAlcAwarded = true
		}

		clearedRecipeID := state.RecipeID
		state.State = StateNone
		state.RecipeID = ""
		state.StartedAt = nil
		state.MaturesAt = nil

		if err := s.repo.SaveSynthesisState(txCtx, state); err != nil {
			return fmt.Errorf("save synthesis state: %w", err)
		}

		resultItem, _ := s.items.FindByID(recipe.ResultItemDefinitionID)
		msg := fmt.Sprintf("まっておったぞ！%s が完成したぞい！出来上がったアイテムは預かり所の方に送っておいたぞい！", resultItem.Name)
		if compAlcAwarded {
			msg += fmt.Sprintf(" %sは 錬金レシピ をコンプリートしました！のじゃ", char.Name)
		}

		res = ClaimResult{
			Recipe:         recipe,
			CreatedItem:    createdInstance,
			TotalCrafts:    state.TotalCrafts,
			CompAlcAwarded: compAlcAwarded,
			Message:        msg,
		}

		if s.synthesisHook != nil {
			_ = s.synthesisHook(txCtx, characterID, clearedRecipeID)
		}
		return nil
	})
	if err != nil {
		return ClaimResult{}, err
	}
	return res, nil
}
