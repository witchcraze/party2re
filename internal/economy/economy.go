package economy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrNilDependency            = errors.New("economy dependency is nil")
	ErrInvalidCharacterID       = errors.New("invalid character ID")
	ErrInvalidAmount            = errors.New("invalid amount")
	ErrInvalidQuantity          = errors.New("invalid quantity")
	ErrInsufficientGold         = errors.New("insufficient gold")
	ErrGoldOverflow             = errors.New("gold calculation overflow")
	ErrInsufficientMedals       = errors.New("insufficient small medals")
	ErrInventoryFull            = errors.New("inventory is full")
	ErrItemNotFound             = errors.New("item instance not found in inventory")
	ErrInsufficientItemQuantity = errors.New("insufficient item quantity in inventory")
	ErrSelfTransferNotAllowed   = errors.New("cannot transfer gold to self")
	ErrCharacterNotFound        = errors.New("character not found")
)

// SafeMultiply safely multiplies two non-negative integers guarding against integer overflow.
func SafeMultiply(a, b int) (int, error) {
	if a <= 0 || b <= 0 {
		return 0, nil
	}
	if a > math.MaxInt/b {
		return 0, ErrGoldOverflow
	}
	return a * b, nil
}

// CharacterRepository defines character persistence for economy transactions.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// InventoryRepository defines inventory persistence for economy transactions.
type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

// TransactionProvider defines atomic transaction execution.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Option configures optional Service dependencies.
type Option func(*Service)

// WithTransactionProvider sets the TransactionProvider for the Service.
func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

// Service provides standardized transactional operations for wallet currency and inventory items.
type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	txProvider  TransactionProvider
}

// NewService creates a new transactional economy Service.
func NewService(characters CharacterRepository, inventories InventoryRepository, opts ...Option) (*Service, error) {
	if characters == nil {
		return nil, fmt.Errorf("%w: character repository", ErrNilDependency)
	}
	if inventories == nil {
		return nil, fmt.Errorf("%w: inventory repository", ErrNilDependency)
	}
	s := &Service{
		characters:  characters,
		inventories: inventories,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

type txContextKey struct{}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx.Value(txContextKey{}) != nil {
		return fn(ctx)
	}
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
			if txCtx.Value(txContextKey{}) != nil {
				return fn(txCtx)
			}
			return fn(context.WithValue(txCtx, txContextKey{}, struct{}{}))
		})
	}
	return fn(context.WithValue(ctx, txContextKey{}, struct{}{}))
}

func (s *Service) findCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if s.txProvider != nil {
		return s.characters.FindByIDForUpdate(ctx, characterID)
	}
	return s.characters.FindByID(ctx, characterID)
}

func (s *Service) findInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	var inv coreinventory.Inventory
	var err error
	if s.txProvider != nil {
		inv, err = s.inventories.FindByCharacterIDForUpdate(ctx, characterID)
	} else {
		inv, err = s.inventories.FindByCharacterID(ctx, characterID)
	}
	if err != nil {
		// If inventory does not exist yet, initialize a new in-memory inventory instance
		inv, _ = coreinventory.New(characterID)
	}
	return inv, nil
}

// DeductGold subtracts gold from a character's wallet under an exclusive row lock.
func (s *Service) DeductGold(ctx context.Context, characterID string, amount int) (corecharacter.Character, error) {
	if strings.TrimSpace(characterID) == "" {
		return corecharacter.Character{}, ErrInvalidCharacterID
	}
	if amount < 0 {
		return corecharacter.Character{}, ErrInvalidAmount
	}

	var result corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if amount == 0 {
			result = char
			return nil
		}
		if char.Money < amount {
			return ErrInsufficientGold
		}
		if err := char.DeductMoney(amount); err != nil {
			return ErrInsufficientGold
		}
		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		result = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return result, nil
}

// AddGold credits gold to a character's wallet under an exclusive row lock.
func (s *Service) AddGold(ctx context.Context, characterID string, amount int) (corecharacter.Character, error) {
	if strings.TrimSpace(characterID) == "" {
		return corecharacter.Character{}, ErrInvalidCharacterID
	}
	if amount < 0 {
		return corecharacter.Character{}, ErrInvalidAmount
	}

	var result corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if amount == 0 {
			result = char
			return nil
		}
		_ = char.AddMoney(amount)
		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		result = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return result, nil
}

