package database_test

import (
	"context"
	"errors"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

// Runs on the shared DB: every assertion is scoped to characters this test created.
func TestCharacterMemoryRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := database.NewCharacterMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	g, leader, err := database.CreateTestGuildWithLeader(ctx, db, "Mem"+id.New()[:8], 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendMemory(ctx, leader.ID, corecharacter.MemoryGuildFound, "first"); err != nil {
		t.Fatal(err)
	}
	// The snapshot is taken at write time: later changes must not rewrite it.
	renamed := leader
	renamed.Level, renamed.SP = leader.Level+4, leader.SP+7
	if err := characters.Update(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendMemory(ctx, leader.ID, corecharacter.MemoryJobMaster, "second"); err != nil {
		t.Fatal(err)
	}
	// A rolled-back operation leaves no entry.
	rollback := errors.New("rollback")
	if err := database.RunInTx(ctx, db, func(txCtx context.Context) error {
		if err := repo.AppendMemory(txCtx, leader.ID, corecharacter.MemoryWish, "rolled back"); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}

	got, err := repo.ListMemories(ctx, leader.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Content != "second" || got[1].Content != "first" {
		t.Fatalf("want [second first] newest first, got %+v", got)
	}
	first, second := got[1], got[0]
	if first.Kind != corecharacter.MemoryGuildFound || first.Name != leader.Name || first.GuildName != g.Name ||
		first.Color != leader.Color || first.Level != leader.Level || first.JobID != leader.JobID ||
		first.Gender != leader.Gender || first.SP != leader.SP || first.CreatedAt.IsZero() {
		t.Fatalf("first entry does not snapshot the character at write time: %+v (character %+v, guild %q)", first, leader, g.Name)
	}
	if second.Level != renamed.Level || second.SP != renamed.SP {
		t.Fatalf("second entry = %+v, want level %d sp %d", second, renamed.Level, renamed.SP)
	}
	if limited, err := repo.ListMemories(ctx, leader.ID, 1); err != nil || len(limited) != 1 || limited[0].Content != "second" {
		t.Fatalf("limit 1 = %+v, %v, want only the newest", limited, err)
	}

	if err := repo.AppendMemory(ctx, "no-such-character", corecharacter.MemoryBirth, "x"); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("unknown character err = %v, want ErrNotFound", err)
	}

	// Without a guild the snapshot has no guild name, and deletion removes the log.
	loner, err := database.CreateTestCharacter(ctx, db, "MemLoner")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendMemory(ctx, loner.ID, corecharacter.MemoryBirth, "born"); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ListMemories(ctx, loner.ID, 10); err != nil || len(got) != 1 || got[0].GuildName != "" {
		t.Fatalf("loner memories = %+v, %v", got, err)
	}
	if err := characters.Delete(ctx, loner.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ListMemories(ctx, loner.ID, 10); err != nil || len(got) != 0 {
		t.Fatalf("memories after deletion = %+v, %v", got, err)
	}
}
