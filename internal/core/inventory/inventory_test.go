package inventory

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func TestInventoryAddsAndCountsItemInstances(t *testing.T) {
	inventory, err := New("character-1")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := item.NewInstance("potion", 2)
	second, _ := item.NewInstance("potion", 3)
	if err := inventory.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := inventory.Add(second); err != nil {
		t.Fatal(err)
	}
	if got := inventory.Quantity("potion"); got != 5 {
		t.Fatalf("Quantity() = %d, want 5", got)
	}
}

func TestInventoryConsumesOwnedQuantityAndRemovesEmptyInstance(t *testing.T) {
	inventory, _ := New("character-1")
	value, _ := item.NewInstance("potion", 2)
	_ = inventory.Add(value)

	if err := inventory.Consume(value.ID, 1); err != nil {
		t.Fatal(err)
	}
	if inventory.Quantity("potion") != 1 {
		t.Fatalf("quantity after partial consume = %d, want 1", inventory.Quantity("potion"))
	}
	if err := inventory.Consume(value.ID, 1); err != nil {
		t.Fatal(err)
	}
	if inventory.Quantity("potion") != 0 || len(inventory.Items) != 0 {
		t.Fatalf("inventory after full consume = %#v", inventory)
	}
}

func TestInventoryRejectsUnownedAndExcessQuantity(t *testing.T) {
	inventory, _ := New("character-1")
	value, _ := item.NewInstance("potion", 2)
	_ = inventory.Add(value)

	if err := inventory.Consume("missing", 1); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("missing item error = %v", err)
	}
	if err := inventory.Consume(value.ID, 3); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("excess quantity error = %v", err)
	}
}

func TestInventoryFindReturnsInstanceByID(t *testing.T) {
	inventory, _ := New("character-1")
	value, _ := item.NewInstance("sword", 1)
	_ = inventory.Add(value)

	found, ok := inventory.Find(value.ID)
	if !ok || found.ID != value.ID || found.DefinitionID != "sword" {
		t.Fatalf("Find() = %#v, %v", found, ok)
	}

	if _, ok := inventory.Find("nonexistent"); ok {
		t.Fatal("Find() returned ok for nonexistent ID")
	}
}

func TestInventoryFindOnNilReturnsNotFound(t *testing.T) {
	var inv *Inventory
	if _, ok := inv.Find("any"); ok {
		t.Fatal("nil Inventory.Find() should return false")
	}
}

func TestInventoryUpdate(t *testing.T) {
	inv, _ := New("character-1")
	inst, _ := item.NewInstance("sword", 1)
	_ = inv.Add(inst)

	inst.EnhancementLevel = 3
	if err := inv.Update(inst); err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	found, ok := inv.Find(inst.ID)
	if !ok || found.EnhancementLevel != 3 {
		t.Fatalf("Find() after update = %#v, want enhancement level 3", found)
	}

	missing := inst
	missing.ID = "missing-id"
	if err := inv.Update(missing); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("Update() missing item expected ErrItemNotFound, got %v", err)
	}
}

func TestInventory_AddsMultipleEquipmentInstancesSeparately(t *testing.T) {
	inv, _ := New("char-equip-inv")
	sword1, _ := item.NewInstance("weapon-01", 1)
	sword2, _ := item.NewInstance("weapon-01", 1)

	if err := inv.Add(sword1); err != nil {
		t.Fatalf("Add(sword1) failed: %v", err)
	}
	if err := inv.Add(sword2); err != nil {
		t.Fatalf("Add(sword2) failed: %v", err)
	}

	if len(inv.Items) != 2 {
		t.Fatalf("expected 2 distinct equipment slots in inventory, got %d", len(inv.Items))
	}
	if inv.Items[0].ID == inv.Items[1].ID {
		t.Errorf("expected distinct instance IDs")
	}
	if inv.Quantity("weapon-01") != 2 {
		t.Errorf("Quantity(weapon-01) = %d, want 2", inv.Quantity("weapon-01"))
	}
}

