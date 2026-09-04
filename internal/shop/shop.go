package shop

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
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

// Deprecated: TransactionRepository is no longer used by shop.Service.
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
	characters  CharacterRepository
	inventories InventoryRepository
	txProvider  TransactionProvider
	catalog     item.DefinitionProvider
	economy     *economy.Service
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
	var ecoOpts []economy.Option
	if s.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
	}
	eco, err := economy.NewService(characters, inventories, ecoOpts...)
	if err != nil {
		return nil, err
	}
	s.economy = eco
	return s, nil
}

// Deprecated: TransactionRepository is no longer used by shop.Service since economy.Service handles transactions.
// Use NewService with WithTransactionProvider option instead.
func NewServiceWithTransaction(characters CharacterRepository, inventories InventoryRepository, _ TransactionRepository, catalog item.DefinitionProvider, opts ...Option) (*Service, error) {
	return NewService(characters, inventories, catalog, opts...)
}

func (s *Service) CalculateSellPrice(basePrice int) int {
	if basePrice <= 0 {
		return 0
	}
	return basePrice / 2
}

func safeMultiply(a, b int) (int, error) {
	val, err := economy.SafeMultiply(a, b)
	if err != nil {
		return 0, ErrPriceOverflow
	}
	return val, nil
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

	var result PurchaseResult
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID:       characterID,
			DeductGold:        totalPrice,
			GrantDefinitionID: itemDefinitionID,
			GrantQuantity:     quantity,
		})
		if err != nil {
			if errors.Is(err, economy.ErrInsufficientGold) {
				return ErrInsufficientFunds
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return corecharacter.ErrNotFound
			}
			return err
		}

		result = PurchaseResult{
			Character:    res.Character,
			Inventory:    res.Inventory,
			ItemInstance: *res.GrantedItem,
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
		// 1. Lock Character first (Deterministic lock order: characters -> inventory_items)
		if _, err := s.findCharacter(txCtx, characterID); err != nil {
			return corecharacter.ErrNotFound
		}

		// 2. Lock Inventory next
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

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID:        characterID,
			AddGold:            totalPayout,
			ConsumeInstanceID:  itemInstanceID,
			ConsumeInstanceQty: quantity,
		})
		if err != nil {
			if errors.Is(err, economy.ErrGoldOverflow) {
				return ErrPriceOverflow
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return corecharacter.ErrNotFound
			}
			if errors.Is(err, economy.ErrItemNotFound) {
				return ErrUnownedItem
			}
			if errors.Is(err, economy.ErrInsufficientItemQuantity) {
				return ErrInvalidQuantity
			}
			return err
		}

		soldInstance := instance
		soldInstance.Quantity = quantity

		result = SaleResult{
			Character:    res.Character,
			Inventory:    res.Inventory,
			SoldInstance: soldInstance,
			TotalPayout:  totalPayout,
		}
		return nil
	})
	if err != nil {
		return SaleResult{}, err
	}

	return result, nil
}
