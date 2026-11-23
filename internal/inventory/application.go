package inventory

import (
	"context"
	"errors"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

type Repository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("inventory repository is nil")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return s.repository.FindByCharacterID(ctx, characterID)
}
