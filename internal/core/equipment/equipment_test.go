package equipment

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestEquipmentEquipsOwnedItemAndExchangesSlot(t *testing.T) {
	value, _ := inventory.New("character-1")
	first, _ := item.NewInstance("sword", 1)
	second, _ := item.NewInstance("axe", 1)
	_ = value.Add(first)
	_ = value.Add(second)
	sword, _ := item.NewEquipmentDefinition("sword", "Training Sword", item.SlotMainHand)
	axe, _ := item.NewEquipmentDefinition("axe", "Training Axe", item.SlotMainHand)
	equipment, _ := New("character-1")

	if replaced, err := equipment.Equip(&value, sword, first.ID); err != nil || replaced != "" {
		t.Fatalf("first Equip() = %q, %v", replaced, err)
	}
	replaced, err := equipment.Equip(&value, axe, second.ID)
	if err != nil || replaced != first.ID {
		t.Fatalf("exchange Equip() = %q, %v, want %q", replaced, err, first.ID)
	}
	if got, ok := equipment.Equipped(item.SlotMainHand); !ok || got != second.ID {
		t.Fatalf("Equipped() = %q, %v", got, ok)
	}
}

func TestEquipmentRejectsUnownedAndIneligibleItems(t *testing.T) {
	value, _ := inventory.New("character-1")
	instance, _ := item.NewInstance("potion", 1)
	_ = value.Add(instance)
	potion, _ := item.NewDefinition("potion", "Recovery Potion")
	equipment, _ := New("character-1")

	if _, err := equipment.Equip(&value, potion, instance.ID); !errors.Is(err, ErrNotEquippable) {
		t.Fatalf("ineligible Equip() error = %v", err)
	}
	weapon, _ := item.NewEquipmentDefinition("weapon", "Training Weapon", item.SlotMainHand)
	if _, err := equipment.Equip(&value, weapon, "missing"); !errors.Is(err, ErrNotOwned) {
		t.Fatalf("unowned Equip() error = %v", err)
	}
}

func TestEquipmentUnequipsItem(t *testing.T) {
	value, _ := inventory.New("character-1")
	instance, _ := item.NewInstance("sword", 1)
	_ = value.Add(instance)
	weapon, _ := item.NewEquipmentDefinition("sword", "Training Sword", item.SlotMainHand)
	equipment, _ := New("character-1")
	_, _ = equipment.Equip(&value, weapon, instance.ID)
	if got, err := equipment.Unequip(item.SlotMainHand); err != nil || got != instance.ID {
		t.Fatalf("Unequip() = %q, %v", got, err)
	}
}