func TestInventory_ConsumeItem_And_ConsumeOne(t *testing.T) {
	inv, _ := New("char-consume-inv")
	inst, _ := item.NewInstance("potion", 5)
	_ = inv.Add(inst)

	// 1. ConsumeOne
	if err := inv.ConsumeOne(inst.ID); err != nil {
		t.Fatalf("ConsumeOne failed: %v", err)
	}
	if inv.Quantity("potion") != 4 {
		t.Errorf("expected quantity 4, got %d", inv.Quantity("potion"))
	}

	// 2. ConsumeOneItem
	one, err := inv.ConsumeOneItem(inst.ID)
	if err != nil || one.Quantity != 1 || one.DefinitionID != "potion" {
		t.Fatalf("ConsumeOneItem failed: err=%v, one=%+v", err, one)
	}
	if inv.Quantity("potion") != 3 {
		t.Errorf("expected quantity 3, got %d", inv.Quantity("potion"))
	}

	// 3. ConsumeItem multi-quantity (consume 2 of 3)
	consumed, err := inv.ConsumeItem(inst.ID, 2)
	if err != nil || consumed.Quantity != 2 {
		t.Fatalf("ConsumeItem(2) failed: err=%v, consumed=%+v", err, consumed)
	}
	if inv.Quantity("potion") != 1 {
		t.Errorf("expected quantity 1, got %d", inv.Quantity("potion"))
	}

	// 4. ConsumeItem excess quantity (trying to consume 2 when 1 left)
	if _, err := inv.ConsumeItem(inst.ID, 2); !errors.Is(err, ErrInvalidQuantity) {
		t.Errorf("expected ErrInvalidQuantity for excess quantity, got %v", err)
	}

	// 5. ConsumeItem invalid quantity <= 0
	if _, err := inv.ConsumeItem(inst.ID, 0); !errors.Is(err, ErrInvalidQuantity) {
		t.Errorf("expected ErrInvalidQuantity for 0, got %v", err)
	}

	// 6. Final exhaust (consume 1 of 1 -> slot removed)
	last, err := inv.ConsumeItem(inst.ID, 1)
	if err != nil || last.Quantity != 1 {
		t.Fatalf("ConsumeItem(1) final failed: err=%v, last=%+v", err, last)
	}
	if len(inv.Items) != 0 || inv.Quantity("potion") != 0 {
		t.Errorf("expected inventory to be empty, got %+v", inv.Items)
	}
}

func TestInventoryCapacity(t *testing.T) {
	inv, err := New("character-cap")
	if err != nil {
		t.Fatal(err)
	}

	if inv.MaxCapacity() != DefaultMaxCapacity {
		t.Fatalf("MaxCapacity() = %d, want %d", inv.MaxCapacity(), DefaultMaxCapacity)
	}
	if inv.IsFull() {
		t.Fatal("empty inventory should not be full")
	}

	var nilInv *Inventory
	if nilInv.IsFull() {
		t.Fatal("nil inventory should not be full")
	}

	item1, _ := item.NewInstance("potion", 1)
	if err := inv.Add(item1); err != nil {
		t.Fatal(err)
	}
	if !inv.IsFull("potion") {
		t.Fatal("inventory with 1 potion should be full for tool hand")
	}
	if inv.IsFull("weapon-01") {
		t.Fatal("inventory with 1 potion should not be full for weapon hand")
	}
	if inv.IsFull("armor-01") {
		t.Fatal("inventory with 1 potion should not be full for armor hand")
	}
	if inv.IsFull() {
		t.Fatal("inventory with 1 potion should not be completely full (capacity 3)")
	}

	item2, _ := item.NewInstance("weapon-01", 1)
	if err := inv.Add(item2); err != nil {
		t.Fatal(err)
	}
	if !inv.IsFull("weapon-02") {
		t.Fatal("inventory with weapon should be full for weapon hand")
	}
	if inv.IsFull("armor-01") {
		t.Fatal("inventory with weapon and tool should not be full for armor hand")
	}
	if inv.IsFull() {
		t.Fatal("inventory with 2 items should not be completely full (capacity 3)")
	}

	item3, _ := item.NewInstance("armor-01", 1)
	if err := inv.Add(item3); err != nil {
		t.Fatal(err)
	}
	if !inv.IsFull() {
		t.Fatal("inventory with all 3 hand fields occupied should be full")
	}
}

