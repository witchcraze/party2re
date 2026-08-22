package equipment

import (
	"context"
	"errors"

	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
)

type Repository interface {
	Save(ctx context.Context, value coreequipment.Equipment) error
	FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("equipment repository is nil")
	}
	return &Service{repository: repository}, nil
}

func (s *Service) Equip(ctx context.Context, characterID string, owned coreequipment.Ownership, definition item.Definition, instanceID string) (coreequipment.Equipment, error) {
	value, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return coreequipment.Equipment{}, err
	}
	if _, err := value.Equip(owned, definition, instanceID); err != nil {
		return coreequipment.Equipment{}, err
	}
	if err := s.repository.Save(ctx, value); err != nil {
		return coreequipment.Equipment{}, err
	}
	return value, nil
}

func (s *Service) Unequip(ctx context.Context, characterID string, slot item.Slot) (coreequipment.Equipment, error) {
	value, err := s.repository.FindByCharacterID(ctx, characterID)
	if err != nil {
		return coreequipment.Equipment{}, err
	}
	if _, err := value.Unequip(slot); err != nil {
		return coreequipment.Equipment{}, err
	}
	if err := s.repository.Save(ctx, value); err != nil {
		return coreequipment.Equipment{}, err
	}
	return value, nil
}
