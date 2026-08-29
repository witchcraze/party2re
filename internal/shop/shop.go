package shop

import (
	"context"
	"errors"
	"math"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

const (
	MaxTransactionQuantity = 9999
)

var (
	ErrNilDependency     = errors.New("shop dependency is nil")
	ErrInsufficientFunds = errors.New("insufficient funds to purchase item")
	ErrItemNotFound      = errors.New("item not found in catalog")
	ErrUnownedItem       = errors.New("item is not owned in inventory")
	ErrInvalidQuantity   = errors.New("invalid transaction quantity")
	ErrPriceOverflow     = errors.New("price calculation overflow")
)

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitTransaction(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
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

type PurchaseResult struct {
	Character    corecharacter.Character
	Inventory    coreinventory.Inventory
	ItemInstance item.Instance
	TotalPrice   int
}

type SaleResult struct {
	Character    corecharacter.Character
	Inventory    coreinventory.Inventory
	SoldInstance item.Instance
	TotalPayout  int
}

type Service struct {
	characters   CharacterRepository
	inventories  InventoryRepository
	transactions TransactionRepository
	txProvider   TransactionProvider
	catalog      item.DefinitionProvider
}

func NewService(characters CharacterRepository, inventories InventoryRepository, catalog item.DefinitionProvider, opts ...Option) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, ErrNilDependency
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
		catalog:     catalog,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func NewServiceWithTransaction(characters CharacterRepository, inventories InventoryRepository, transactions TransactionRepository, catalog item.DefinitionProvider, opts ...Option) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, ErrNilDependency
	}
	s := &Service{
		characters:   characters,
		inventories:  inventories,
		transactions: transactions,
		catalog:      catalog,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) CalculateSellPrice(basePrice int) int {
	if basePrice <= 0 {
		return 0
	}
	return basePrice / 2
}

func safeMultiply(a, b int) (int, error) {
	if a <= 0 || b <= 0 {
		return 0, nil
	}
	if a > math.MaxInt/b {
		return 0, ErrPriceOverflow
	}
	return a * b, nil
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

func (s *Service) Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (PurchaseResult, error) {
	if quantity <= 0 || quantity > MaxTransactionQuantity {
		return PurchaseResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return PurchaseResult{}, corecharacter.ErrNotFound
	}

	definition, err := s.catalog.FindByID(itemDefinitionID)
	if err != nil {
		return PurchaseResult{}, ErrItemNotFound
	}

	totalPrice, err := safeMultiply(definition.Price, quantity)
	if err != nil {
		return PurchaseResult{}, err
	}

	instance, err := item.NewInstance(itemDefinitionID, quantity)
	if err != nil {
		return PurchaseResult{}, err
	}

	var result PurchaseResult
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		if err := inv.Add(instance); err != nil {
			return err
		}

		char.Money -= totalPrice

		if err := s.commit(txCtx, char, inv); err != nil {
			return err
		}

		result = PurchaseResult{
			Character:    char,
			Inventory:    inv,
			ItemInstance: instance,
			TotalPrice:   totalPrice,
		}
		return nil
	})
	if err != nil {
		return PurchaseResult{}, err
	}

	return result, nil
}

func (s *Service) Sell(ctx context.Context, characterID string, itemInstanceID string, quantity int) (SaleResult, error) {
	if quantity <= 0 || quantity > MaxTransactionQuantity {
		return SaleResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return SaleResult{}, corecharacter.ErrNotFound
	}

	var result SaleResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		instance, found := inv.Find(itemInstanceID)
		if !found {
			return ErrUnownedItem
		}
		if instance.Quantity < quantity {
			return ErrInvalidQuantity
		}

		definition, err := s.catalog.FindByID(instance.DefinitionID)
		if err != nil {
			return ErrItemNotFound
		}

		sellUnitPrice := s.CalculateSellPrice(definition.Price)
		totalPayout, err := safeMultiply(sellUnitPrice, quantity)
		if err != nil {
			return err
		}

		if math.MaxInt-char.Money < totalPayout {
			return ErrPriceOverflow
		}

		if err := inv.Consume(itemInstanceID, quantity); err != nil {
			return err
		}

		char.Money += totalPayout

		if err := s.commit(txCtx, char, inv); err != nil {
			return err
		}

		result = SaleResult{
			Character:    char,
			Inventory:    inv,
			SoldInstance: instance,
			TotalPayout:  totalPayout,
		}
		return nil
	})
	if err != nil {
		return SaleResult{}, err
	}

	return result, nil
}

func (s *Service) commit(ctx context.Context, char corecharacter.Character, inv coreinventory.Inventory) error {
	if s.transactions != nil && s.txProvider == nil {
		return s.transactions.CommitTransaction(ctx, char, inv)
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return err
	}
	return s.inventories.Save(ctx, inv)
}
