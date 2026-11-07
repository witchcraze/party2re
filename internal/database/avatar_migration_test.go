package database_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/database"
)

func TestAvatarImgurOnlyMigrationClearsOnlyNonCanonical(t *testing.T) {
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "guilds", "guild_members", "character_profiles", "character_presence_snapshots", "schema_migrations")
	ctx := context.Background()
	values := map[string]string{
		"https://i.imgur.com/abc12.gif":      "https://i.imgur.com/abc12.gif",
		"https://i.imgur.com/abcd123456.gif": "https://i.imgur.com/abcd123456.gif",
		"":                                   "",
		"HTTPS://I.IMGUR.COM/abc12.gif":      "",
		"https://i.imgur.com/abcd.gif":       "",
		"https://example.com/a.png":          "",
		"data:image/png;base64,iVBORw0KGgo=": "",
		"chr/052.gif":                        "chr/052.gif", // God wish appearance, fixed separately
	}
	ids := map[string]string{}
	for in := range values {
		c, err := database.CreateTestCharacter(ctx, db, "AvMig")
		if err != nil {
			t.Fatal(err)
		}
		ids[in] = c.ID
		if _, err := db.Exec(`INSERT INTO character_profiles (character_id, comment, avatar_url, aura_effect) VALUES (?, 'keep', ?, 4)`, c.ID, in); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO character_presence_snapshots (character_id, kind, recorded_at, name, color, icon_url) VALUES (?, 'entry', 1, ?, '#ffffff', ?)`, c.ID, c.Name, in); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("../../migrations/117_avatar_imgur_only.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(string(data), ";\n") {
		if strings.Contains(stmt, "UPDATE") || strings.Contains(stmt, "INSERT") {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	for in, want := range values {
		var avatar, comment, icon string
		var aura int
		if err := db.QueryRow(`SELECT avatar_url, comment, aura_effect FROM character_profiles WHERE character_id = ?`, ids[in]).Scan(&avatar, &comment, &aura); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT icon_url FROM character_presence_snapshots WHERE character_id = ?`, ids[in]).Scan(&icon); err != nil {
			t.Fatal(err)
		}
		wantIcon := want
		if character.ValidateAvatarURL(want) != nil {
			wantIcon = "" // public snapshots only ever carry the avatar contract
		}
		if avatar != want || icon != wantIcon || comment != "keep" || aura != 4 {
			t.Errorf("%q: avatar=%q icon=%q comment=%q aura=%d", in, avatar, icon, comment, aura)
		}
		if avatar != "chr/052.gif" && character.ValidateAvatarURL(avatar) != nil {
			t.Errorf("%q: migrated value %q violates the Go contract", in, avatar)
		}
	}
}
