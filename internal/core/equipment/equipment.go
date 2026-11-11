package equipment

import (
	"errors"
	"maps"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrInvalidEquipment = errors.New("equipment is invalid")
	ErrNotOwned         = errors.New("item is not owned")
	ErrNotEquippable    = errors.New("item is not equippable")
	ErrSlotOccupied     = errors.New("equipment slot is occupied")
	ErrSlotEmpty        = errors.New("equipment slot is empty")
)

type Ownership interface {
	Find(instanceID string) (item.Instance, bool)
}

type Equipment struct {
	CharacterID string
	Slots       map[item.Slot]string
}

// Clone returns equipment whose slot mutations are isolated from the original.
func (e Equipment) Clone() Equipment {
	e.Slots = maps.Clone(e.Slots)
	return e
}

func New(characterID string) (Equipment, error) {
	if strings.TrimSpace(characterID) == "" {
		return Equipment{}, ErrInvalidEquipment
	}
	return Equipment{CharacterID: strings.TrimSpace(characterID), Slots: make(map[item.Slot]string)}, nil
}

func (e *Equipment) Equip(owned Ownership, definition item.Definition, instanceID string) (string, error) {
	if e == nil || e.CharacterID == "" || owned == nil || instanceID == "" {
		return "", ErrInvalidEquipment
	}
	if definition.Slot == item.SlotNone {
		return "", ErrNotEquippable
	}
	instance, ok := owned.Find(instanceID)
	if !ok || instance.DefinitionID != definition.ID {
		return "", ErrNotOwned
	}
	replaced := e.Slots[definition.Slot]
	e.Slots[definition.Slot] = instanceID
	return replaced, nil
}

func (e *Equipment) Unequip(slot item.Slot) (string, error) {
	if e == nil || e.CharacterID == "" {
		return "", ErrInvalidEquipment
	}
	instanceID, ok := e.Slots[slot]
	if !ok {
		return "", ErrSlotEmpty
	}
	delete(e.Slots, slot)
	return instanceID, nil
}

// Detach unlinks every slot holding instanceID before its inventory row is
// consumed, clearing the weapon or armor customization tied to that slot.
func (e *Equipment) Detach(char *corecharacter.Character, instanceID string) (bool, error) {
	changed := false
	for slot, equippedID := range e.Slots {
		if equippedID != instanceID {
			continue
		}
		if _, err := e.Unequip(slot); err != nil {
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
	return changed, nil
}

func (e *Equipment) Equipped(slot item.Slot) (string, bool) {
	if e == nil {
		return "", false
	}
	instanceID, ok := e.Slots[slot]
	return instanceID, ok
}
