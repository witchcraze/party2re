package database

import (
	"context"
	"os"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestShopRepositoryCommitTransaction(t *testing.T) {
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
	shopRepo, err := NewShopRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Shop DB Test")
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
	inst, err := item.NewInstance("potion", 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(inst); err != nil {
		t.Fatal(err)
	}

	char.Money = 350
	if err := shopRepo.CommitTransaction(ctx, char, inv); err != nil {
		t.Fatalf("CommitTransaction() error = %v", err)
	}

	// Verify restored state
	restoredChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredChar.Money != 350 {
		t.Errorf("restored Character.Money = %d, want 350", restoredChar.Money)
	}

	restoredInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredInv.Items) != 1 || restoredInv.Items[0].Quantity != 3 {
		t.Errorf("restored Inventory = %#v", restoredInv)
	}
}

func TestShopRepositoryNilDB(t *testing.T) {
	if _, err := NewShopRepository(nil); err == nil {
		t.Fatal("NewShopRepository(nil) expected error, got nil")
	}
}
