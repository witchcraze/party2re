package database

import (
	"context"
	"os"
	"testing"

	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestEquipmentRepositoryPersistsAndLoadsSlots(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := CreateTestCharacter(context.Background(), db, "Equipment Test")
	if err != nil {
		t.Fatal(err)
	}

	inventory, err := coreinventory.New(character.ID)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := item.NewInstance("sword", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Add(instance); err != nil {
		t.Fatal(err)
	}
	inventories, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventories.Save(context.Background(), inventory); err != nil {
		t.Fatal(err)
	}

	value, err := coreequipment.New(character.ID)
	if err != nil {
		t.Fatal(err)
	}
	value.Slots[item.SlotMainHand] = instance.ID
	repository, err := NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	got, err := repository.FindByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CharacterID != value.CharacterID || got.Slots[item.SlotMainHand] != instance.ID {
		t.Fatalf("FindByCharacterID() = %#v, want %#v", got, value)
	}
}
