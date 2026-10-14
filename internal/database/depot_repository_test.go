package database

import (
	"context"
	"os"
	"reflect"
	"slices"
	"sync"
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

func TestDepotRepositoryPreservesItemOrder(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	actor, err := CreateTestCharacter(ctx, db, "Depot order")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	items := []item.Instance{
		{ID: actor.ID[:30] + "ff", DefinitionID: "weapon-01", Quantity: 1, EnhancementLevel: 6},
		{ID: actor.ID[:30] + "bb", DefinitionID: "armor-02", Quantity: 1},
		{ID: actor.ID[:30] + "aa", DefinitionID: "item-001", Quantity: 8},
	}
	dep, err := CreateTestDepot(ctx, db, actor.ID, 0, items)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder := func(want []item.Instance) {
		t.Helper()
		got, err := repo.FindByCharacterID(ctx, actor.ID)
		if err != nil || !reflect.DeepEqual(got.Items, want) {
			t.Fatalf("public reload: %+v, error=%v, want=%+v", got.Items, err, want)
		}
		if err := repo.RunInTx(ctx, func(txCtx context.Context) error {
			got, err := repo.FindByCharacterIDForUpdate(txCtx, actor.ID)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(got.Items, want) {
				t.Errorf("locked reload: %+v, want=%+v", got.Items, want)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertOrder(items)
	slices.Reverse(dep.Items)
	if err := repo.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}
	assertOrder(dep.Items)
	before := slices.Clone(dep.Items)
	dep.Items = append(dep.Items, dep.Items[0])
	if err := repo.Save(ctx, dep); err == nil {
		t.Fatal("duplicate instance must fail")
	}
	assertOrder(before)
	// Rows created before the migration share position zero and retain ID ordering.
	if _, err := db.ExecContext(ctx, "UPDATE depot_items SET sort_position = 0 WHERE character_id = ?", actor.ID); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(before, func(a, b item.Instance) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	assertOrder(before)
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

	inst, err := item.NewInstance("item-001", 5)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CreateTestDepot(ctx, db, char.ID, 2, []item.Instance{inst})
	if err != nil {
		t.Fatalf("CreateTestDepot error = %v", err)
	}

	restored, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID() error = %v", err)
	}

	if restored.CharacterID != char.ID || restored.ExDepot != 2 || len(restored.Items) != 1 || restored.Items[0].Quantity != 5 {
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
	char.Money = 500000
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

	// 1. Deposit item
	dep, err := service.DepositItem(ctx, char.ID, potion.ID)
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

	// 2. Expand depot
	dep, err = service.Expand(ctx, char.ID)
	if err != nil {
		t.Fatalf("Expand error: %v", err)
	}
	if dep.ExDepot != 1 {
		t.Errorf("depot ex_depot = %d, want 1", dep.ExDepot)
	}

	// 3. Withdraw item
	dep, err = service.WithdrawItem(ctx, char.ID, potion.ID)
	if err != nil {
		t.Fatalf("WithdrawItem error: %v", err)
	}
	if len(dep.Items) != 0 {
		t.Errorf("depot items count = %d, want 0", len(dep.Items))
	}
}

func TestDepotRepositoryConcurrentAdjacentSaveNoDeadlock(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char1, err := CreateTestCharacter(ctx, db, "Depot Adjacent 1")
	if err != nil {
		t.Fatal(err)
	}
	char2, err := CreateTestCharacter(ctx, db, "Depot Adjacent 2")
	if err != nil {
		t.Fatal(err)
	}

	// Initialize depots with initial items
	it1, _ := item.NewInstance("item-001", 10)
	it2, _ := item.NewInstance("weapon-01", 1)
	if _, err := CreateTestDepot(ctx, db, char1.ID, 5, []item.Instance{it1, it2}); err != nil {
		t.Fatal(err)
	}
	it3, _ := item.NewInstance("armor-01", 1)
	it4, _ := item.NewInstance("item-002", 5)
	if _, err := CreateTestDepot(ctx, db, char2.ID, 5, []item.Instance{it3, it4}); err != nil {
		t.Fatal(err)
	}

	const iterations = 20
	errCh := make(chan error, iterations*2)

	runWorker := func(charID string) {
		for i := 0; i < iterations; i++ {
			err := repo.RunInTx(ctx, func(txCtx context.Context) error {
				dep, err := repo.FindByCharacterIDForUpdate(txCtx, charID)
				if err != nil {
					return err
				}
				// Alternately add new items and trim back to avoid capacity overflow
				newItem, err := item.NewInstance("item-001", 1)
				if err != nil {
					return err
				}
				_ = dep.AddItem(newItem)
				if len(dep.Items) > 10 {
					dep.Items = dep.Items[:5]
				}
				return repo.Save(txCtx, dep)
			})
			if err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		runWorker(char1.ID)
	}()
	go func() {
		defer wg.Done()
		runWorker(char2.ID)
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("Concurrent Save failed: %v", err)
		}
	}

	// Verify both depots are readable and intact
	d1, err := repo.FindByCharacterID(ctx, char1.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID char1 error: %v", err)
	}
	if len(d1.Items) == 0 {
		t.Errorf("char1 depot should have items")
	}

	d2, err := repo.FindByCharacterID(ctx, char2.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID char2 error: %v", err)
	}
	if len(d2.Items) == 0 {
		t.Errorf("char2 depot should have items")
	}
}
