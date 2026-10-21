package battle

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
)

// BuildParticipantWithState rebuilds equipment and abilities after buffered
// consumptions, without mutating durable inventory. A nil MP preserves stored MP.
func (s *Service) BuildParticipantWithState(ctx context.Context, characterID string, hp int, mp *int, consumed []corebattle.ConsumedItem) (corebattle.Participant, error) {
	if s.charRepo == nil {
		return corebattle.Participant{}, ErrCharacterNotFound
	}
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return corebattle.Participant{}, err
	}
	char.Stats.HP = hp
	if mp != nil {
		char.Stats.MP = *mp
	}
	return s.buildParticipantFromCharacter(ctx, char, consumed)
}

func consumeBufferedItems(inv *coreinventory.Inventory, equip *coreequipment.Equipment, consumed []corebattle.ConsumedItem) error {
	if len(consumed) == 0 {
		return nil
	}
	*inv = inv.Clone()
	*equip = equip.Clone()
	for _, entry := range consumed {
		if entry.Quantity <= 0 {
			return coreinventory.ErrInvalidQuantity
		}
		remaining := entry.Quantity
		// Clone the candidates because Consume can remove elements from inv.Items.
		for _, inst := range slices.Clone(inv.Items) {
			if inst.DefinitionID != entry.ID || (entry.InstanceID != "" && inst.ID != entry.InstanceID) {
				continue
			}
			take := min(inst.Quantity, remaining)
			if err := inv.Consume(inst.ID, take); err != nil {
				return err
			}
			if _, exists := inv.Find(inst.ID); !exists {
				for slot, id := range equip.Slots {
					if id == inst.ID {
						if _, err := equip.Unequip(slot); err != nil {
							return err
						}
					}
				}
			}
			remaining -= take
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return coreinventory.ErrItemNotFound
		}
	}
	return nil
}