// TransferGold atomically transfers gold between two characters using deterministic lock order.
func (s *Service) TransferGold(ctx context.Context, fromCharacterID, toCharacterID string, amount int) (fromChar, toChar corecharacter.Character, err error) {
	fromCharacterID = strings.TrimSpace(fromCharacterID)
	toCharacterID = strings.TrimSpace(toCharacterID)

	if fromCharacterID == "" || toCharacterID == "" {
		return corecharacter.Character{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if fromCharacterID == toCharacterID {
		return corecharacter.Character{}, corecharacter.Character{}, ErrSelfTransferNotAllowed
	}
	if amount <= 0 {
		return corecharacter.Character{}, corecharacter.Character{}, ErrInvalidAmount
	}

	firstID, secondID := id.Sort2(fromCharacterID, toCharacterID)

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		c1, err := s.characters.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			return ErrCharacterNotFound
		}
		c2, err := s.characters.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			return ErrCharacterNotFound
		}

		var sender, recipient corecharacter.Character
		if c1.ID == fromCharacterID {
			sender = c1
			recipient = c2
		} else {
			sender = c2
			recipient = c1
		}

		if sender.Money < amount {
			return ErrInsufficientGold
		}
		if err := sender.DeductMoney(amount); err != nil {
			return ErrInsufficientGold
		}
		_ = recipient.AddMoney(amount)

		if err := s.characters.Update(txCtx, sender); err != nil {
			return err
		}
		if err := s.characters.Update(txCtx, recipient); err != nil {
			return err
		}

		fromChar = sender
		toChar = recipient
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, corecharacter.Character{}, err
	}
	return fromChar, toChar, nil
}

// DeductSmallMedals subtracts small medals from a character's wallet under an exclusive row lock.
func (s *Service) DeductSmallMedals(ctx context.Context, characterID string, amount int) (corecharacter.Character, error) {
	if strings.TrimSpace(characterID) == "" {
		return corecharacter.Character{}, ErrInvalidCharacterID
	}
	if amount < 0 {
		return corecharacter.Character{}, ErrInvalidAmount
	}

	var result corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if amount == 0 {
			result = char
			return nil
		}
		if char.SmallMedals < amount {
			return ErrInsufficientMedals
		}
		if err := char.DeductSmallMedals(amount); err != nil {
			return ErrInsufficientMedals
		}
		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		result = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return result, nil
}

// AddSmallMedals credits small medals to a character's wallet under an exclusive row lock.
func (s *Service) AddSmallMedals(ctx context.Context, characterID string, amount int) (corecharacter.Character, error) {
	if strings.TrimSpace(characterID) == "" {
		return corecharacter.Character{}, ErrInvalidCharacterID
	}
	if amount < 0 {
		return corecharacter.Character{}, ErrInvalidAmount
	}

	var result corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if amount == 0 {
			result = char
			return nil
		}
		_ = char.AddSmallMedals(amount)
		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		result = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return result, nil
}

// GrantItem creates a new item instance and saves it into the character's inventory under an exclusive lock.
func (s *Service) GrantItem(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (coreinventory.Inventory, coreitem.Instance, error) {
	if strings.TrimSpace(characterID) == "" {
		return coreinventory.Inventory{}, coreitem.Instance{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemDefinitionID) == "" {
		return coreinventory.Inventory{}, coreitem.Instance{}, ErrItemNotFound
	}
	if quantity <= 0 {
		return coreinventory.Inventory{}, coreitem.Instance{}, ErrInvalidQuantity
	}

	var resInv coreinventory.Inventory
	var resInst coreitem.Instance

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(itemDefinitionID, quantity)
		if err != nil {
			return err
		}

		if err := inv.Add(inst); err != nil {
			return fmt.Errorf("%w: %v", ErrInventoryFull, err)
		}

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		resInv = inv
		resInst = inst
		return nil
	})
	if err != nil {
		return coreinventory.Inventory{}, coreitem.Instance{}, err
	}
	return resInv, resInst, nil
}

// ConsumeItemInstance consumes quantity from a specific inventory item instance ID.
func (s *Service) ConsumeItemInstance(ctx context.Context, characterID string, itemInstanceID string, quantity int) (coreinventory.Inventory, error) {
	if strings.TrimSpace(characterID) == "" {
		return coreinventory.Inventory{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemInstanceID) == "" {
		return coreinventory.Inventory{}, ErrItemNotFound
	}
	if quantity <= 0 {
		return coreinventory.Inventory{}, ErrInvalidQuantity
	}

	var resInv coreinventory.Inventory
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, found := inv.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}
		if inst.Quantity < quantity {
			return ErrInsufficientItemQuantity
		}

		if err := inv.Consume(itemInstanceID, quantity); err != nil {
			return err
		}

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		resInv = inv
		return nil
	})
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	return resInv, nil
}

// ConsumeItemDefinition consumes quantity across any item instances matching the definition ID.
func (s *Service) ConsumeItemDefinition(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (coreinventory.Inventory, error) {
	if strings.TrimSpace(characterID) == "" {
		return coreinventory.Inventory{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemDefinitionID) == "" {
		return coreinventory.Inventory{}, ErrItemNotFound
	}
	if quantity <= 0 {
		return coreinventory.Inventory{}, ErrInvalidQuantity
	}

	var resInv coreinventory.Inventory
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		if inv.Quantity(itemDefinitionID) < quantity {
			return ErrInsufficientItemQuantity
		}

		remaining := quantity
		// Iterate over items and consume matching definitions
		for _, inst := range inv.Items {
			if inst.DefinitionID == itemDefinitionID && inst.Quantity > 0 {
				toTake := inst.Quantity
				if toTake > remaining {
					toTake = remaining
				}
				_ = inv.Consume(inst.ID, toTake)
				remaining -= toTake
				if remaining <= 0 {
					break
				}
			}
		}

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		resInv = inv
		return nil
	})
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	return resInv, nil
}

