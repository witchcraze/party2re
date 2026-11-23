package inventory

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func TestInventoryRejectsUnownedAndExcessQuantity(t *testing.T) {
	inventory, _ := New("character-1")
	value, _ := item.NewInstance("potion", 1)
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

func TestInventoryConsumesPreviouslyStoredStack(t *testing.T) {
	inst, _ := item.NewInstance("potion", 5)
	inv := Inventory{CharacterID: "char-consume-inv", Items: []item.Instance{inst}}

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

func TestInventoryAddRejectsNonUnitQuantity(t *testing.T) {
	inv, _ := New("char-hand")
	for _, quantity := range []int{-1, 0, 2} {
		stack := item.Instance{ID: "stack", DefinitionID: "item-001", Quantity: quantity}
		if err := inv.Add(stack); !errors.Is(err, ErrInvalidQuantity) {
			t.Errorf("Add with quantity %d = %v, want ErrInvalidQuantity", quantity, err)
		}
	}
	if len(inv.Items) != 0 {
		t.Fatalf("rejected writes changed inventory: %+v", inv.Items)
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
	for _, definitionID := range []string{"item-001", "item-012"} {
		inst, _ := item.NewInstance(definitionID, 1)
		if err := inv.Add(inst); !errors.Is(err, ErrInventoryFull) {
			t.Errorf("Add(%s) = %v, want ErrInventoryFull", definitionID, err)
		}
	}
	if len(inv.Items) != 1 || inv.Items[0] != shieldInst {
		t.Fatalf("rejected writes changed inventory: %+v", inv.Items)
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
			name:     "weapon-only inventory proves empty tool hand",
			items:    []item.Instance{{ID: "a", DefinitionID: "weapon-01", Quantity: 1}},
			wantHeld: ptr(""),
		},
		{
			name: "weapon and armor beside a tool prove that tool",
			items: []item.Instance{
				{ID: "a", DefinitionID: "weapon-01", Quantity: 1},
				{ID: "b", DefinitionID: "armor-01", Quantity: 1},
				{ID: "c", DefinitionID: "item-001", Quantity: 1},
			},
			wantHeld: ptr("item-001"),
		},
		{
			name: "two tool-hand rows break the hand invariant and stay unknown",
			items: []item.Instance{
				{ID: "a", DefinitionID: "item-001", Quantity: 1},
				{ID: "b", DefinitionID: "item-002", Quantity: 1},
			},
			wantHeld: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := Inventory{CharacterID: "char-hand", Items: tt.items}
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
