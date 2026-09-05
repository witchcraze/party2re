package database

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func TestInventoryRepositoryPersistsAndLoadsInventory(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := CreateTestCharacter(context.Background(), db, "Inventory Test")
	if err != nil {
		t.Fatal(err)
	}

	instance, err := item.NewInstance("potion", 2)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := CreateTestInventoryWithItems(context.Background(), db, character.ID, []item.Instance{instance})
	if err != nil {
		t.Fatal(err)
	}

	repository, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repository.FindByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID() error = %v", err)
	}
	if got.CharacterID != inventory.CharacterID || got.Quantity(instance.DefinitionID) != instance.Quantity {
		t.Fatalf("FindByCharacterID() = %#v, want %#v", got, inventory)
	}
}

func TestItemDefinitionRepositoryPersistsAndLoadsDefinition(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repository, err := NewItemDefinitionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	want, err := item.NewDefinition("potion", "Recovery Potion", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := repository.FindByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got != want {
		t.Fatalf("FindByID() = %#v, want %#v", got, want)
	}
}
