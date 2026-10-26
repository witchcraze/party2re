package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

func TestDungeonRepository_RunQualifiedCleanup(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	repo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "SQL cleanup", 1000)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := chars.Delete(ctx, char.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", char.PlayerID); err != nil {
			t.Error(err)
		}
	})
	old := dungeon.ActiveExpedition{ID: id.New(), CharacterID: char.ID, DungeonID: "dungeon-01", Status: dungeon.StatusExploring, CurrentFloor: 1, CurrentHP: char.Stats.HP, TurnsRemaining: 20, StartedAt: time.Now().UTC().Truncate(time.Second)}
	if err := repo.SaveActiveExpedition(ctx, old); err != nil {
		t.Fatal(err)
	}
	next := old
	next.ID, next.DungeonID = id.New(), "dungeon-02"
	next.StartedAt = next.StartedAt.Add(time.Hour)
	next.PosX, next.TurnsRemaining, next.AccumulatedGold = 2, 17, 77
	if err := repo.SaveActiveExpedition(ctx, next); err != nil {
		t.Fatal(err)
	}
	assertReplacement := func() {
		t.Helper()
		got, err := repo.GetActiveExpedition(ctx, char.ID)
		if err != nil || got == nil || got.ID != next.ID || got.DungeonID != next.DungeonID || !got.StartedAt.Equal(next.StartedAt) || got.PosX != 2 || got.TurnsRemaining != 17 || got.AccumulatedGold != 77 {
			t.Fatalf("SQL replacement identity/state changed: %+v %v", got, err)
		}
	}
	assertReplacement()
	for _, identity := range [][2]string{{char.ID, old.ID}, {id.New(), next.ID}} {
		if err := repo.DeleteActiveExpedition(ctx, identity[0], identity[1]); err != nil {
			t.Fatal(err)
		}
		assertReplacement()
	}
	record, err := repo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	history := dungeon.DungeonExpeditionHistory{ID: old.ID, CharacterID: char.ID, DungeonID: old.DungeonID, Outcome: dungeon.StatusEscaped, GoldReward: 100, CreatedAt: time.Now().UTC()}
	for i := 0; i < 2; i++ {
		err := repo.FinalizeExpedition(ctx, history, record, &char, nil)
		if (i == 0 && err != nil) || (i == 1 && !errors.Is(err, dungeon.ErrExpeditionAlreadyFinished)) {
			t.Fatalf("finalization %d: %v", i, err)
		}
		assertReplacement()
	}
	for i := 0; i < 2; i++ {
		if err := repo.DeleteActiveExpedition(ctx, char.ID, next.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := repo.GetActiveExpedition(ctx, char.ID); err != nil || got != nil {
		t.Fatalf("matching SQL cleanup retained run: %+v %v", got, err)
	}
	after, err := chars.FindByID(ctx, char.ID)
	if err != nil || after.Money != 1100 {
		t.Fatalf("old run payout must occur once: %+v %v", after, err)
	}
}
