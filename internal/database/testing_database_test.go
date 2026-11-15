package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestCreateTestDatabase(t *testing.T) {
	source := CreateTestDatabase(t, "players", "characters")
	ctx := context.Background()
	player, err := CreateTestPlayer(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	var sourceName string
	if err := source.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&sourceName); err != nil {
		t.Fatal(err)
	}
	// Use owned source data so exact counts cannot race with other test packages.
	// Source credentials need not have access to the newly created database.
	adminCfg, err := mysql.ParseDSN(os.Getenv("PARTY2_TEST_DB_ADMIN_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	adminCfg.DBName = sourceName
	t.Setenv("PARTY2_DB_DSN", adminCfg.FormatDSN())
	var name string
	t.Run("schema only and foreign keys", func(t *testing.T) {
		// Reverse dependency order to check that schema copy is order-independent.
		db := CreateTestDatabase(t, "characters", "players")
		if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
			t.Fatal(err)
		}
		if name == sourceName || !strings.HasPrefix(name, "party2_test_") {
			t.Fatalf("dedicated database = %q, source = %q", name, sourceName)
		}
		var tables, players, fkChecks int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM players").Scan(&players); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT @@FOREIGN_KEY_CHECKS").Scan(&fkChecks); err != nil {
			t.Fatal(err)
		}
		if tables != 2 || players != 0 || fkChecks != 1 {
			t.Fatalf("tables=%d players=%d fkChecks=%d; want 2, 0, 1", tables, players, fkChecks)
		}
		var table, ddl string
		if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE characters").Scan(&table, &ddl); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ddl, "FOREIGN KEY (`player_id`) REFERENCES `players`") {
			t.Fatal("character owner foreign key was not copied")
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players"); err != nil {
			t.Fatal(err)
		}
	})
	var remaining int
	if err := source.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", name).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("database %s was not dropped", name)
	}
	if err := source.QueryRowContext(ctx, "SELECT COUNT(*) FROM players WHERE id = ?", player.ID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("source player changed: count=%d error=%v", remaining, err)
	}
	t.Run("empty table list", func(t *testing.T) {
		db := CreateTestDatabase(t)
		var tables int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables); err != nil || tables != 0 {
			t.Fatalf("empty table list copied %d tables: %v", tables, err)
		}
	})
}

func TestCreateTestDatabaseSkipsWithoutDSN(t *testing.T) {
	t.Setenv("PARTY2_DB_DSN", "")
	returned := false
	t.Run("disabled", func(t *testing.T) {
		CreateTestDatabase(t, "players")
		returned = true
	})
	if returned {
		t.Fatal("DB-disabled fixture did not skip")
	}
}
