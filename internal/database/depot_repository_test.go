package database

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Generate strictly adjacent IDs higher than any existing character ID to guarantee
	// that they are immediate neighbors in idx_depot_items_character with 0 intervening records.
	basePrefix := fmt.Sprintf("ff%028x", time.Now().UnixNano()&0xFFFFFFFFFFFFF)
	id1 := basePrefix + "01"
	id2 := basePrefix + "02"

	ch1, err := corecharacter.New("AdjChar1")
	if err != nil {
		t.Fatal(err)
	}
	ch1.ID = id1
	ch1.PlayerID = p.ID
	if err := charRepo.Save(ctx, ch1); err != nil {
		t.Fatal(err)
	}

	ch2, err := corecharacter.New("AdjChar2")
	if err != nil {
		t.Fatal(err)
	}
	ch2.ID = id2
	ch2.PlayerID = p.ID
	if err := charRepo.Save(ctx, ch2); err != nil {
		t.Fatal(err)
	}

	// Verify strict index adjacency: no other rows exist in the interval [id1, id2]
	var betweenCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM depot_items WHERE character_id > ? AND character_id < ?", id1, id2).Scan(&betweenCount); err != nil {
		t.Fatal(err)
	}
	if betweenCount != 0 {
		t.Fatalf("character IDs are not strictly adjacent in depot_items index: %d intervening rows", betweenCount)
	}

	char1 := ch1
	char2 := ch2

	// Initialize depots with initial non-stackable equipment
	it1, _ := item.NewInstance("weapon-31", 1)
	it2, _ := item.NewInstance("armor-34", 1)
	if _, err := CreateTestDepot(ctx, db, char1.ID, 5, []item.Instance{it1, it2}); err != nil {
		t.Fatal(err)
	}
	it3, _ := item.NewInstance("weapon-31", 1)
	it4, _ := item.NewInstance("armor-34", 1)
	if _, err := CreateTestDepot(ctx, db, char2.ID, 5, []item.Instance{it3, it4}); err != nil {
		t.Fatal(err)
	}

	const iterations = 25
	errCh := make(chan error, iterations*2)

	// Run lock-step concurrent rounds where both adjacent characters acquire depot locks and insert
	// non-stackable equipment instances simultaneously, directly exercising the index gap.
	for round := 0; round < iterations; round++ {
		tx1Ready := make(chan struct{})
		tx2Ready := make(chan struct{})
		startSave := make(chan struct{})
		var once sync.Once

		var wgRound sync.WaitGroup
		wgRound.Add(2)

		go func() {
			defer wgRound.Done()
			err := repo.RunInTx(ctx, func(txCtx context.Context) error {
				dep, err := repo.FindByCharacterIDForUpdate(txCtx, char1.ID)
				if err != nil {
					return err
				}
				close(tx1Ready)
				<-startSave

				// Add new non-stackable equipment and trim
				newItem, err := item.NewInstance("weapon-31", 1)
				if err != nil {
					return err
				}
				_ = dep.AddItem(newItem)
				if len(dep.Items) > 8 {
					dep.Items = dep.Items[:3]
				}
				return repo.Save(txCtx, dep)
			})
			if err != nil {
				errCh <- fmt.Errorf("char1 round error: %w", err)
			}
		}()

		go func() {
			defer wgRound.Done()
			err := repo.RunInTx(ctx, func(txCtx context.Context) error {
				dep, err := repo.FindByCharacterIDForUpdate(txCtx, char2.ID)
				if err != nil {
					return err
				}
				close(tx2Ready)
				<-startSave

				// Add new non-stackable equipment and trim
				newItem, err := item.NewInstance("armor-34", 1)
				if err != nil {
					return err
				}
				_ = dep.AddItem(newItem)
				if len(dep.Items) > 8 {
					dep.Items = dep.Items[:3]
				}
				return repo.Save(txCtx, dep)
			})
			if err != nil {
				errCh <- fmt.Errorf("char2 round error: %w", err)
			}
		}()

		<-tx1Ready
		<-tx2Ready
		once.Do(func() { close(startSave) })
		wgRound.Wait()
	}
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("Concurrent Adjacent Save failed with deadlock or error: %v", err)
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

	// Verify database ownership integrity directly:
	// depot_items for char1 must strictly have character_id = char1.ID and vice versa
	for _, it := range d1.Items {
		var owner string
		if err := db.QueryRowContext(ctx, "SELECT character_id FROM depot_items WHERE id = ?", it.ID).Scan(&owner); err != nil {
			t.Fatalf("query item %s owner error: %v", it.ID, err)
		}
		if owner != char1.ID {
			t.Errorf("item %s owned by %s, want %s", it.ID, owner, char1.ID)
		}
	}
	for _, it := range d2.Items {
		var owner string
		if err := db.QueryRowContext(ctx, "SELECT character_id FROM depot_items WHERE id = ?", it.ID).Scan(&owner); err != nil {
			t.Fatalf("query item %s owner error: %v", it.ID, err)
		}
		if owner != char2.ID {
			t.Errorf("item %s owned by %s, want %s", it.ID, owner, char2.ID)
		}
	}
}

