package blacksmith

import (
	"context"
	"errors"
	"fmt"

	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

// EquippedItem is an equipped weapon or armor with its effective display name.
type EquippedItem struct {
	ItemDefinitionID string `json:"item_definition_id"`
	Name             string `json:"name"`
	CustomName       string `json:"custom_name"`
}

// StoredWeapon is a storage record with its effective display name (blacksmith.cgi:204).
type StoredWeapon struct {
	Deposit
	Name string `json:"name"`
}

// Observation holds the read-only facts behind the legacy header and prompts (blacksmith.cgi:37-50,110-139,192-237).
type Observation struct {
	Crystal          int            `json:"crystal"`
	Weapon           *EquippedItem  `json:"weapon"`
	WeaponSealID     int            `json:"weapon_seal_id"`
	Armor            *EquippedItem  `json:"armor"`
	AvailableSealIDs []int          `json:"available_seal_ids"`
	Deposits         []StoredWeapon `json:"deposits"`
}

// Observe reads the owner's customization and storage facts without mutation.
func (s *Service) Observe(ctx context.Context, characterID string) (Observation, error) {
	deposits, err := s.ListDeposits(ctx, characterID)
	if err != nil {
		return Observation{}, err
	}
	if s.equipment == nil {
		return Observation{}, errors.New("equipment repository not configured")
	}
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return Observation{}, err
	}
	equip, err := s.equipment.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Observation{}, err
	}
	inv, err := s.inventories.FindByCharacterID(ctx, characterID)
	if err != nil {
		return Observation{}, err
	}
	o := Observation{Crystal: char.Crystal, WeaponSealID: char.WeaponSeal, AvailableSealIDs: []int{}, Deposits: make([]StoredWeapon, 0, len(deposits))}
	if o.Weapon, err = s.equippedItem(equip, inv, item.SlotMainHand, char.WeaponCustomName); err != nil {
		return Observation{}, err
	}
	if o.Armor, err = s.equippedItem(equip, inv, item.SlotBody, char.ArmorCustomName); err != nil {
		return Observation{}, err
	}
	if o.Weapon != nil {
		for _, seal := range ListAvailableSeals(o.Weapon.ItemDefinitionID) {
			o.AvailableSealIDs = append(o.AvailableSealIDs, seal.ID)
		}
	}
	for _, deposit := range deposits {
		name, err := s.effectiveName(deposit.ItemDefinitionID, deposit.CustomName)
		if err != nil {
			return Observation{}, err
		}
		o.Deposits = append(o.Deposits, StoredWeapon{Deposit: deposit, Name: name})
	}
	return o, nil
}

// A dangling equipped instance is surfaced rather than observed as an empty slot.
func (s *Service) equippedItem(equip coreequipment.Equipment, inv coreinventory.Inventory, slot item.Slot, customName string) (*EquippedItem, error) {
	instanceID, ok := equip.Equipped(slot)
	if !ok || instanceID == "" {
		return nil, nil
	}
	instance, found := inv.Find(instanceID)
	if !found {
		return nil, fmt.Errorf("equipped %s instance %s is missing from inventory", slot, instanceID)
	}
	name, err := s.effectiveName(instance.DefinitionID, customName)
	if err != nil {
		return nil, err
	}
	return &EquippedItem{ItemDefinitionID: instance.DefinitionID, Name: name, CustomName: customName}, nil
}

func (s *Service) effectiveName(definitionID, customName string) (string, error) {
	if customName != "" {
		return customName, nil
	}
	def, err := s.catalog.FindByID(definitionID)
	if err != nil {
		return "", err
	}
	return def.Name, nil
}
