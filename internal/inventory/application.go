package inventory

import (
	"context"
	"errors"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type Repository interface {
	Save(ctx context.Context, value coreinventory.Inventory) error
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

func (s *Service) Add(ctx context.Context, characterID string, value item.Instance) (coreinventory.Inventory, error) {
	inventory, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	if err := inventory.Add(value); err != nil {
		return coreinventory.Inventory{}, err
	}
	if err := s.repository.Save(ctx, inventory); err != nil {
		return coreinventory.Inventory{}, err
	}
	return inventory, nil
}

func (s *Service) Consume(ctx context.Context, characterID, instanceID string, quantity int) (coreinventory.Inventory, error) {
	inventory, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	if err := inventory.Consume(instanceID, quantity); err != nil {
		return coreinventory.Inventory{}, err
	}
	if err := s.repository.Save(ctx, inventory); err != nil {
		return coreinventory.Inventory{}, err
	}
	return inventory, nil
}
