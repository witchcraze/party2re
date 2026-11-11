package depot

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
)

type EquipmentRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	Save(ctx context.Context, value coreequipment.Equipment) error
}

func WithEquipmentRepository(repo EquipmentRepository) Option {
	return func(s *Service) { s.equipment = repo }
}

// Called after character/inventory locks and before Depot locks or inventory deletion.
func (s *Service) detachEquipment(ctx context.Context, char *corecharacter.Character, instanceID string) (bool, error) {
	eq, err := s.lockEquipment(ctx, char.ID)
	if err != nil || eq == nil {
		return false, err
	}
	changed, err := eq.Detach(char, instanceID)
	if err != nil || !changed {
		return false, err
	}
	if err := s.equipment.Save(ctx, *eq); err != nil {
		return false, err
	}
	return true, nil
}

// lockEquipment returns nil when the repository is not configured.
func (s *Service) lockEquipment(ctx context.Context, characterID string) (*coreequipment.Equipment, error) {
	if s.equipment == nil {
		// Explicit test adapters may omit equipment; production supplies the repository.
		return nil, nil
	}
	eq, err := s.equipment.FindByCharacterIDForUpdate(ctx, characterID)
	if err != nil {
		return nil, err
	}
	return &eq, nil
}
