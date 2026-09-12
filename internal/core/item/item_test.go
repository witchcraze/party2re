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

func TestDefinition_IsStackable(t *testing.T) {
	consumable, _ := NewDefinition("item-001", "Herb", 30)
	if !consumable.IsStackable() {
		t.Errorf("expected consumable to be stackable, got false")
	}

	weapon, _ := NewEquipmentDefinition("weapon-01", "Sword", 100, SlotMainHand)
	if weapon.IsStackable() {
		t.Errorf("expected weapon to be non-stackable, got true")
	}

	shield, _ := NewEquipmentDefinition("shield-01", "Shield", 80, SlotOffHand)
	if shield.IsStackable() {
		t.Errorf("expected shield to be non-stackable, got true")
	}

	armor, _ := NewEquipmentDefinition("armor-01", "Armor", 120, SlotBody)
	if armor.IsStackable() {
		t.Errorf("expected armor to be non-stackable, got true")
	}

	accessory, _ := NewEquipmentDefinition("acc-01", "Ring", 200, SlotAccessory)
	if accessory.IsStackable() {
		t.Errorf("expected accessory to be non-stackable, got true")
	}
}

func TestNewInstanceWithEnhancement_Invariants(t *testing.T) {
	// EnhancementLevel == 0 with quantity > 1 is valid (consumables/materials)
	inst, err := NewInstanceWithEnhancement("item-001", 5, 0)
	if err != nil || inst.Quantity != 5 || inst.EnhancementLevel != 0 {
		t.Fatalf("unexpected error for unenhanced stack: %v, inst: %+v", err, inst)
	}

	// EnhancementLevel > 0 with quantity == 1 is valid (enhanced equipment)
	instEnh, err := NewInstanceWithEnhancement("weapon-01", 1, 5)
	if err != nil || instEnh.Quantity != 1 || instEnh.EnhancementLevel != 5 {
		t.Fatalf("unexpected error for enhanced single item: %v, inst: %+v", err, instEnh)
	}

	// EnhancementLevel > 0 with quantity > 1 MUST fail invariant
	if _, err := NewInstanceWithEnhancement("weapon-01", 2, 5); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("expected ErrInvalidInstance for enhanced item with quantity > 1, got %v", err)
	}

	// Negative enhancement level fails
	if _, err := NewInstanceWithEnhancement("weapon-01", 1, -1); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("expected ErrInvalidInstance for negative enhancement, got %v", err)
	}
}

func TestDefinition_NewInstance_Invariants(t *testing.T) {
	weapon, _ := NewEquipmentDefinition("weapon-01", "Sword", 100, SlotMainHand)
	herb, _ := NewDefinition("item-001", "Herb", 30)

	// Consumable definition allows quantity > 1
	herbInst, err := herb.NewInstance(5)
	if err != nil || herbInst.Quantity != 5 {
		t.Fatalf("expected herb quantity 5, got err=%v, inst=%+v", err, herbInst)
	}

	// Equipment definition rejects quantity > 1
	if _, err := weapon.NewInstance(2); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("expected ErrInvalidInstance for equipment with quantity > 1, got %v", err)
	}

	// Equipment definition allows quantity == 1
	weapInst, err := weapon.NewInstance(1)
	if err != nil || weapInst.Quantity != 1 {
		t.Fatalf("expected weapon quantity 1, got err=%v, inst=%+v", err, weapInst)
	}

	// Equipment definition allows enhanced quantity == 1
	weapEnh, err := weapon.NewInstanceWithEnhancement(1, 3)
	if err != nil || weapEnh.Quantity != 1 || weapEnh.EnhancementLevel != 3 {
		t.Fatalf("expected weapon +3, got err=%v, inst=%+v", err, weapEnh)
	}

	// Equipment definition rejects enhanced quantity > 1
	if _, err := weapon.NewInstanceWithEnhancement(2, 3); !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("expected ErrInvalidInstance for enhanced weapon with quantity > 1, got %v", err)
	}
}

func TestInstance_CanStackWith(t *testing.T) {
	a, _ := NewInstanceWithEnhancement("item-001", 2, 0)
	b, _ := NewInstanceWithEnhancement("item-001", 3, 0)
	diffDef, _ := NewInstanceWithEnhancement("item-002", 1, 0)
	enhA, _ := NewInstanceWithEnhancement("weapon-01", 1, 5)
	enhB, _ := NewInstanceWithEnhancement("weapon-01", 1, 5)

	// Stackable consumable instances can stack
	if !a.CanStackWith(b, true) {
		t.Errorf("expected identical consumables to be stackable")
	}

	// Stackable is false (equipment): cannot stack
	if a.CanStackWith(b, false) {
		t.Errorf("expected CanStackWith=false when isStackable=false")
	}

	// Different definitions cannot stack
	if a.CanStackWith(diffDef, true) {
		t.Errorf("expected CanStackWith=false for different definitions")
	}

	// Enhanced items cannot stack
	if enhA.CanStackWith(enhB, true) {
		t.Errorf("expected CanStackWith=false for enhanced items")
	}
}

func TestDefinition_ValidateInstance(t *testing.T) {
	weapon, _ := NewEquipmentDefinition("weapon-01", "Sword", 100, SlotMainHand)
	herb, _ := NewDefinition("item-001", "Herb", 30)

	validWeapon, _ := NewInstanceWithEnhancement("weapon-01", 1, 0)
	invalidWeaponQty := Instance{ID: "w-2", DefinitionID: "weapon-01", Quantity: 2, EnhancementLevel: 0}
	wrongDefInstance, _ := NewInstance("item-001", 1)
	validHerb, _ := NewInstance("item-001", 10)

	if err := weapon.ValidateInstance(validWeapon); err != nil {
		t.Errorf("expected valid weapon, got %v", err)
	}
	if err := weapon.ValidateInstance(invalidWeaponQty); !errors.Is(err, ErrInvalidInstance) {
		t.Errorf("expected ErrInvalidInstance for weapon quantity > 1, got %v", err)
	}
	if err := weapon.ValidateInstance(wrongDefInstance); !errors.Is(err, ErrInvalidInstance) {
		t.Errorf("expected ErrInvalidInstance for wrong definition, got %v", err)
	}
	if err := herb.ValidateInstance(validHerb); err != nil {
		t.Errorf("expected valid herb stack, got %v", err)
	}
}

func TestIsStackableID(t *testing.T) {
	// From default catalog
	if !IsStackableID("item-001") {
		t.Errorf("expected item-001 to be stackable")
	}
	if IsStackableID("weapon-01") {
		t.Errorf("expected weapon-01 to be non-stackable")
	}

	// Heuristic fallback for test IDs
	if IsStackableID("sword-1") {
		t.Errorf("expected sword-1 to be non-stackable via prefix")
	}
	if IsStackableID("armor-heavy") {
		t.Errorf("expected armor-heavy to be non-stackable via prefix")
	}
	if !IsStackableID("potion-healing") {
		t.Errorf("expected potion-healing to be stackable")
	}
}
