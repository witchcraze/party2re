package depot

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
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
	return Depot{
		CharacterID: characterID,
		Gold:        0,
		Capacity:    DefaultDepotCapacity,
		Items:       []item.Instance{},
	}, nil
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
	Save(ctx context.Context, value Depot) error
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
	Execute(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

type Tx interface {
	GetCharacter(ctx context.Context, characterID string) (corecharacter.Character, error)
	SaveCharacter(ctx context.Context, character corecharacter.Character) error
	GetInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	SaveInventory(ctx context.Context, inventory coreinventory.Inventory) error
	GetDepot(ctx context.Context, characterID string) (Depot, error)
	SaveDepot(ctx context.Context, depot Depot) error
}

type Service struct {
	depotRepo Repository
	charRepo  CharacterRepository
	invRepo   InventoryRepository
	txRepo    TransactionRepository
}

func NewService(
	depotRepo Repository,
	charRepo CharacterRepository,
	invRepo InventoryRepository,
) (*Service, error) {
	if depotRepo == nil || charRepo == nil || invRepo == nil {
		return nil, errors.New("dependencies are nil")
	}
	return &Service{
		depotRepo: depotRepo,
		charRepo:  charRepo,
		invRepo:   invRepo,
	}, nil
}

func NewServiceWithTransaction(
	depotRepo Repository,
	charRepo CharacterRepository,
	invRepo InventoryRepository,
	txRepo TransactionRepository,
) (*Service, error) {
	if depotRepo == nil || charRepo == nil || invRepo == nil || txRepo == nil {
		return nil, errors.New("dependencies are nil")
	}
	return &Service{
		depotRepo: depotRepo,
		charRepo:  charRepo,
		invRepo:   invRepo,
		txRepo:    txRepo,
	}, nil
}

func (s *Service) GetDepot(ctx context.Context, characterID string) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	depot, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return NewDepot(characterID)
		}
		return Depot{}, err
	}
	return depot, nil
}

func (s *Service) DepositGold(ctx context.Context, characterID string, amount int) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	if amount <= 0 {
		return Depot{}, ErrInvalidAmount
	}

	var resultDepot Depot
	operation := func(ctx context.Context, tx Tx) error {
		char, err := tx.GetCharacter(ctx, characterID)
		if err != nil {
			return err
		}
		if char.Money < amount {
			return ErrInsufficientFunds
		}

		dep, err := tx.GetDepot(ctx, characterID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				dep, err = NewDepot(characterID)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		char.Money -= amount
		dep.Gold += amount

		if err := tx.SaveCharacter(ctx, char); err != nil {
			return fmt.Errorf("save character: %w", err)
		}
		if err := tx.SaveDepot(ctx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}

		resultDepot = dep
		return nil
	}

	if s.txRepo != nil {
		if err := s.txRepo.Execute(ctx, operation); err != nil {
			return Depot{}, err
		}
		return resultDepot, nil
	}

	// Fallback non-tx path
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	if char.Money < amount {
		return Depot{}, ErrInsufficientFunds
	}
	dep, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			dep, err = NewDepot(characterID)
			if err != nil {
				return Depot{}, err
			}
		} else {
			return Depot{}, err
		}
	}
	char.Money -= amount
	dep.Gold += amount

	if err := s.charRepo.Update(ctx, char); err != nil {
		return Depot{}, err
	}
	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return Depot{}, err
	}
	return dep, nil
}

