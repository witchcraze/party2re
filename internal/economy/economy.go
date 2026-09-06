package economy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/event"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

// WithEventDispatcher sets the in-process domain event dispatcher.
func WithEventDispatcher(dispatcher *event.Dispatcher) Option {
	return func(s *Service) {
		s.dispatcher = dispatcher
	}
}

// Service provides standardized transactional operations for wallet currency and inventory items.
type Service struct {
	characters  CharacterRepository
	inventories InventoryRepository
	txProvider  TransactionProvider
	dispatcher  *event.Dispatcher
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
