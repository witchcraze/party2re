package database_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestBossRepository_ItemDeliveryAndDepotFallback(t *testing.T) {
	db := database.OpenTestDB(t)

	ctx := context.Background()
	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		t.Fatal(err)
	}
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

	char, err := database.CreateTestCharacter(ctx, db, "Boss Overflow Hero")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Deliver first item when inventory is empty -> should land in inventory and persist character updates
	char.HeroCount++
	char.Money += 1500
	item1 := &coreitem.Instance{DefinitionID: "potion", Quantity: 1}
	if err := bossRepo.SettleMember(ctx, char, item1); err != nil {
		t.Fatalf("SettleMember item1 failed: %v", err)
	}

	savedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if savedChar.HeroCount != char.HeroCount || savedChar.Money != char.Money {
		t.Fatalf("SettleMember did not persist character updates: got HeroCount=%d Money=%d, want %d/%d",
			savedChar.HeroCount, savedChar.Money, char.HeroCount, char.Money)
	}

	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "potion" {
		t.Fatalf("expected 1 item in inventory, got: %#v", inv.Items)
	}

	// 2. Deliver second item when inventory is full -> should overflow to depot
	item2 := &coreitem.Instance{DefinitionID: "ether", Quantity: 1}
	if err := bossRepo.SettleMember(ctx, char, item2); err != nil {
		t.Fatalf("SettleMember item2 overflow failed: %v", err)
	}

	invAfter2, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID after 2 failed: %v", err)
	}
	if len(invAfter2.Items) != 1 {
		t.Fatalf("inventory must remain at capacity 1, got %d items", len(invAfter2.Items))
	}

	var dep depot.Depot
	dep, err = depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot failed: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "ether" {
		t.Fatalf("expected 1 overflow item in depot, got: %#v", dep.Items)
	}

	// 3. Fill depot to capacity, then deliver third item -> should be treated as lost drop without error
	for i := len(dep.Items); i < dep.Capacity; i++ {
		fillItem, _ := coreitem.NewInstance(fmt.Sprintf("filler_%d", i), 1)
		_ = dep.AddItem(fillItem, false)
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("failed to fill depot: %v", err)
	}

	item3 := &coreitem.Instance{DefinitionID: "elixir", Quantity: 1}
	if err := bossRepo.SettleMember(ctx, char, item3); err != nil {
		t.Fatalf("SettleMember with full inventory and depot should succeed as lost drop: %v", err)
	}

	depAfter3, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot after 3 failed: %v", err)
	}
	if len(depAfter3.Items) != dep.Capacity {
		t.Fatalf("depot item count changed, expected %d, got %d", dep.Capacity, len(depAfter3.Items))
	}
}

func TestBossRepository_UnsealClock(t *testing.T) {
	db := database.CreateTestDatabase(t, "boss_unseal_clock")
	repo, err := database.NewBossRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 1. Before any unseal -> nil, nil
	initial, err := repo.GetUnsealedAt(ctx)
	if err != nil {
		t.Fatalf("GetUnsealedAt initial: %v", err)
	}
	if initial != nil {
		t.Fatalf("expected nil unsealed_at before first unseal, got %v", *initial)
	}

	// 2. First unseal inserts row id = 1
	t1 := time.Date(2026, 11, 15, 10, 0, 0, 123456000, time.UTC)
	if err := repo.SaveUnsealedAt(ctx, t1); err != nil {
		t.Fatalf("SaveUnsealedAt t1: %v", err)
	}
	got1, err := repo.GetUnsealedAt(ctx)
	if err != nil || got1 == nil || !got1.Equal(t1) {
		t.Fatalf("GetUnsealedAt after t1 = %v (err=%v), want %v", got1, err, t1)
	}

	// 3. Re-unseal overwrites the single row
	t2 := t1.Add(45 * time.Minute)
	if err := repo.SaveUnsealedAt(ctx, t2); err != nil {
		t.Fatalf("SaveUnsealedAt t2: %v", err)
	}
	got2, err := repo.GetUnsealedAt(ctx)
	if err != nil || got2 == nil || !got2.Equal(t2) {
		t.Fatalf("GetUnsealedAt after t2 = %v (err=%v), want %v", got2, err, t2)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM boss_unseal_clock`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected singleton row count 1, got %d (err=%v)", count, err)
	}

	// 4. Rolled-back transaction preserves previous timestamp
	t3 := t2.Add(10 * time.Minute)
	rollbackErr := errors.New("rollback sentinel")
	err = database.RunInTx(ctx, db, func(txCtx context.Context) error {
		if err := repo.SaveUnsealedAt(txCtx, t3); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected rollback sentinel, got %v", err)
	}
	gotAfterRollback, err := repo.GetUnsealedAt(ctx)
	if err != nil || gotAfterRollback == nil || !gotAfterRollback.Equal(t2) {
		t.Fatalf("rollback modified unsealed_at: got %v (err=%v), want %v", gotAfterRollback, err, t2)
	}
}
