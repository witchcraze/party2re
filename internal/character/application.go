package character

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

var (
	ErrNotFound      = corecharacter.ErrNotFound
	ErrInvalidPlayer = errors.New("player ID is required")
)

type Repository interface {
	Save(ctx context.Context, value corecharacter.Character) error
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type Service struct {
	repository Repository
}

type CreationOptions struct {
	JobID  string
	Gender string
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("character repository is nil")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Create(ctx context.Context, playerID, name string) (corecharacter.Character, error) {
	return s.CreateWithOptions(ctx, playerID, name, CreationOptions{})
}

func (s *Service) CreateWithOptions(ctx context.Context, playerID, name string, options CreationOptions) (corecharacter.Character, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return corecharacter.Character{}, ErrInvalidPlayer
	}
	if options.JobID == "" {
		options.JobID = corecharacter.DefaultJobID
	}
	if options.Gender == "" {
		options.Gender = corecharacter.DefaultGender
	}
	value, err := corecharacter.NewWithOptions(name, options.JobID, options.Gender, nil)
	if err != nil {
		return corecharacter.Character{}, err
	}
	value.PlayerID = playerID
	if err := s.repository.Save(ctx, value); err != nil {
		return corecharacter.Character{}, err
	}

	return value, nil
}

func (s *Service) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	return s.repository.FindByID(ctx, id)
}

func (s *Service) ListByPlayer(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	return s.repository.FindByPlayerID(ctx, playerID)
}

func (s *Service) Rebirth(ctx context.Context, id string) (corecharacter.Character, error) {
	char, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return corecharacter.Character{}, err
	}
	if err := progression.Rebirth(&char); err != nil {
		return corecharacter.Character{}, err
	}
	if err := s.repository.Update(ctx, char); err != nil {
		return corecharacter.Character{}, err
	}
	return char, nil
}
