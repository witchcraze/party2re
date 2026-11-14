package player_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestDeletionFixtureReclaimsOwnedRecords(t *testing.T) {
	observer := newDeletionFixture(t)
	sentinel := observer.player(t)
	for _, deleted := range []bool{false, true} {
		name := "remaining after rollback"
		if deleted {
			name = "already deleted"
		}
		var playerIDs, characterIDs []string
		var guildID string
		var fixture *deletionFixture
		t.Run(name, func(t *testing.T) {
			f := newDeletionFixture(t)
			fixture = f
			owner := f.player(t)
			a, b := f.character(t, owner.ID), f.character(t, owner.ID)
			other := f.character(t, "")
			g := f.createGuild(t, a, b, other)
			guildID = g.ID
			if len(f.playerIDs) != 4 {
				t.Fatalf("tracked owners = %v, want destination and three generated owners", f.playerIDs)
			}
			playerIDs = append(playerIDs, f.playerIDs...)
			for _, c := range []corecharacter.Character{a, b, other} {
				characterIDs = append(characterIDs, c.ID)
			}
			if playerIDs[1] == owner.ID || playerIDs[1] == sentinel.ID || playerIDs[1] == playerIDs[2] {
				t.Fatal("original owners were overwritten")
			}
			for _, playerID := range playerIDs {
				if observer.count(t, `SELECT COUNT(*) FROM players WHERE id = ?`, playerID) != 1 {
					t.Fatalf("fixture player %s does not exist before teardown", playerID)
				}
			}
			if _, _, err := f.accounts.CreateAPIToken(context.Background(), owner.ID, "cleanup", nil); err != nil {
				t.Fatal(err)
			}
			if deleted {
				if err := f.accounts.DeleteAccount(context.Background(), owner.ID, ""); err != nil {
					t.Fatal(err)
				}
			}
			// Teardown must work even when failure injection remains armed.
			f.chars.failDelete, f.chars.failListing, f.players.failDelete = other.ID, true, true
		})
		for _, playerID := range playerIDs {
			if observer.count(t, `SELECT COUNT(*) FROM players WHERE id = ?`, playerID) != 0 ||
				observer.count(t, `SELECT COUNT(*) FROM player_api_tokens WHERE player_id = ?`, playerID) != 0 {
				t.Errorf("fixture player %s or its tokens survived teardown", playerID)
			}
		}
		for _, charID := range characterIDs {
			if observer.count(t, `SELECT COUNT(*) FROM characters WHERE id = ?`, charID) != 0 ||
				observer.count(t, `SELECT COUNT(*) FROM inventory_items WHERE character_id = ?`, charID) != 0 ||
				observer.count(t, `SELECT COUNT(*) FROM guild_members WHERE character_id = ?`, charID) != 0 {
				t.Errorf("fixture character %s or its dependents survived teardown", charID)
			}
		}
		if observer.count(t, `SELECT COUNT(*) FROM guilds WHERE id = ?`, guildID) != 0 {
			t.Error("fixture guild survived teardown")
		}
		for _, newsID := range fixture.newsIDs {
			if observer.count(t, `SELECT COUNT(*) FROM news_articles WHERE id = ?`, newsID) != 0 {
				t.Errorf("fixture news %s survived teardown", newsID)
			}
		}
		if observer.count(t, `SELECT COUNT(*) FROM players WHERE id = ?`, sentinel.ID) != 1 {
			t.Fatal("unrelated sentinel player was deleted")
		}
	}
}
