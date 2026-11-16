package database_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
)

func TestRemoveAchievementsMigrationDropsPopulatedTables(t *testing.T) {
	db := database.CreateTestDatabase(t, "players", "characters", "character_depots", "schema_migrations")
	ctx := context.Background()
	apply := func(name string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "migrations", name+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(data), ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err := db.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	apply("050_achievements_and_medals")
	char, err := database.CreateTestCharacter(ctx, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET small_medals=7 WHERE id=?", char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO character_achievements (character_id,achievement_id,current_progress,is_completed,is_claimed) VALUES (?, 'adventure_novice', 1, TRUE, TRUE)", char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO character_medals (character_id,medal_id,medal_name,category) VALUES (?, 'medal_adv_bronze', 'Bronze', 'adventure')", char.ID); err != nil {
		t.Fatal(err)
	}
	apply("135_drop_achievements_and_medals")
	apply("135_drop_achievements_and_medals")
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('character_achievements','character_medals')").Scan(&count); err != nil || count != 0 {
		t.Fatalf("removed tables = %d, error = %v", count, err)
	}
	var medals int
	if err := db.QueryRowContext(ctx, "SELECT small_medals FROM characters WHERE id=?", char.ID).Scan(&medals); err != nil || medals != 7 {
		t.Fatalf("small medals = %d, error = %v", medals, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version IN ('050_achievements_and_medals','135_drop_achievements_and_medals')").Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration records = %d, error = %v", count, err)
	}
}
