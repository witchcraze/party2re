package character

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// InspectResult is the public status other players see; wallet money is not
// part of legacy shiraberu for another player.
type InspectResult struct {
	CharacterID string            `json:"character_id"`
	Name        string            `json:"name"`
	Gender      string            `json:"gender"`
	Level       int               `json:"level"`
	JobID       string            `json:"job_id"`
	SP          int               `json:"sp"`
	OldJobID    string            `json:"old_job_id"`
	OldSP       int               `json:"old_sp"`
	Tired       int               `json:"tired"`
	MaxHP       int               `json:"max_hp"`
	MaxMP       int               `json:"max_mp"`
	Attack      int               `json:"attack"`
	Defense     int               `json:"defense"`
	Agility     int               `json:"agility"`
	Weapon      *EquippedItemInfo `json:"weapon,omitempty"`
	Armor       *EquippedItemInfo `json:"armor,omitempty"`
	Accessory   *EquippedItemInfo `json:"accessory,omitempty"`
	Self        bool              `json:"self"`
}

type EquippedItemInfo struct {
	InstanceID       string `json:"instance_id"`
	DefinitionID     string `json:"definition_id"`
	Name             string `json:"name"`
	CustomName       string `json:"custom_name,omitempty"`
	EnhancementLevel int    `json:"enhancement_level"`
}

// Inspector projects the status part of the common しらべる (system.cgi shiraberu).
type Inspector struct {
	Characters interface {
		FindByID(ctx context.Context, id string) (corecharacter.Character, error)
		FindByName(ctx context.Context, name string) (corecharacter.Character, error)
	}
	Equipment interface {
		FindByCharacterID(ctx context.Context, characterID string) (coreequipment.Equipment, error)
	}
	Inventory interface {
		FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	}
	Items interface {
		FindByID(id string) (coreitem.Definition, error)
	}
}

// Inspect falls back to the inspector's own status when the target is omitted
// or does not exist, as legacy does; other read failures propagate.
func (s Inspector) Inspect(ctx context.Context, inspectorID, targetID, targetName string) (InspectResult, error) {
	var (
		char corecharacter.Character
		err  = corecharacter.ErrNotFound
	)
	switch {
	case targetID != "":
		char, err = s.Characters.FindByID(ctx, targetID)
	case targetName != "":
		char, err = s.Characters.FindByName(ctx, targetName)
	}
	if errors.Is(err, corecharacter.ErrNotFound) {
		char, err = s.Characters.FindByID(ctx, inspectorID)
	}
	if err != nil {
		return InspectResult{}, err
	}
	equip, err := s.Equipment.FindByCharacterID(ctx, char.ID)
	if err != nil {
		return InspectResult{}, err
	}
	inv, err := s.Inventory.FindByCharacterID(ctx, char.ID)
	if err != nil {
		return InspectResult{}, err
	}
	// Legacy shows armor only when a weapon is equipped ("if $p{wea}"), a copy
	// defect; armor is shown whenever equipped.
	return InspectResult{
		CharacterID: char.ID, Name: char.Name, Gender: char.Gender, Level: char.Level,
		JobID: char.JobID, SP: char.SP, OldJobID: char.OldJobID, OldSP: char.OldSP,
		Tired: char.Tired, MaxHP: char.Stats.MaxHP, MaxMP: char.Stats.MaxMP,
		Attack: char.Stats.Attack, Defense: char.Stats.Defense, Agility: char.Stats.Agility,
		Weapon:    s.equipped(equip, inv, coreitem.SlotMainHand, char.WeaponCustomName),
		Armor:     s.equipped(equip, inv, coreitem.SlotBody, char.ArmorCustomName),
		Accessory: s.equipped(equip, inv, coreitem.SlotAccessory, ""),
		Self:      char.ID == inspectorID,
	}, nil
}

func (s Inspector) equipped(equip coreequipment.Equipment, inv coreinventory.Inventory, slot coreitem.Slot, customName string) *EquippedItemInfo {
	id, ok := equip.Equipped(slot)
	if !ok {
		return nil
	}
	inst, ok := inv.Find(id)
	if !ok {
		return nil
	}
	name := inst.DefinitionID
	if def, err := s.Items.FindByID(inst.DefinitionID); err == nil && def.Name != "" {
		name = def.Name
	}
	return &EquippedItemInfo{InstanceID: inst.ID, DefinitionID: inst.DefinitionID, Name: name, CustomName: customName, EnhancementLevel: inst.EnhancementLevel}
}