// ExchangeRequest describes a compound atomic economic exchange.
type ExchangeRequest struct {
	CharacterID         string
	DeductGold          int
	AddGold             int
	DeductMedals        int
	AddMedals           int
	ConsumeInstanceID   string
	ConsumeInstanceQty  int
	ConsumeDefinitionID string
	ConsumeDefQty       int
	GrantDefinitionID   string
	GrantQuantity       int
}

// ExchangeResult describes the outcome of a compound economic exchange.
type ExchangeResult struct {
	Character   corecharacter.Character
	Inventory   coreinventory.Inventory
	GrantedItem *coreitem.Instance
}

// Exchange performs a compound atomic currency and inventory exchange following strict lock hierarchy.
func (s *Service) Exchange(ctx context.Context, req ExchangeRequest) (*ExchangeResult, error) {
	if strings.TrimSpace(req.CharacterID) == "" {
		return nil, ErrInvalidCharacterID
	}
	if req.DeductGold < 0 || req.AddGold < 0 || req.DeductMedals < 0 || req.AddMedals < 0 {
		return nil, ErrInvalidAmount
	}
	if req.ConsumeInstanceQty < 0 || req.ConsumeDefQty < 0 || req.GrantQuantity < 0 {
		return nil, ErrInvalidQuantity
	}

	var result *ExchangeResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character first (Deterministic lock order: characters -> inventory_items)
		char, err := s.findCharacter(txCtx, req.CharacterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// Validate gold and medal deductions
		if req.DeductGold > 0 {
			if char.Money < req.DeductGold {
				return ErrInsufficientGold
			}
			if err := char.DeductMoney(req.DeductGold); err != nil {
				return ErrInsufficientGold
			}
		}
		if req.DeductMedals > 0 {
			if char.SmallMedals < req.DeductMedals {
				return ErrInsufficientMedals
			}
			if err := char.DeductSmallMedals(req.DeductMedals); err != nil {
				return ErrInsufficientMedals
			}
		}
		if req.AddGold > 0 {
			_ = char.AddMoney(req.AddGold)
		}
		if req.AddMedals > 0 {
			_ = char.AddSmallMedals(req.AddMedals)
		}

		// 2. Lock Inventory next
		needsInventory := req.ConsumeInstanceID != "" || req.ConsumeDefinitionID != "" || req.GrantDefinitionID != ""
		var inv coreinventory.Inventory
		var grantedInst *coreitem.Instance

		if needsInventory {
			inv, err = s.findInventory(txCtx, req.CharacterID)
			if err != nil {
				return err
			}

			// Validate and consume item instance if requested
			if req.ConsumeInstanceID != "" && req.ConsumeInstanceQty > 0 {
				inst, found := inv.Find(req.ConsumeInstanceID)
				if !found {
					return ErrItemNotFound
				}
				if inst.Quantity < req.ConsumeInstanceQty {
					return ErrInsufficientItemQuantity
				}
				if err := inv.Consume(req.ConsumeInstanceID, req.ConsumeInstanceQty); err != nil {
					return err
				}
			}

			// Validate and consume item definition if requested
			if req.ConsumeDefinitionID != "" && req.ConsumeDefQty > 0 {
				if inv.Quantity(req.ConsumeDefinitionID) < req.ConsumeDefQty {
					return ErrInsufficientItemQuantity
				}
				remaining := req.ConsumeDefQty
				for _, inst := range inv.Items {
					if inst.DefinitionID == req.ConsumeDefinitionID && inst.Quantity > 0 {
						toTake := inst.Quantity
						if toTake > remaining {
							toTake = remaining
						}
						_ = inv.Consume(inst.ID, toTake)
						remaining -= toTake
						if remaining <= 0 {
							break
						}
					}
				}
			}

			// Grant item if requested
			if req.GrantDefinitionID != "" && req.GrantQuantity > 0 {
				newInst, err := coreitem.NewInstance(req.GrantDefinitionID, req.GrantQuantity)
				if err != nil {
					return err
				}
				if err := inv.Add(newInst); err != nil {
					return fmt.Errorf("%w: %v", ErrInventoryFull, err)
				}
				grantedInst = &newInst
			}

			if err := s.inventories.Save(txCtx, inv); err != nil {
				return err
			}
		}

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		result = &ExchangeResult{
			Character:   char,
			Inventory:   inv,
			GrantedItem: grantedInst,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