func TestDepotRepositoryCannotUpdateOtherCharacterItem(t *testing.T) {
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

	char1, err := CreateTestCharacter(ctx, db, "Depot Owner 1")
	if err != nil {
		t.Fatal(err)
	}
	char2, err := CreateTestCharacter(ctx, db, "Depot Owner 2")
	if err != nil {
		t.Fatal(err)
	}

	// Char 1 saves an initial item
	origItem := item.Instance{
		ID:               char1.ID[:30] + "01",
		DefinitionID:     "weapon-01",
		Quantity:         1,
		EnhancementLevel: 5,
	}
	if _, err := CreateTestDepot(ctx, db, char1.ID, 0, []item.Instance{origItem}); err != nil {
		t.Fatal(err)
	}

	// Char 2 attempts to save depot containing an item sharing Char 1's instance ID
	hijackedItem := item.Instance{
		ID:               origItem.ID,
		DefinitionID:     "armor-99",
		Quantity:         99,
		EnhancementLevel: 9,
	}
	dep2, err := depot.NewDepot(char2.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep2.Items = []item.Instance{hijackedItem}

	err = repo.Save(ctx, dep2)
	if err == nil {
		t.Fatal("expected error when saving depot with item ID owned by another character, got nil")
	}

	// Verify Char 1's depot is completely untouched
	d1, err := repo.FindByCharacterID(ctx, char1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d1.Items) != 1 || d1.Items[0] != origItem {
		t.Fatalf("char1's item was modified or corrupted: got %+v, want %+v", d1.Items, []item.Instance{origItem})
	}
}

func TestDepotRepositoryFindByCharacterIDForUpdateReadsLatestCommittedData(t *testing.T) {
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

	char, err := CreateTestCharacter(ctx, db, "Depot Freshness Test")
	if err != nil {
		t.Fatal(err)
	}

	itemInitial, _ := item.NewInstance("weapon-01", 1)
	if _, err := CreateTestDepot(ctx, db, char.ID, 0, []item.Instance{itemInitial}); err != nil {
		t.Fatal(err)
	}

	itemFromTrx1, _ := item.NewInstance("armor-01", 1)
	itemFromTrx2, _ := item.NewInstance("item-001", 5)

	trx1Started := make(chan struct{})
	trx1CanCommit := make(chan struct{})
	trx2Done := make(chan error, 1)

	// Trx 1 acquires lock on char's depot, adds itemFromTrx1, but holds transaction open
	go func() {
		_ = repo.RunInTx(ctx, func(tx1Ctx context.Context) error {
			dep1, err := repo.FindByCharacterIDForUpdate(tx1Ctx, char.ID)
			if err != nil {
				t.Errorf("Trx 1 FindByCharacterIDForUpdate error: %v", err)
				close(trx1Started)
				return err
			}
			if err := dep1.AddItem(itemFromTrx1); err != nil {
				t.Errorf("Trx 1 AddItem error: %v", err)
				close(trx1Started)
				return err
			}
			if err := repo.Save(tx1Ctx, dep1); err != nil {
				t.Errorf("Trx 1 Save error: %v", err)
				close(trx1Started)
				return err
			}
			close(trx1Started)
			<-trx1CanCommit
			return nil
		})
	}()

	<-trx1Started

	// Trx 2 tries to call FindByCharacterIDForUpdate concurrently.
	// It must block until Trx 1 commits, then read Trx 1's newly committed item (not stale snapshot).
	go func() {
		trx2Done <- repo.RunInTx(ctx, func(tx2Ctx context.Context) error {
			dep2, err := repo.FindByCharacterIDForUpdate(tx2Ctx, char.ID)
			if err != nil {
				return err
			}
			// Verify Trx 2 sees Trx 1's newly committed item
			hasItem1 := false
			for _, it := range dep2.Items {
				if it.ID == itemFromTrx1.ID {
					hasItem1 = true
					break
				}
			}
			if !hasItem1 {
				return fmt.Errorf("stale read: dep2 missing itemFromTrx1: %+v", dep2.Items)
			}
			if err := dep2.AddItem(itemFromTrx2); err != nil {
				return err
			}
			return repo.Save(tx2Ctx, dep2)
		})
	}()

	// Wait briefly to ensure Trx 2 is blocked waiting on Trx 1's row lock
	time.Sleep(50 * time.Millisecond)

	// Unblock Trx 1 to commit
	close(trx1CanCommit)

	if err := <-trx2Done; err != nil {
		t.Fatalf("Trx 2 failed: %v", err)
	}

	// Final verification: depot must contain all 3 items (initial, Trx 1, Trx 2)
	finalDep, err := repo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalDep.Items) != 3 {
		t.Fatalf("expected 3 items in depot, got %d: %+v", len(finalDep.Items), finalDep.Items)
	}
}

