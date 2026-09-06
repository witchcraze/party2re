package depot

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

const DefaultDepotCapacity = 50

var (
	ErrNotFound              = errors.New("depot not found")
	ErrDepotFull             = errors.New("depot is at full capacity")
	ErrInventoryFull         = errors.New("inventory is full")
	ErrItemNotFound          = errors.New("item not found")
	ErrInsufficientFunds     = errors.New("insufficient character funds")
	ErrInsufficientDepotGold = errors.New("insufficient depot gold")
	ErrInvalidAmount         = errors.New("amount must be positive")
	ErrInvalidCharacterID    = errors.New("invalid character ID")
	ErrInvalidItemInstanceID = errors.New("invalid item instance ID")
)

type Depot struct {
	CharacterID string
	Gold        int
	Capacity    int
	Items       []item.Instance
}

func NewDepot(characterID string) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	return Depot{CharacterID: characterID, Gold: 0, Capacity: DefaultDepotCapacity, Items: []item.Instance{}}, nil
}

func (d *Depot) AddItem(instance item.Instance) error {
	if len(d.Items) >= d.Capacity {
		return ErrDepotFull
	}
	for i, existing := range d.Items {
		if existing.DefinitionID == instance.DefinitionID {
			d.Items[i].Quantity += instance.Quantity
			return nil
		}
	}
	d.Items = append(d.Items, instance)
	return nil
}

func (d *Depot) RemoveItem(instanceID string) (item.Instance, error) {
	for i, existing := range d.Items {
		if existing.ID == instanceID {
			d.Items = append(d.Items[:i], d.Items[i+1:]...)
			return existing, nil
		}
	}
	return item.Instance{}, ErrItemNotFound
}

type Repository interface {
	FindByCharacterID(ctx context.Context, characterID string) (Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (Depot, error)
	Save(ctx context.Context, value Depot) error
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

type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) { s.txProvider = txProvider }
}

func WithTransactionRunner(runner TransactionRunner) Option {
	return func(s *Service) { s.runner = runner }
}

func WithEconomy(eco *economy.Service) Option {
	return func(s *Service) { s.runner = eco }
}

type Service struct {
	depotRepo  Repository
	charRepo   CharacterRepository
	invRepo    InventoryRepository
	txProvider TransactionProvider
	runner     TransactionRunner
}

func NewService(depotRepo Repository, charRepo CharacterRepository, invRepo InventoryRepository, opts ...Option) (*Service, error) {
	if depotRepo == nil || charRepo == nil || invRepo == nil {
		return nil, errors.New("dependencies are nil")
	}
	s := &Service{depotRepo: depotRepo, charRepo: charRepo, invRepo: invRepo}
	for _, opt := range opts {
		opt(s)
	}
	if s.runner == nil {
		var ecoOpts []economy.Option
		if s.txProvider != nil {
			ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
		}
		eco, err := economy.NewService(charRepo, invRepo, ecoOpts...)
		if err != nil {
			return nil, err
		}
		s.runner = eco
	}
	return s, nil
}

func NewServiceWithTransaction(depotRepo Repository, charRepo CharacterRepository, invRepo InventoryRepository, txProvider TransactionProvider, opts ...Option) (*Service, error) {
	if txProvider == nil {
		return nil, errors.New("dependencies are nil")
	}
	return NewService(depotRepo, charRepo, invRepo, append([]Option{WithTransactionProvider(txProvider)}, opts...)...)
}

func (s *Service) findOrCreateDepot(ctx context.Context, characterID string) (Depot, error) {
	dep, err := s.depotRepo.FindByCharacterIDForUpdate(ctx, characterID)
	if err != nil && errors.Is(err, ErrNotFound) {
		return NewDepot(characterID)
	}
	return dep, err
}

func (s *Service) saveDepot(ctx context.Context, dep Depot) error {
	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return fmt.Errorf("save depot: %w", err)
	}
	return nil
}

func validateGoldOp(characterID string, amount int) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if amount <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

func validateItemOp(characterID string, itemInstanceID string) error {
	if characterID == "" {
		return ErrInvalidCharacterID
	}
	if itemInstanceID == "" {
		return ErrInvalidItemInstanceID
	}
	return nil
}

func mapEconomyError(err error) error {
	if errors.Is(err, economy.ErrInsufficientGold) {
		return ErrInsufficientFunds
	}
	if errors.Is(err, economy.ErrCharacterNotFound) {
		return corecharacter.ErrNotFound
	}
	return err
}

func (s *Service) GetDepot(ctx context.Context, characterID string) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	depot, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil && errors.Is(err, ErrNotFound) {
		return NewDepot(characterID)
	}
	return depot, err
}

func (s *Service) DepositGold(ctx context.Context, characterID string, amount int) (Depot, error) {
	if err := validateGoldOp(characterID, amount); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID, Cost: economy.ResourceCost{Gold: amount}}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID)
		if err != nil {
			return err
		}
		dep.Gold += amount
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

func (s *Service) WithdrawGold(ctx context.Context, characterID string, amount int) (Depot, error) {
	if err := validateGoldOp(characterID, amount); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.depotRepo.FindByCharacterIDForUpdate(tc.Context, characterID)
		if err != nil {
			return err
		}
		if dep.Gold < amount {
			return ErrInsufficientDepotGold
		}
		dep.Gold -= amount
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		tc.AddGrant(economy.ResourceGrant{Gold: amount})
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

func (s *Service) DepositItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID)
		if err != nil {
			return err
		}
		if len(dep.Items) >= dep.Capacity {
			return ErrDepotFull
		}
		itemInstance, found := tc.Inventory.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}
		if err := tc.Inventory.Consume(itemInstanceID, itemInstance.Quantity); err != nil {
			return err
		}
		if err := dep.AddItem(itemInstance); err != nil {
			return err
		}
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

func (s *Service) WithdrawItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.depotRepo.FindByCharacterIDForUpdate(tc.Context, characterID)
		if err != nil {
			return err
		}
		itemInstance, err := dep.RemoveItem(itemInstanceID)
		if err != nil {
			return err
		}
		if err := tc.Inventory.Add(itemInstance); err != nil {
			return ErrInventoryFull
		}
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}
