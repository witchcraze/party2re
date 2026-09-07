package depot_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestDepotLifecycle_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	createdChar, err := database.CreateTestCharacterWithFunds(ctx, db, "Depot Tester", 500000)
	if err != nil {
		t.Fatal(err)
	}

	depotService, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial GetDepot
	initialDep, err := depotService.GetDepot(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("GetDepot error = %v", err)
	}
	if initialDep.ExDepot != 0 || len(initialDep.Items) != 0 {
		t.Fatalf("initial depot not empty: %#v", initialDep)
	}

	// 2. Deposit an item
	potion, err := item.NewInstance("item-001", 3)
	if err != nil {
		t.Fatalf("NewInstance error = %v", err)
	}
	_, err = database.CreateTestInventoryWithItems(ctx, db, createdChar.ID, []item.Instance{potion})
	if err != nil {
		t.Fatalf("CreateTestInventoryWithItems error = %v", err)
	}

	dep, err := depotService.DepositItem(ctx, createdChar.ID, potion.ID)
	if err != nil {
		t.Fatalf("DepositItem error = %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-001" {
		t.Fatalf("unexpected depot items: %#v", dep.Items)
	}

	// 3. Expand depot capacity
	dep, err = depotService.Expand(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("Expand error = %v", err)
	}
	if dep.ExDepot != 1 || dep.Capacity != 10 {
		t.Errorf("depot ExDepot = %d, Capacity = %d, want 1, 10", dep.ExDepot, dep.Capacity)
	}

	// 4. Withdraw Item
	dep, err = depotService.WithdrawItem(ctx, createdChar.ID, potion.ID)
	if err != nil {
		t.Fatalf("WithdrawItem error = %v", err)
	}
	if len(dep.Items) != 0 {
		t.Fatalf("depot items count = %d, want 0", len(dep.Items))
	}

	// Verify database persistence
	restoredChar, err := charRepo.FindByID(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("FindByID error = %v", err)
	}
	expectedMoney := 500000 - 200000 // 1 expansion cost
	if restoredChar.Money != expectedMoney {
		t.Errorf("character money = %d, want %d", restoredChar.Money, expectedMoney)
	}

	restoredDep, err := depotRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID error = %v", err)
	}
	if restoredDep.ExDepot != 1 || restoredDep.Capacity != 10 {
		t.Errorf("restored depot mismatch: %#v", restoredDep)
	}
}

func TestConcurrentDepotExpansion(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Character only has funds for exactly 1 expansion (200,000 G)
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Concurrent Expand", 250000)
	if err != nil {
		t.Fatal(err)
	}

	depotService, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt two concurrent expansions (each 200k, total 400k > 250k)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := depotService.Expand(ctx, char.ID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	successCount := 0
	for err := range errs {
		if err == nil {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful expansion, got %d", successCount)
	}

	restoredDep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID error = %v", err)
	}
	if restoredDep.ExDepot != 1 {
		t.Fatalf("expected ex_depot = 1, got %d", restoredDep.ExDepot)
	}
}
