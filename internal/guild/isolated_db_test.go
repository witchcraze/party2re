package guild_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
)

func guildIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated guild tests")
	}

	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("failed to close source DB: %v", err)
		}
	})
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("failed to close admin DB: %v", err)
		}
	})

	name := "party2_guild_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("failed to drop isolated database %s: %v", name, err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close isolated DB: %v", err)
		}
	})

	for _, table := range []string{"players", "characters", "character_depots", "guilds", "guild_members", "character_memories"} {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestGuildIsolatedDB_DisbandPreservesSharedGuild(t *testing.T) {
	db := guildIsolatedDB(t)
	shared, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := shared.Close(); err != nil {
			t.Errorf("failed to close shared DB: %v", err)
		}
	})
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
