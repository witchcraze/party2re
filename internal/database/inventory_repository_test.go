package database

import (
	"context"
	"os"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

	characterID := "00000000000000000000000000000011"
	inventory, err := coreinventory.New(characterID)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := item.NewInstance("potion", 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Add(instance); err != nil {
		t.Fatal(err)
	}

	repository, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO characters
			(id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience)
		VALUES (?, 'Inventory Test', 'starter', 'unspecified', 30, 6, 30, 6, 6, 6, 6, 200, 1, 0)
		ON DUPLICATE KEY UPDATE id = VALUES(id)
	`, characterID); err != nil {
		t.Fatal(err)
	}

	if err := repository.Save(context.Background(), inventory); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := repository.FindByCharacterID(context.Background(), characterID)
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
