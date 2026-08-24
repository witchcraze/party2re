package database

import (
	"context"
	"os"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestBlacksmithRepositoryNilDB(t *testing.T) {
	if _, err := NewBlacksmithRepository(nil); err == nil {
		t.Fatal("NewBlacksmithRepository(nil) expected error, got nil")
	}
}

func TestBlacksmithRepositoryCommitEnhancement(t *testing.T) {
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
	bsRepo, err := NewBlacksmithRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Blacksmith DB Test")
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
	sword, err := item.NewInstanceWithEnhancement("weapon-01", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	_ = inv.Add(sword)

	char.Money = 400
	if err := bsRepo.CommitEnhancement(ctx, char, inv); err != nil {
		t.Fatalf("CommitEnhancement() error = %v", err)
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
	if len(restoredInv.Items) != 1 || restoredInv.Items[0].EnhancementLevel != 2 {
		t.Errorf("restored item enhancement level = %#v, want level 2", restoredInv)
	}
}
