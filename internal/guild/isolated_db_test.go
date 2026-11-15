package guild_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
)

func guildIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	return database.CreateTestDatabase(t, []string{"players", "characters", "character_depots", "guilds", "guild_members", "guild_decorations", "character_memories"}...)
}

func TestGuildIsolatedDB_DisbandPreservesSharedGuild(t *testing.T) {
	db := guildIsolatedDB(t)
	shared := database.OpenTestDB(t)
	ctx := context.Background()
	sharedGuild, _, err := database.CreateTestGuildWithLeader(ctx, shared, "IsolationSentinel", 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Only remove this test's guild; its character/player use ordinary shared fixtures.
		if _, err := shared.ExecContext(ctx, "DELETE FROM guilds WHERE id = ?", sharedGuild.ID); err != nil {
			t.Errorf("failed to remove sentinel guild: %v", err)
		}
	})
	localGuild, _, err := database.CreateTestGuildWithLeader(ctx, db, "IsolatedInactive", 10000)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-guild.InactivityDisbandDuration - 24*time.Hour)
	if _, err := shared.ExecContext(ctx, "UPDATE guilds SET last_active_at = ? WHERE id = ?", old, sharedGuild.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET last_active_at = ? WHERE id = ?", old, localGuild.ID); err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := guild.NewService(repo, guild.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	count, disbanded, err := svc.DisbandInactiveGuilds(ctx, now, 100)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(disbanded) != 1 || disbanded[0] != localGuild.ID {
		t.Fatalf("expected only local guild to disband, got count=%d, guilds=%v", count, disbanded)
	}
	var members int
	if err := shared.QueryRowContext(ctx, "SELECT COUNT(*) FROM guilds g JOIN guild_members m ON m.guild_id = g.id WHERE g.id = ?", sharedGuild.ID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 1 {
		t.Fatalf("shared inactive guild and its member must survive, got %d members", members)
	}
}
