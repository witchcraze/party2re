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
