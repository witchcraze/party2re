package database

import (
	"context"
	"os"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestAlchemyRepositoryNilDB(t *testing.T) {
	if _, err := NewAlchemyRepository(nil); err == nil {
		t.Fatal("NewAlchemyRepository(nil) expected error, got nil")
	}
}

func TestAlchemyRepositoryCommitSynthesis(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	alcRepo, err := NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Alchemy DB Test")
	if err != nil {
		t.Fatal(err)
	}
	char.Money = 500
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	superHerb, err := item.NewInstance("item-002", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(superHerb)

	char.Money = 400
	if err := alcRepo.CommitSynthesis(ctx, char, inv); err != nil {
		t.Fatalf("CommitSynthesis() error = %v", err)
	}

	restoredChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredChar.Money != 400 {
		t.Errorf("restored character money = %d, want 400", restoredChar.Money)
	}

	restoredInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredInv.Items) != 1 || restoredInv.Items[0].DefinitionID != "item-002" {
		t.Errorf("restored inventory item = %#v, want item-002", restoredInv)
	}
}
