package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/id"
)

func TestBlacksmithRepositoryNilDB(t *testing.T) {
	if _, err := NewBlacksmithRepository(nil); err == nil {
		t.Fatal("NewBlacksmithRepository(nil) expected error, got nil")
	}
}

func TestBlacksmithRepositoryStorage_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	bsRepo, err := NewBlacksmithRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Blacksmith Storage Test")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial list should be empty
	initialList, err := bsRepo.ListByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID: %v", err)
	}
	if len(initialList) != 0 {
		t.Fatalf("expected 0 deposits initially, got %d", len(initialList))
	}

	// 2. Save a deposit in slot 1
	dep1 := blacksmith.Deposit{
		ID:               id.New(),
		CharacterID:      char.ID,
		Slot:             1,
		ItemDefinitionID: "weapon-01",
		SealID:           2,
		CustomName:       "Excalibur",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}
	if err := bsRepo.Save(ctx, dep1); err != nil {
		t.Fatalf("Save slot 1: %v", err)
	}

	// 3. Save a deposit in slot 2
	dep2 := blacksmith.Deposit{
		ID:               id.New(),
		CharacterID:      char.ID,
		Slot:             2,
		ItemDefinitionID: "weapon-50",
		SealID:           8,
		CustomName:       "Frostblade",
		CreatedAt:        time.Now().UTC().Truncate(time.Second),
	}
	if err := bsRepo.Save(ctx, dep2); err != nil {
		t.Fatalf("Save slot 2: %v", err)
	}

	// 4. Verify list returns both deposits ordered by slot
	deposits, err := bsRepo.ListByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID: %v", err)
	}
	if len(deposits) != 2 {
		t.Fatalf("expected 2 deposits, got %d", len(deposits))
	}
	if deposits[0].Slot != 1 || deposits[0].ItemDefinitionID != "weapon-01" || deposits[0].SealID != 2 || deposits[0].CustomName != "Excalibur" {
		t.Errorf("unexpected deposit 1: %+v", deposits[0])
	}
	if deposits[1].Slot != 2 || deposits[1].ItemDefinitionID != "weapon-50" || deposits[1].SealID != 8 || deposits[1].CustomName != "Frostblade" {
		t.Errorf("unexpected deposit 2: %+v", deposits[1])
	}

	// 5. Verify ListByCharacterIDForUpdate within transaction
	err = RunInTx(ctx, db, func(txCtx context.Context) error {
		txDeposits, err := bsRepo.ListByCharacterIDForUpdate(txCtx, char.ID)
		if err != nil {
			return err
		}
		if len(txDeposits) != 2 {
			t.Fatalf("expected 2 deposits in tx, got %d", len(txDeposits))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTx: %v", err)
	}

	// 6. Delete slot 1
	if err := bsRepo.Delete(ctx, char.ID, 1); err != nil {
		t.Fatalf("Delete slot 1: %v", err)
	}

	// 7. Verify only slot 2 remains
	remaining, err := bsRepo.ListByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListByCharacterID after delete: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Slot != 2 {
		t.Fatalf("expected only slot 2 to remain, got %+v", remaining)
	}
}
