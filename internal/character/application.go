package character

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

var ErrNotFound = corecharacter.ErrNotFound

type Repository interface {
	Save(ctx context.Context, value corecharacter.Character) error
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
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

func (s *Service) Create(ctx context.Context, name string) (corecharacter.Character, error) {
	return s.CreateWithOptions(ctx, name, CreationOptions{})
}

func (s *Service) CreateWithOptions(ctx context.Context, name string, options CreationOptions) (corecharacter.Character, error) {
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
	if err := s.repository.Save(ctx, value); err != nil {
		return corecharacter.Character{}, err
	}

	return value, nil
}

func (s *Service) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	return s.repository.FindByID(ctx, id)
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
