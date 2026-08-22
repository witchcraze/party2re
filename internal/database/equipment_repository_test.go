package database

import (
	"context"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
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

	character, _ := corecharacter.New("Equipment Test")
	characters, _ := NewCharacterRepository(db)
	if err := characters.Save(context.Background(), character); err != nil {
		t.Fatal(err)
	}
	instanceID := "11111111111111111111111111111111"
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity)
		VALUES (?, ?, 'sword', 1)
	`, instanceID, character.ID); err != nil {
		t.Fatal(err)
	}
	value, _ := coreequipment.New(character.ID)
	value.Slots[item.SlotMainHand] = instanceID
	repository, _ := NewEquipmentRepository(db)
	if err := repository.Save(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	got, err := repository.FindByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CharacterID != value.CharacterID || got.Slots[item.SlotMainHand] != instanceID {
		t.Fatalf("FindByCharacterID() = %#v, want %#v", got, value)
	}
}
