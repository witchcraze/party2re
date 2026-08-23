package shop

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrNilDependency     = errors.New("shop dependency is nil")
	ErrInsufficientFunds = errors.New("insufficient funds to purchase item")
	ErrItemNotFound      = errors.New("item not found in catalog")
	ErrUnownedItem       = errors.New("item is not owned in inventory")
	ErrInvalidQuantity   = errors.New("invalid transaction quantity")
)

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type TransactionRepository interface {
	CommitTransaction(ctx context.Context, character corecharacter.Character, inventory coreinventory.Inventory) error
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
	catalog      item.DefinitionProvider
}

func NewService(characters CharacterRepository, inventories InventoryRepository, catalog item.DefinitionProvider) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, ErrNilDependency
	}
	return &Service{
		characters:  characters,
		inventories: inventories,
		catalog:     catalog,
	}, nil
}

func NewServiceWithTransaction(characters CharacterRepository, inventories InventoryRepository, transactions TransactionRepository, catalog item.DefinitionProvider) (*Service, error) {
	if characters == nil || inventories == nil || catalog == nil {
		return nil, ErrNilDependency
	}
	return &Service{
		characters:   characters,
		inventories:  inventories,
		transactions: transactions,
		catalog:      catalog,
	}, nil
}

func (s *Service) CalculateSellPrice(basePrice int) int {
	if basePrice <= 0 {
		return 0
	}
	return basePrice / 2
}

func (s *Service) Purchase(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (PurchaseResult, error) {
	if quantity <= 0 {
		return PurchaseResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return PurchaseResult{}, corecharacter.ErrNotFound
	}

	definition, err := s.catalog.FindByID(itemDefinitionID)
	if err != nil {
		return PurchaseResult{}, ErrItemNotFound
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return PurchaseResult{}, err
	}

	totalPrice := definition.Price * quantity
	if char.Money < totalPrice {
		return PurchaseResult{}, ErrInsufficientFunds
	}

	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return PurchaseResult{}, err
	}

	instance, err := item.NewInstance(itemDefinitionID, quantity)
	if err != nil {
		return PurchaseResult{}, err
	}

	if err := inv.Add(instance); err != nil {
		return PurchaseResult{}, err
	}

	char.Money -= totalPrice

	if err := s.commit(ctx, char, inv); err != nil {
		return PurchaseResult{}, err
	}

	return PurchaseResult{
		Character:    char,
		Inventory:    inv,
		ItemInstance: instance,
		TotalPrice:   totalPrice,
	}, nil
}

func (s *Service) Sell(ctx context.Context, characterID string, itemInstanceID string, quantity int) (SaleResult, error) {
	if quantity <= 0 {
		return SaleResult{}, ErrInvalidQuantity
	}
	if characterID == "" {
		return SaleResult{}, corecharacter.ErrNotFound
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return SaleResult{}, err
	}

	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return SaleResult{}, err
	}

	instance, found := inv.Find(itemInstanceID)
	if !found {
		return SaleResult{}, ErrUnownedItem
	}
	if instance.Quantity < quantity {
		return SaleResult{}, ErrInvalidQuantity
	}

	definition, err := s.catalog.FindByID(instance.DefinitionID)
	if err != nil {
		return SaleResult{}, ErrItemNotFound
	}

	sellUnitPrice := s.CalculateSellPrice(definition.Price)
	totalPayout := sellUnitPrice * quantity

	if err := inv.Consume(itemInstanceID, quantity); err != nil {
		return SaleResult{}, err
	}

	char.Money += totalPayout

	if err := s.commit(ctx, char, inv); err != nil {
		return SaleResult{}, err
	}

	return SaleResult{
		Character:    char,
		Inventory:    inv,
		SoldInstance: instance,
		TotalPayout:  totalPayout,
	}, nil
}

func (s *Service) commit(ctx context.Context, char corecharacter.Character, inv coreinventory.Inventory) error {
	if s.transactions != nil {
		return s.transactions.CommitTransaction(ctx, char, inv)
	}
	if err := s.characters.Update(ctx, char); err != nil {
		return err
	}
	return s.inventories.Save(ctx, inv)
}
