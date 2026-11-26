package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
)

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
