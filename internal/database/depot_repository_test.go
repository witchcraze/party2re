package database

import (
	"context"
	"os"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestDepotRepositoryNilDB(t *testing.T) {
	if _, err := NewDepotRepository(nil); err == nil {
		t.Fatal("NewDepotRepository(nil) expected error, got nil")
	}
}

func TestDepotRepositorySaveAndFind(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	depotRepo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Depot DB Test")
	if err != nil {
		t.Fatal(err)
	}

	dep, err := depot.NewDepot(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep.Gold = 1000
	inst, err := item.NewInstance("item-001", 5)
	if err != nil {
		t.Fatal(err)
	}
	_ = dep.AddItem(inst)

	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	restored, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID() error = %v", err)
	}

	if restored.CharacterID != char.ID || restored.Gold != 1000 || len(restored.Items) != 1 || restored.Items[0].Quantity != 5 {
		t.Fatalf("restored depot mismatch: %#v", restored)
	}
}

func TestDepotRepositoryExecuteTransaction(t *testing.T) {
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
	depotRepo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Depot Tx Test")
	if err != nil {
		t.Fatal(err)
	}
	char.Money = 300
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	potion, _ := item.NewInstance("item-001", 2)
	_ = inv.Add(potion)
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	service, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)
	if err != nil {
		t.Fatal(err)
	}

	// Deposit gold
	dep, err := service.DepositGold(ctx, char.ID, 100)
	if err != nil {
		t.Fatalf("DepositGold error: %v", err)
	}
	if dep.Gold != 100 {
		t.Errorf("depot gold = %d, want 100", dep.Gold)
	}

	// Deposit item
	dep, err = service.DepositItem(ctx, char.ID, potion.ID)
	if err != nil {
		t.Fatalf("DepositItem error: %v", err)
	}
	if len(dep.Items) != 1 {
		t.Errorf("depot items count = %d, want 1", len(dep.Items))
	}

	// Verify restored inventory is empty
	restoredInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredInv.Items) != 0 {
		t.Errorf("restored inventory count = %d, want 0", len(restoredInv.Items))
	}
}
