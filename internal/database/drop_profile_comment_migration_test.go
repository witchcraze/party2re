package database_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
)

func TestDropCharacterProfileCommentMigration(t *testing.T) {
	db := database.CreateTestDatabase(t, "players", "characters", "character_depots", "character_profiles", "schema_migrations")
	ctx := context.Background()

	// Ensure pre-140 comment column exists even when the cloned schema already applied 140.
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE character_profiles
		ADD COLUMN IF NOT EXISTS comment VARCHAR(160) NOT NULL DEFAULT '' AFTER character_id
	`); err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "ProfMig")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO character_profiles (character_id, comment, avatar_url, aura_effect, bio_data)
		VALUES (?, 'legacy comment', 'https://i.imgur.com/abc12.gif', 5, '{"hobby":"Fishing"}')
	`, char.ID); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "140_drop_character_profile_comment.sql"))
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

	var colCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'character_profiles' AND column_name = 'comment'
	`).Scan(&colCount); err != nil || colCount != 0 {
		t.Fatalf("comment column count = %d, err = %v", colCount, err)
	}

	var avatar, bio string
	var aura int
	if err := db.QueryRowContext(ctx, `
		SELECT avatar_url, aura_effect, bio_data FROM character_profiles WHERE character_id = ?
	`, char.ID).Scan(&avatar, &aura, &bio); err != nil {
		t.Fatalf("select preserved profile failed: %v", err)
	}
	if avatar != "https://i.imgur.com/abc12.gif" || aura != 5 || !strings.Contains(bio, "Fishing") {
		t.Fatalf("preserved profile mismatch: avatar=%q aura=%d bio=%q", avatar, aura, bio)
	}

	var migCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM schema_migrations WHERE version = '140_drop_character_profile_comment'
	`).Scan(&migCount); err != nil || migCount != 1 {
		t.Fatalf("migration record count = %d, err = %v", migCount, err)
	}
}
