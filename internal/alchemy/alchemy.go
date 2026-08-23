package alchemy

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrInvalidCharacterID    = errors.New("invalid character ID")
	ErrInvalidRecipeID       = errors.New("invalid recipe ID")
	ErrInsufficientFunds     = errors.New("insufficient character funds for alchemy")
	ErrInsufficientMaterials = errors.New("insufficient ingredients for alchemy")
)

type Result struct {
	Recipe      Recipe
	CreatedItem item.Instance
	GoldCost    int
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitSynthesis(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
}

type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	txRepo      TransactionRepository
	recipes     *RecipeCatalog
	items       item.DefinitionProvider
}

func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	recipes *RecipeCatalog,
	items item.DefinitionProvider,
) (*Service, error) {
	if characters == nil || inventories == nil || recipes == nil || items == nil {
		return nil, errors.New("dependencies are nil")
	}
	return &Service{
		characters:  characters,
		inventories: inventories,
		recipes:     recipes,
		items:       items,
	}, nil
}

func NewServiceWithTransaction(
	characters CharacterRepository,
	inventories InventoryRepository,
	txRepo TransactionRepository,
	recipes *RecipeCatalog,
	items item.DefinitionProvider,
) (*Service, error) {
	if characters == nil || inventories == nil || recipes == nil || items == nil {
		return nil, errors.New("dependencies are nil")
	}
	return &Service{
		characters:  characters,
		inventories: inventories,
		txRepo:      txRepo,
		recipes:     recipes,
		items:       items,
	}, nil
}

func (s *Service) Synthesize(ctx context.Context, characterID string, recipeID string) (Result, error) {
	if characterID == "" {
		return Result{}, ErrInvalidCharacterID
	}
	if recipeID == "" {
		return Result{}, ErrInvalidRecipeID
	}

	recipe, err := s.recipes.FindByID(recipeID)
	if err != nil {
		return Result{}, err
	}

	character, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return Result{}, err
	}

	if character.Money < recipe.GoldFee {
		return Result{}, ErrInsufficientFunds
	}

	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Result{}, err
	}

	// Verify all ingredients are available
	for _, ing := range recipe.Ingredients {
		if inv.Quantity(ing.DefinitionID) < ing.Quantity {
			return Result{}, ErrInsufficientMaterials
		}
	}

	// Consume ingredients
	for _, ing := range recipe.Ingredients {
		needed := ing.Quantity
		for _, inst := range inv.Items {
			if inst.DefinitionID == ing.DefinitionID && inst.Quantity > 0 {
				toTake := inst.Quantity
				if toTake > needed {
					toTake = needed
				}
				_ = inv.Consume(inst.ID, toTake)
				needed -= toTake
				if needed <= 0 {
					break
				}
			}
		}
	}

	// Deduct gold
	character.Money -= recipe.GoldFee

	// Create and add result item instance
	createdInstance, err := item.NewInstance(recipe.ResultItemDefinitionID, recipe.ResultQuantity)
	if err != nil {
		return Result{}, fmt.Errorf("create synthesized item instance: %w", err)
	}

	if err := inv.Add(createdInstance); err != nil {
		return Result{}, fmt.Errorf("add synthesized item to inventory: %w", err)
	}

	// Persist atomically if transaction repository is configured
	if s.txRepo != nil {
		if err := s.txRepo.CommitSynthesis(ctx, character, inv); err != nil {
			return Result{}, fmt.Errorf("commit synthesis transaction: %w", err)
		}
	} else {
		if err := s.characters.Update(ctx, character); err != nil {
			return Result{}, fmt.Errorf("update character: %w", err)
		}
		if err := s.inventories.Save(ctx, inv); err != nil {
			return Result{}, fmt.Errorf("save inventory: %w", err)
		}
	}

	return Result{
		Recipe:      recipe,
		CreatedItem: createdInstance,
		GoldCost:    recipe.GoldFee,
	}, nil
}
