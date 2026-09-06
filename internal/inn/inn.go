package inn

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	DefaultFeePerLevel = 5
	MinFee             = 5
)

var (
	ErrNilRepository     = errors.New("character repository is nil")
	ErrInsufficientFunds = errors.New("insufficient gold to rest at inn")
	ErrInvalidFee        = errors.New("fee per level must be non-negative")
)

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
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

func WithTransactionRunner(runner TransactionRunner) Option {
	return func(s *Service) {
		s.runner = runner
	}
}

func WithEconomy(eco *economy.Service) Option {
	return func(s *Service) {
		s.runner = eco
	}
}

type Service struct {
	characters  CharacterRepository
	txProvider  TransactionProvider
	runner      TransactionRunner
	feePerLevel int
}

func NewService(characters CharacterRepository, opts ...Option) (*Service, error) {
	return NewServiceWithFee(characters, DefaultFeePerLevel, opts...)
}

func NewServiceWithFee(characters CharacterRepository, feePerLevel int, opts ...Option) (*Service, error) {
	if characters == nil {
		return nil, ErrNilRepository
	}
	if feePerLevel < 0 {
		return nil, ErrInvalidFee
	}
	s := &Service{characters: characters, feePerLevel: feePerLevel}
	for _, opt := range opts {
		opt(s)
	}

	if s.runner == nil {
		var ecoOpts []economy.Option
		if s.txProvider != nil {
			ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
		}
		eco, err := economy.NewService(characters, &noopInventoryRepo{}, ecoOpts...)
		if err != nil {
			return nil, err
		}
		s.runner = eco
	}

	return s, nil
}

func (s *Service) CalculateFee(level int) int {
	if level < 1 {
		level = 1
	}
	fee := level * s.feePerLevel
	if fee < MinFee {
		return MinFee
	}
	return fee
}

func (s *Service) Rest(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if characterID == "" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}

	res, err := s.runner.ExecuteTransaction(ctx, economy.TransactionRequest{
		CharacterID: characterID,
		CostFunc: func(char corecharacter.Character) (economy.ResourceCost, error) {
			return economy.ResourceCost{Gold: s.CalculateFee(char.Level)}, nil
		},
	}, func(tc *economy.TxContext) error {
		tc.Character.Stats.HP = tc.Character.Stats.MaxHP
		tc.Character.Stats.MP = tc.Character.Stats.MaxMP
		return nil
	})
	if err != nil {
		if errors.Is(err, economy.ErrInsufficientGold) {
			return corecharacter.Character{}, ErrInsufficientFunds
		}
		if errors.Is(err, economy.ErrCharacterNotFound) {
			return corecharacter.Character{}, corecharacter.ErrNotFound
		}
		return corecharacter.Character{}, err
	}

	return res.Character, nil
}

type noopInventoryRepo struct{}

func (n *noopInventoryRepo) FindByCharacterID(_ context.Context, charID string) (coreinventory.Inventory, error) {
	return coreinventory.New(charID)
}

func (n *noopInventoryRepo) FindByCharacterIDForUpdate(_ context.Context, charID string) (coreinventory.Inventory, error) {
	return coreinventory.New(charID)
}

func (n *noopInventoryRepo) Save(_ context.Context, _ coreinventory.Inventory) error {
	return nil
}
