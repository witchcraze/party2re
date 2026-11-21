package database_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
)

func TestRemoveNonLegacyRankingsMigration(t *testing.T) {
	db := database.CreateTestDatabase(t, "players", "characters", "adventures", "ranking_snapshots", "schema_migrations")
	ctx := context.Background()

	// Recreate the pre-139 indexes if absent so the migration's DROP INDEX path is exercised too.
	for _, stmt := range []string{
		"CREATE INDEX idx_characters_level_exp ON characters (level DESC, experience DESC, id ASC)",
		"CREATE INDEX idx_characters_money ON characters (money DESC, id ASC)",
		"CREATE INDEX idx_adventures_char_outcome ON adventures (character_id, outcome)",
	} {
		// Ignore duplicate-index error if the source DB did not yet apply 139.
		_, _ = db.ExecContext(ctx, stmt) //lint:ignore error-swallow test setup ensures pre-139 index state
	}

	now := time.Now().UTC()
	for _, rt := range []string{
		"level",
		"player_wealth",
		"character_wealth",
		"boss_defeat",
		"adventure_victory",
		"job_mastery",
		"small_medals",
		"monster_kills",
	} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO ranking_snapshots (ranking_type, snapshot_data, total_count, calculated_at, updated_at) VALUES (?, '[]', 0, ?, ?)`,
			rt, now, now,
		); err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "139_remove_non_legacy_rankings.sql"))
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		for _, stmt := range strings.Split(string(data), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatalf("exec %q: %v", strings.TrimSpace(stmt), err)
				}
			}
		}
	}

	// Apply twice to verify idempotency.
	apply()
	apply()

	var staleCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ranking_snapshots
		WHERE ranking_type IN ('level','player_wealth','character_wealth','boss_defeat','adventure_victory','job_mastery','small_medals')
	`).Scan(&staleCount); err != nil || staleCount != 0 {
		t.Fatalf("stale snapshot count = %d, err = %v", staleCount, err)
	}

	var keptCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ranking_snapshots WHERE ranking_type = 'monster_kills'`).Scan(&keptCount); err != nil || keptCount != 1 {
		t.Fatalf("kept snapshot count = %d, err = %v", keptCount, err)
	}

	var idxCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND ((table_name = 'characters' AND index_name IN ('idx_characters_level_exp', 'idx_characters_money'))
		    OR (table_name = 'adventures' AND index_name = 'idx_adventures_char_outcome'))
	`).Scan(&idxCount); err != nil || idxCount != 0 {
		t.Fatalf("removed index count = %d, err = %v", idxCount, err)
	}

	var migCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = '139_remove_non_legacy_rankings'`).Scan(&migCount); err != nil || migCount != 1 {
		t.Fatalf("migration record count = %d, err = %v", migCount, err)
	}
}
