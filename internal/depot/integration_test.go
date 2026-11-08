package depot_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
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

	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	depotService, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo, depot.WithItemDefinitionProvider(catalog))
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

	// 4. Withdraw Item (stack of 3 -> 1 unit in hand with new ID, 2 remaining in depot)
	dep, err = depotService.WithdrawItem(ctx, createdChar.ID, potion.ID)
	if err != nil {
		t.Fatalf("WithdrawItem error = %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].ID != potion.ID || dep.Items[0].Quantity != 2 {
		t.Fatalf("depot items after first withdraw = %#v, want 1 row with ID=%s Quantity=2", dep.Items, potion.ID)
	}
	restoredInv, err := invRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID inv error = %v", err)
	}
	if len(restoredInv.Items) != 1 || restoredInv.Items[0].DefinitionID != "item-001" || restoredInv.Items[0].Quantity != 1 || restoredInv.Items[0].ID == potion.ID {
		t.Fatalf("inventory after first withdraw = %#v, want 1 unit with new ID", restoredInv.Items)
	}

	// 5. Redeposit the withdrawn unit (merges back into the depot stack of 3)
	dep, err = depotService.DepositItem(ctx, createdChar.ID, restoredInv.Items[0].ID)
	if err != nil {
		t.Fatalf("DepositItem redeposit error = %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].ID != potion.ID || dep.Items[0].Quantity != 3 {
		t.Fatalf("depot items after redeposit = %#v, want 1 row with ID=%s Quantity=3", dep.Items, potion.ID)
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
	if restoredDep.ExDepot != 1 || restoredDep.Capacity != 10 || len(restoredDep.Items) != 1 || restoredDep.Items[0].Quantity != 3 {
		t.Errorf("restored depot mismatch: %#v", restoredDep)
	}
}

func TestDepotStackWithdrawRollbackAndConcurrent_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
	colRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	inductor := &depotLegendInductor{err: errors.New("stack withdraw rollback")}
	colSvc, err := collection.NewService(colRepo, 180, 1, collection.WithLegendInductor(inductor))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, txProvider,
		depot.WithItemDefinitionProvider(catalog),
		depot.WithCollectionRecorder(colSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "StackWithdraw", 10000)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := item.NewInstance("item-001", 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, char.ID, 0, []item.Instance{stack}); err != nil {
		t.Fatal(err)
	}

	// 1. Rollback on N>=2 stack withdrawal preserves all 3 units in depot and 0 in hand
	if _, err := svc.WithdrawItem(ctx, char.ID, stack.ID); err == nil {
		t.Fatal("expected WithdrawItem to fail on inductor error")
	}
	depAfterFail, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(depAfterFail.Items) != 1 || depAfterFail.Items[0].ID != stack.ID || depAfterFail.Items[0].Quantity != 3 {
		t.Fatalf("depot after rollback = %#v, err = %v", depAfterFail.Items, err)
	}
	invAfterFail, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(invAfterFail.Items) != 0 {
		t.Fatalf("inventory after rollback = %#v, err = %v", invAfterFail.Items, err)
	}

	// 2. Concurrent withdrawals of the same stack: each later swap returns the
	// held unit to the stack, so every call succeeds and units are conserved.
	inductor.err = nil
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		_, err := svc.WithdrawItem(ctx, char.ID, stack.ID)
		return err
	})
	if res.Failures != 0 || res.Deadlocks != 0 || res.Successes != res.TotalOps {
		t.Fatalf("stress results=%+v", res)
	}

	depFinal, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(depFinal.Items) != 1 || depFinal.Items[0].ID != stack.ID || depFinal.Items[0].Quantity != 2 {
		t.Fatalf("depot after concurrent withdraw = %#v, err = %v", depFinal.Items, err)
	}
	invFinal, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(invFinal.Items) != 1 || invFinal.Items[0].DefinitionID != "item-001" || invFinal.Items[0].Quantity != 1 || invFinal.Items[0].ID == stack.ID {
		t.Fatalf("inventory after concurrent withdraw = %#v, err = %v", invFinal.Items, err)
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

type depotLegendInductor struct {
	err   error
	calls []string
}

func (l *depotLegendInductor) RecordLegend(_ context.Context, category, characterID string) error {
	if l.err != nil {
		return l.err
	}
	l.calls = append(l.calls, category+":"+characterID)
	return nil
}

func TestDepotIntegration_MandatoryInductionRollback(t *testing.T) {
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
	colRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	inductor := &depotLegendInductor{err: errors.New("mandatory depot legend failure")}
	colSvc, err := collection.NewService(colRepo, 180, 141,
		collection.WithTotalWeapons(1),
		collection.WithLegendInductor(inductor),
	)
	if err != nil {
		t.Fatal(err)
	}

	depotService, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, txProvider,
		depot.WithItemDefinitionProvider(catalog),
		depot.WithCollectionRecorder(colSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "DepotRollbackTester", 50000)
	if err != nil {
		t.Fatal(err)
	}

	// Setup depot with weapon-01
	weaponInst, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	initialDepot, err := database.CreateTestDepot(ctx, db, char.ID, 0, []item.Instance{weaponInst})
	if err != nil {
		t.Fatal(err)
	}
	if len(initialDepot.Items) != 1 {
		t.Fatalf("expected 1 item in depot, got %d", len(initialDepot.Items))
	}

	// 1. Initial withdrawal fails due to legend induction failure
	_, err = depotService.WithdrawItem(ctx, char.ID, weaponInst.ID)
	if err == nil {
		t.Fatal("expected WithdrawItem to fail when collection legend induction fails")
	}

	// 2. Assert full database rollback: depot still has weapon, inventory has 0 items, collection 0 items, milestone uncompleted
	dep, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(dep.Items) != 1 {
		t.Errorf("depot item count = %d; want 1 (not withdrawn, rolled back)", len(dep.Items))
	}
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(inv.Items) != 0 {
		t.Errorf("inventory item count = %d; want 0 (not added, rolled back)", len(inv.Items))
	}
	colEntries, err := colRepo.GetItemCollection(ctx, char.ID, "weapon")
	if err != nil || len(colEntries) != 0 {
		t.Errorf("collection weapon count = %d; want 0", len(colEntries))
	}
	isCompleted, err := colRepo.IsCompleted(ctx, char.ID, "weapon")
	if err != nil || isCompleted {
		t.Errorf("weapon milestone completed = %v; want false", isCompleted)
	}

	// 3. Resolve failure and retry withdrawal
	inductor.err = nil
	_, err = depotService.WithdrawItem(ctx, char.ID, weaponInst.ID)
	if err != nil {
		t.Fatalf("expected WithdrawItem to succeed after resolving inductor failure: %v", err)
	}

	// 4. Assert DB state after successful retry
	dep, err = depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(dep.Items) != 0 {
		t.Errorf("depot item count = %d; want 0", len(dep.Items))
	}
	inv, err = invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(inv.Items) != 1 {
		t.Errorf("inventory item count = %d; want 1", len(inv.Items))
	}
	colEntries, err = colRepo.GetItemCollection(ctx, char.ID, "weapon")
	if err != nil || len(colEntries) != 1 {
		t.Errorf("collection weapon count = %d; want 1", len(colEntries))
	}
	isCompleted, err = colRepo.IsCompleted(ctx, char.ID, "weapon")
	if err != nil || !isCompleted {
		t.Errorf("weapon milestone completed = %v; want true", isCompleted)
	}
	if len(inductor.calls) != 1 {
		t.Errorf("inductor calls = %d; want 1", len(inductor.calls))
	}
}