func TestInventoryHandOccupancyToolShieldAccessorySharing(t *testing.T) {
	// Shields (SlotOffHand) and accessories (SlotAccessory) share kind 3 with consumables
	inv, _ := New("char-tool-sharing")

	shieldInst, _ := item.NewInstance("item-071", 1) // shield
	if err := inv.Add(shieldInst); err != nil {
		t.Fatal(err)
	}

	if !inv.IsHandOccupied(3) {
		t.Errorf("tool hand should be occupied by shield")
	}
	if !inv.IsFull("item-001") {
		t.Errorf("consumable should be considered full when holding shield")
	}
	if !inv.IsFull("item-012") {
		t.Errorf("accessory should be considered full when holding shield")
	}
	if inv.IsFull("weapon-01") {
		t.Errorf("weapon should not be full when holding shield")
	}
	if inv.IsFull("armor-01") {
		t.Errorf("armor should not be full when holding shield")
	}
}

func TestInventoryHeldToolID(t *testing.T) {
	tests := []struct {
		name     string
		items    []item.Instance
		wantHeld *string
	}{
		{
			name:     "empty inventory proves empty tool hand",
			items:    nil,
			wantHeld: ptr(""),
		},
		{
			name:     "single tool proves held tool",
			items:    []item.Instance{{ID: "a", DefinitionID: "item-001", Quantity: 1}},
			wantHeld: ptr("item-001"),
		},
		{
			name:     "single accessory proves held accessory",
			items:    []item.Instance{{ID: "a", DefinitionID: "item-012", Quantity: 1}},
			wantHeld: ptr("item-012"),
		},
		{
			name:     "single shield proves held item",
			items:    []item.Instance{{ID: "a", DefinitionID: "item-071", Quantity: 1}},
			wantHeld: ptr("item-071"),
		},
		{
			name:     "stacked tool row does not prove held tool",
			items:    []item.Instance{{ID: "a", DefinitionID: "item-001", Quantity: 2}},
			wantHeld: nil,
		},
		{
			name:     "single weapon does not prove held tool",
			items:    []item.Instance{{ID: "a", DefinitionID: "weapon-01", Quantity: 1}},
			wantHeld: nil,
		},
		{
			name:     "single armor does not prove held tool",
			items:    []item.Instance{{ID: "a", DefinitionID: "armor-01", Quantity: 1}},
			wantHeld: nil,
		},
		{
			name: "multiple items remain ambiguous unknown",
			items: []item.Instance{
				{ID: "a", DefinitionID: "item-001", Quantity: 1},
				{ID: "b", DefinitionID: "item-002", Quantity: 1},
			},
			wantHeld: nil,
		},
		{
			name: "weapon plus tool remains ambiguous unknown",
			items: []item.Instance{
				{ID: "a", DefinitionID: "weapon-01", Quantity: 1},
				{ID: "b", DefinitionID: "item-001", Quantity: 1},
			},
			wantHeld: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := New("char-hand")
			if err != nil {
				t.Fatal(err)
			}
			for _, inst := range tt.items {
				if err := inv.Add(inst); err != nil {
					t.Fatal(err)
				}
			}
			got := inv.HeldToolID()
			if tt.wantHeld == nil {
				if got != nil {
					t.Fatalf("HeldToolID() = %v, want nil", *got)
				}
			} else {
				if got == nil || *got != *tt.wantHeld {
					t.Fatalf("HeldToolID() = %v, want %v", got, *tt.wantHeld)
				}
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}
