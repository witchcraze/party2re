package database

import (
	"context"
	"database/sql"
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
	db := OpenTestDB(t)

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "FutureMem DB Test")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, char)

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
	db := OpenTestDB(t)

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "FutureMem Tx Test")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, char)

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

func TestFutureMemoryFixtureCleanup(t *testing.T) {
	observer := OpenTestDB(t)
	ctx := context.Background()

	obsRepo, err := NewFutureMemoryRepository(observer)
	if err != nil {
		t.Fatalf("NewFutureMemoryRepository observer: %v", err)
	}

	unrelated, err := CreateTestCharacter(ctx, observer, "UnrelFM")
	if err != nil {
		t.Fatalf("CreateTestCharacter unrelated: %v", err)
	}
	RegisterCharacterCleanup(t, observer, unrelated)

	now := time.Now().UTC().Truncate(time.Second)
	unrelatedMem := corecharacter.FutureMemory{
		ID:          id.New(),
		CharacterID: unrelated.ID,
		JobID:       "job-05",
		OldJobID:    "job-01",
		Level:       40,
		CreatedAt:   now,
	}
	if err := obsRepo.Save(ctx, unrelatedMem); err != nil {
		t.Fatalf("Save unrelated future memory: %v", err)
	}

	var normalChar, earlyChar corecharacter.Character
	seed := func(t *testing.T, db *sql.DB, charID string) {
		t.Helper()
		repo, err := NewFutureMemoryRepository(db)
		if err != nil {
			t.Fatalf("NewFutureMemoryRepository: %v", err)
		}
		if err := repo.Save(ctx, corecharacter.FutureMemory{
			ID:          id.New(),
			CharacterID: charID,
			JobID:       "job-05",
			OldJobID:    "job-01",
			Level:       50,
			CreatedAt:   now,
		}); err != nil {
			t.Fatalf("Save owned future memory: %v", err)
		}
	}

	t.Run("normal scope end", func(t *testing.T) {
		db := OpenTestDB(t)
		var err error
		normalChar, err = CreateTestCharacter(ctx, db, "FMNorm")
		if err != nil {
			t.Fatalf("CreateTestCharacter normal: %v", err)
		}
		RegisterCharacterCleanup(t, db, normalChar)
		seed(t, db, normalChar.ID)
	})

	t.Run("early return path", func(t *testing.T) {
		db := OpenTestDB(t)
		var err error
		earlyChar, err = CreateTestCharacter(ctx, db, "FMEarly")
		if err != nil {
			t.Fatalf("CreateTestCharacter early: %v", err)
		}
		RegisterCharacterCleanup(t, db, earlyChar)
		seed(t, db, earlyChar.ID)
		return
	})

	assertRowCount := func(query, arg string, want int) {
		t.Helper()
		assertFixtureRowCount(t, ctx, observer, query, arg, want)
	}

	for _, c := range []corecharacter.Character{normalChar, earlyChar} {
		assertRowCount("SELECT COUNT(*) FROM characters WHERE id = ?", c.ID, 0)
		assertRowCount("SELECT COUNT(*) FROM character_depots WHERE character_id = ?", c.ID, 0)
		assertRowCount("SELECT COUNT(*) FROM character_future_memories WHERE character_id = ?", c.ID, 0)
		assertRowCount("SELECT COUNT(*) FROM players WHERE id = ?", c.PlayerID, 0)
	}

	assertRowCount("SELECT COUNT(*) FROM characters WHERE id = ?", unrelated.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM character_depots WHERE character_id = ?", unrelated.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM character_future_memories WHERE id = ?", unrelatedMem.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM players WHERE id = ?", unrelated.PlayerID, 1)
}
