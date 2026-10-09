package database

import (
	"context"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

func TestFutureMemoryRepositoryNilDB(t *testing.T) {
	if _, err := NewFutureMemoryRepository(nil); err == nil {
		t.Fatal("NewFutureMemoryRepository(nil) expected error, got nil")
	}
}

func TestFutureMemoryRepositorySaveAndFind(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "FutureMem DB Test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	repo, err := NewFutureMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	mem := corecharacter.FutureMemory{
		ID:          id.New(),
		CharacterID: char.ID,
		JobID:       "job-05",
		OldJobID:    "job-01",
		Level:       50,
		Experience:  12000,
		MaxHP:       600,
		MaxMP:       300,
		Attack:      150,
		Defense:     110,
		Agility:     90,
		Gender:      "female",
		OverLevel:   true,
		CreatedAt:   now,
	}

	if err := repo.Save(ctx, mem); err != nil {
		t.Fatalf("repo.Save() error = %v", err)
	}

	memories, err := repo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("repo.FindByCharacterID() error = %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	got := memories[0]
	if got.ID != mem.ID || got.CharacterID != char.ID || got.JobID != "job-05" || got.OldJobID != "job-01" ||
		got.Level != 50 || got.Experience != 12000 || got.MaxHP != 600 || got.MaxMP != 300 ||
		got.Attack != 150 || got.Defense != 110 || got.Agility != 90 || got.Gender != "female" || !got.OverLevel {
		t.Fatalf("retrieved memory mismatch: %#v, want %#v", got, mem)
	}

	// Delete
	if err := repo.Delete(ctx, char.ID, mem.ID); err != nil {
		t.Fatalf("repo.Delete() error = %v", err)
	}
	memoriesAfter, err := repo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("repo.FindByCharacterID() after delete error = %v", err)
	}
	if len(memoriesAfter) != 0 {
		t.Fatalf("expected 0 memories after delete, got %d", len(memoriesAfter))
	}
}

func TestFutureMemoryRepositoryAmbientTransaction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "FutureMem Tx Test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID)
		_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	})

	repo, err := NewFutureMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	mem := corecharacter.FutureMemory{
		ID:          id.New(),
		CharacterID: char.ID,
		JobID:       "job-05",
		OldJobID:    "job-01",
		Level:       50,
		CreatedAt:   time.Now().UTC(),
	}

	// Rollback in transaction
	err = RunInTx(ctx, db, func(txCtx context.Context) error {
		if err := repo.Save(txCtx, mem); err != nil {
			return err
		}
		// intentional rollback
		return context.Canceled
	})
	if err == nil {
		t.Fatal("expected tx to return error, got nil")
	}

	memories, err := repo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 0 {
		t.Fatalf("expected 0 memories after rollback, got %d", len(memories))
	}
}