func (s *Service) WithdrawGold(ctx context.Context, characterID string, amount int) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	if amount <= 0 {
		return Depot{}, ErrInvalidAmount
	}

	var resultDepot Depot
	operation := func(ctx context.Context, tx Tx) error {
		dep, err := tx.GetDepot(ctx, characterID)
		if err != nil {
			return err
		}
		if dep.Gold < amount {
			return ErrInsufficientDepotGold
		}

		char, err := tx.GetCharacter(ctx, characterID)
		if err != nil {
			return err
		}

		dep.Gold -= amount
		char.Money += amount

		if err := tx.SaveDepot(ctx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}
		if err := tx.SaveCharacter(ctx, char); err != nil {
			return fmt.Errorf("save character: %w", err)
		}

		resultDepot = dep
		return nil
	}

	if s.txRepo != nil {
		if err := s.txRepo.Execute(ctx, operation); err != nil {
			return Depot{}, err
		}
		return resultDepot, nil
	}

	// Fallback non-tx path
	dep, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	if dep.Gold < amount {
		return Depot{}, ErrInsufficientDepotGold
	}
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	dep.Gold -= amount
	char.Money += amount

	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return Depot{}, err
	}
	if err := s.charRepo.Update(ctx, char); err != nil {
		return Depot{}, err
	}
	return dep, nil
}

func (s *Service) DepositItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	if itemInstanceID == "" {
		return Depot{}, ErrInvalidItemInstanceID
	}

	var resultDepot Depot
	operation := func(ctx context.Context, tx Tx) error {
		inv, err := tx.GetInventory(ctx, characterID)
		if err != nil {
			return err
		}

		dep, err := tx.GetDepot(ctx, characterID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				dep, err = NewDepot(characterID)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		if len(dep.Items) >= dep.Capacity {
			return ErrDepotFull
		}

		itemInstance, err := inv.Remove(itemInstanceID)
		if err != nil {
			return ErrItemNotFound
		}

		if err := dep.AddItem(itemInstance); err != nil {
			return err
		}

		if err := tx.SaveInventory(ctx, inv); err != nil {
			return fmt.Errorf("save inventory: %w", err)
		}
		if err := tx.SaveDepot(ctx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}

		resultDepot = dep
		return nil
	}

	if s.txRepo != nil {
		if err := s.txRepo.Execute(ctx, operation); err != nil {
			return Depot{}, err
		}
		return resultDepot, nil
	}

	// Fallback non-tx path
	inv, err := s.invRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	dep, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			dep, err = NewDepot(characterID)
			if err != nil {
				return Depot{}, err
			}
		} else {
			return Depot{}, err
		}
	}
	if len(dep.Items) >= dep.Capacity {
		return Depot{}, ErrDepotFull
	}
	itemInstance, err := inv.Remove(itemInstanceID)
	if err != nil {
		return Depot{}, ErrItemNotFound
	}
	if err := dep.AddItem(itemInstance); err != nil {
		return Depot{}, err
	}
	if err := s.invRepo.Save(ctx, inv); err != nil {
		return Depot{}, err
	}
	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return Depot{}, err
	}
	return dep, nil
}

func (s *Service) WithdrawItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if characterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	if itemInstanceID == "" {
		return Depot{}, ErrInvalidItemInstanceID
	}

	var resultDepot Depot
	operation := func(ctx context.Context, tx Tx) error {
		dep, err := tx.GetDepot(ctx, characterID)
		if err != nil {
			return err
		}

		inv, err := tx.GetInventory(ctx, characterID)
		if err != nil {
			return err
		}

		itemInstance, err := dep.RemoveItem(itemInstanceID)
		if err != nil {
			return err
		}

		if err := inv.Add(itemInstance); err != nil {
			return ErrInventoryFull
		}

		if err := tx.SaveDepot(ctx, dep); err != nil {
			return fmt.Errorf("save depot: %w", err)
		}
		if err := tx.SaveInventory(ctx, inv); err != nil {
			return fmt.Errorf("save inventory: %w", err)
		}

		resultDepot = dep
		return nil
	}

	if s.txRepo != nil {
		if err := s.txRepo.Execute(ctx, operation); err != nil {
			return Depot{}, err
		}
		return resultDepot, nil
	}

	// Fallback non-tx path
	dep, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	inv, err := s.invRepo.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	itemInstance, err := dep.RemoveItem(itemInstanceID)
	if err != nil {
		return Depot{}, err
	}
	if err := inv.Add(itemInstance); err != nil {
		return Depot{}, ErrInventoryFull
	}
	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return Depot{}, err
	}
	if err := s.invRepo.Save(ctx, inv); err != nil {
		return Depot{}, err
	}
	return dep, nil
}
