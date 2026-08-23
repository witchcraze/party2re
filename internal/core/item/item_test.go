package item

import (
	"errors"
	"testing"
)

func TestNewDefinitionSeparatesDefinitionIdentityFromInstances(t *testing.T) {
	definition, err := NewDefinition(" potion ", " Recovery Potion ", 30)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := NewInstance(definition.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if definition.ID != "potion" || definition.Name != "Recovery Potion" || definition.Price != 30 ||
		instance.DefinitionID != definition.ID || instance.ID == "" || instance.Quantity != 2 {
		t.Fatalf("definition = %#v, instance = %#v", definition, instance)
	}
}

func TestItemRejectsInvalidValues(t *testing.T) {
	if _, err := NewDefinition("", "Potion", 10); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewDefinition() error = %v", err)
	}
	if _, err := NewDefinition("potion", "Potion", -1); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewDefinition(negative price) error = %v", err)
	}
	if _, err := NewInstance("", 1); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("NewInstance() error = %v", err)
	}
	if _, err := NewInstance("potion", 0); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("NewInstance() error = %v", err)
	}
}

func TestNewEquipmentDefinitionSetsSlotAndPrice(t *testing.T) {
	def, err := NewEquipmentDefinition("sword-01", "Training Sword", 100, SlotMainHand)
	if err != nil {
		t.Fatal(err)
	}
	if def.ID != "sword-01" || def.Name != "Training Sword" || def.Price != 100 || def.Slot != SlotMainHand {
		t.Fatalf("NewEquipmentDefinition() = %#v", def)
	}
}

func TestNewEquipmentDefinitionRejectsInvalidSlotAndPrice(t *testing.T) {
	if _, err := NewEquipmentDefinition("sword", "Sword", 100, SlotNone); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewEquipmentDefinition(SlotNone) error = %v", err)
	}
	if _, err := NewEquipmentDefinition("sword", "Sword", -1, SlotMainHand); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewEquipmentDefinition(negative price) error = %v", err)
	}
	if _, err := NewEquipmentDefinition("sword", "Sword", 50, Slot("bad")); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("NewEquipmentDefinition(bad slot) error = %v", err)
	}
}