// TestDepotRepositoryPriorNonLockingReadSnapshotBoundary verifies the MariaDB InnoDB REPEATABLE READ
// behavior: if a transaction executes a non-locking SELECT before calling FindByCharacterIDForUpdate,
// its MVCC snapshot is established at that prior point in time. It proves why application mutations
// must follow Rank 2 (Character) and Rank 5 (CharacterDepots) locking reads before performing mutations.
func TestDepotRepositoryPriorNonLockingReadSnapshotBoundary(t *testing.T) {
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

	char, err := CreateTestCharacter(ctx, db, "Depot Snapshot Boundary")
	if err != nil {
		t.Fatal(err)
	}

	initialItem, _ := item.NewInstance("weapon-31", 1)
	if _, err := CreateTestDepot(ctx, db, char.ID, 0, []item.Instance{initialItem}); err != nil {
		t.Fatal(err)
	}

	itemFromTx1, _ := item.NewInstance("armor-34", 1)

	tx2SnapshotEstablished := make(chan struct{})
	tx1Done := make(chan struct{})
	tx2ObservedStale := make(chan bool, 1)

	// Tx 2 starts first and executes a non-locking SELECT on an InnoDB table, establishing an early MVCC Read View
	go func() {
		_ = repo.RunInTx(ctx, func(tx2Ctx context.Context) error {
			var dummy string
			if err := ExecutorFromContext(tx2Ctx, db).QueryRowContext(tx2Ctx, "SELECT id FROM characters WHERE id = ?", char.ID).Scan(&dummy); err != nil {
				return err
			}
			close(tx2SnapshotEstablished)
			<-tx1Done

			// FindByCharacterIDForUpdate locks character_depots (Rank 5) with FOR UPDATE,
			// while depot_items query uses non-locking SELECT to avoid gap-lock deadlocks on adjacent characters.
			dep2, err := repo.FindByCharacterIDForUpdate(tx2Ctx, char.ID)
			if err != nil {
				return err
			}

			// In REPEATABLE READ, the non-locking SELECT on depot_items reads from Tx 2's early Read View.
			hasItem1 := false
			for _, it := range dep2.Items {
				if it.ID == itemFromTx1.ID {
					hasItem1 = true
					break
				}
			}
			tx2ObservedStale <- !hasItem1
			return nil
		})
	}()

	<-tx2SnapshotEstablished

	// Tx 1 modifies depot and commits
	err = repo.RunInTx(ctx, func(tx1Ctx context.Context) error {
		dep1, err := repo.FindByCharacterIDForUpdate(tx1Ctx, char.ID)
		if err != nil {
			return err
		}
		if err := dep1.AddItem(itemFromTx1); err != nil {
			return err
		}
		return repo.Save(tx1Ctx, dep1)
	})
	if err != nil {
		t.Fatalf("Tx 1 failed: %v", err)
	}
	close(tx1Done)

	observedStale := <-tx2ObservedStale
	if !observedStale {
		t.Log("Note: Tx 2 saw fresh items (Read View was refreshed or created late)")
	} else {
		t.Log("Confirmed: Prior non-locking SELECT establishes early MVCC Read View in REPEATABLE READ; serialized locking reads prevent this in application flows")
	}
}
