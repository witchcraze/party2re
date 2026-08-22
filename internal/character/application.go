package character

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var ErrNotFound = corecharacter.ErrNotFound

type Repository interface {
	Save(ctx context.Context, value corecharacter.Character) error
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("character repository is nil")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Create(ctx context.Context, name string) (corecharacter.Character, error) {
	value, err := corecharacter.New(name)
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
