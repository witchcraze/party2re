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
	Update(ctx context.Context, value corecharacter.Character) error
}

type Service struct {
	characters  CharacterRepository
	feePerLevel int
}

func NewService(characters CharacterRepository) (*Service, error) {
	return NewServiceWithFee(characters, DefaultFeePerLevel)
}

func NewServiceWithFee(characters CharacterRepository, feePerLevel int) (*Service, error) {
	if characters == nil {
		return nil, ErrNilRepository
	}
	if feePerLevel < 0 {
		return nil, ErrInvalidFee
	}
	return &Service{characters: characters, feePerLevel: feePerLevel}, nil
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
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return corecharacter.Character{}, err
	}

	fee := s.CalculateFee(char.Level)
	if char.Money < fee {
		return corecharacter.Character{}, ErrInsufficientFunds
	}

	char.Money -= fee
	char.Stats.HP = char.Stats.MaxHP
	char.Stats.MP = char.Stats.MaxMP

	if err := s.characters.Update(ctx, char); err != nil {
		return corecharacter.Character{}, err
	}

	return char, nil
}
