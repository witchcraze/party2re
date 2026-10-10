package depot

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
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
	if s.equipment == nil {
		// Explicit test adapters may omit equipment; production supplies the repository.
		return false, nil
	}
	eq, err := s.equipment.FindByCharacterIDForUpdate(ctx, char.ID)
	if err != nil {
		return false, err
	}
	changed := false
	for slot, equippedID := range eq.Slots {
		if equippedID != instanceID {
			continue
		}
		if _, err := eq.Unequip(slot); err != nil {
			return false, err
		}
		switch slot {
		case item.SlotMainHand:
			char.ClearWeaponCustomization()
		case item.SlotBody:
			char.ClearArmorCustomization()
		}
		changed = true
	}
	if !changed {
		return false, nil
	}
	if err := s.equipment.Save(ctx, eq); err != nil {
		return false, err
	}
	return true, nil
}
