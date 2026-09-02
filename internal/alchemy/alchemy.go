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
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitSynthesis(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	txRepo      TransactionRepository
	txProvider  TransactionProvider
	recipes     *RecipeCatalog
	items       item.DefinitionProvider
}

func NewService(
	characters CharacterRepository,
	inventories InventoryRepository,
	recipes *RecipeCatalog,
	items item.DefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil || recipes == nil || items == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		recipes:     recipes,
		items:       items,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func NewServiceWithTransaction(
	characters CharacterRepository,
	inventories InventoryRepository,
	txRepo TransactionRepository,
	recipes *RecipeCatalog,
	items item.DefinitionProvider,
	opts ...Option,
) (*Service, error) {
	if characters == nil || inventories == nil || recipes == nil || items == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		txRepo:      txRepo,
		recipes:     recipes,
		items:       items,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) findCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if s.txProvider != nil {
		return s.characters.FindByIDForUpdate(ctx, characterID)
	}
	return s.characters.FindByID(ctx, characterID)
}

func (s *Service) findInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if s.txProvider != nil {
		return s.inventories.FindByCharacterIDForUpdate(ctx, characterID)
	}
	return s.inventories.FindByCharacterID(ctx, characterID)
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

	var result Result
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		character, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		if character.Money < recipe.GoldFee {
			return ErrInsufficientFunds
		}

		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		// Verify all ingredients are available
		for _, ing := range recipe.Ingredients {
			if inv.Quantity(ing.DefinitionID) < ing.Quantity {
				return ErrInsufficientMaterials
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
		if err := character.DeductMoney(recipe.GoldFee); err != nil {
			return ErrInsufficientFunds
		}

		// Create and add result item instance
		createdInstance, err := item.NewInstance(recipe.ResultItemDefinitionID, recipe.ResultQuantity)
		if err != nil {
			return fmt.Errorf("create synthesized item instance: %w", err)
		}

		if err := inv.Add(createdInstance); err != nil {
			return fmt.Errorf("add synthesized item to inventory: %w", err)
		}

		// Persist atomically if transaction repository is configured (and no txProvider)
		if s.txRepo != nil && s.txProvider == nil {
			if err := s.txRepo.CommitSynthesis(txCtx, character, inv); err != nil {
				return fmt.Errorf("commit synthesis transaction: %w", err)
			}
		} else {
			if err := s.characters.Update(txCtx, character); err != nil {
				return fmt.Errorf("update character: %w", err)
			}
			if err := s.inventories.Save(txCtx, inv); err != nil {
				return fmt.Errorf("save inventory: %w", err)
			}
		}

		result = Result{
			Recipe:      recipe,
			CreatedItem: createdInstance,
			GoldCost:    recipe.GoldFee,
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	return result, nil
}
