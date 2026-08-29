package inn

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
	txProvider  TransactionProvider
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

func (s *Service) Rest(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if characterID == "" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}

	var updatedChar corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.findCharacter(txCtx, characterID)
		if err != nil {
			return err
		}

		fee := s.CalculateFee(char.Level)
		if char.Money < fee {
			return ErrInsufficientFunds
		}

		char.Money -= fee
		char.Stats.HP = char.Stats.MaxHP
		char.Stats.MP = char.Stats.MaxMP

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		updatedChar = char
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}

	return updatedChar, nil
}
