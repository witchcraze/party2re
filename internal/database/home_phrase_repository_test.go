package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestCompanionPhraseRetention(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	newCharacter := func(t *testing.T) string {
		t.Helper()
		char, err := database.CreateTestCharacter(ctx, db, id.New()[:corecharacter.MaxNameLength])
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			for _, table := range []string{"character_homes", "character_depots"} {
				if _, err := db.ExecContext(cleanupCtx, "DELETE FROM "+table+" WHERE character_id = ?", char.ID); err != nil {
					t.Errorf("cleanup %s failed: %v", table, err)
				}
			}
			// Deleting the character cascades to its phrases.
			if _, err := db.ExecContext(cleanupCtx, "DELETE FROM characters WHERE id = ?", char.ID); err != nil {
				t.Errorf("cleanup character failed: %v", err)
			}
			if _, err := db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", char.PlayerID); err != nil {
				t.Errorf("cleanup player failed: %v", err)
			}
		})
		return char.ID
	}
	phraseAt := func(charID, text string, at time.Time) home.CompanionPhrase {
		return home.CompanionPhrase{ID: id.New(), CharacterID: charID, Phrase: text, CreatedAt: at}
	}
	seed := func(t *testing.T, charID string, count int) {
		t.Helper()
		for i := range count {
			p := phraseAt(charID, fmt.Sprintf("p%d", i), base.Add(time.Duration(i)*time.Second))
			if err := repo.AddCompanionPhrase(ctx, p, 1000); err != nil {
				t.Fatal(err)
			}
		}
	}
	texts := func(t *testing.T, charID string) []string {
		t.Helper()
		phrases, err := repo.ListCompanionPhrases(ctx, charID)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(phrases))
		for i, p := range phrases {
			out[i] = p.Phrase
		}
		return out
	}

	const limit = 3
	for _, tc := range []struct {
		name   string
		before int
		want   []string
	}{
		{"below limit keeps the oldest", limit - 1, []string{"new", "p1", "p0"}},
		{"at limit evicts the oldest", limit, []string{"new", "p2", "p1"}},
		{"above a lowered limit trims every excess entry", limit + 2, []string{"new", "p4", "p3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			charID := newCharacter(t)
			seed(t, charID, tc.before)
			if err := repo.AddCompanionPhrase(ctx, phraseAt(charID, "new", base.Add(time.Hour)), limit); err != nil {
				t.Fatal(err)
			}
			if got := texts(t, charID); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("limit one keeps only the new phrase", func(t *testing.T) {
		charID := newCharacter(t)
		seed(t, charID, 1)
		if err := repo.AddCompanionPhrase(ctx, phraseAt(charID, "new", base.Add(time.Hour)), 1); err != nil {
			t.Fatal(err)
		}
		if got := texts(t, charID); fmt.Sprint(got) != "[new]" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("duplicates count as separate entries", func(t *testing.T) {
		charID := newCharacter(t)
		for i := range 4 {
			if err := repo.AddCompanionPhrase(ctx, phraseAt(charID, "same", base.Add(time.Duration(i)*time.Second)), limit); err != nil {
				t.Fatal(err)
			}
		}
		if got := texts(t, charID); fmt.Sprint(got) != "[same same same]" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("equal timestamps order by id descending", func(t *testing.T) {
		charID := newCharacter(t)
		for _, phraseID := range []string{"tie-b", "tie-c", "tie-a"} {
			p := home.CompanionPhrase{ID: phraseID + charID[:8], CharacterID: charID, Phrase: phraseID, CreatedAt: base}
			if err := repo.AddCompanionPhrase(ctx, p, limit); err != nil {
				t.Fatal(err)
			}
		}
		if got := texts(t, charID); fmt.Sprint(got) != "[tie-c tie-b tie-a]" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("failed insert keeps existing phrases", func(t *testing.T) {
		charID := newCharacter(t)
		first := phraseAt(charID, "first", base)
		if err := repo.AddCompanionPhrase(ctx, first, 1); err != nil {
			t.Fatal(err)
		}
		duplicateID := phraseAt(charID, "second", base.Add(time.Hour))
		duplicateID.ID = first.ID
		if err := repo.AddCompanionPhrase(ctx, duplicateID, 1); err == nil {
			t.Fatal("expected duplicate ID failure")
		}
		if got := texts(t, charID); fmt.Sprint(got) != "[first]" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("ambient rollback restores evicted phrases", func(t *testing.T) {
		charID := newCharacter(t)
		seed(t, charID, limit)
		sentinel := errors.New("rollback")
		err := database.RunInTx(ctx, db, func(txCtx context.Context) error {
			if err := repo.AddCompanionPhrase(txCtx, phraseAt(charID, "new", base.Add(time.Hour)), limit); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected rollback, got %v", err)
		}
		if got := texts(t, charID); fmt.Sprint(got) != "[p2 p1 p0]" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("unknown character is reported", func(t *testing.T) {
		err := repo.AddCompanionPhrase(ctx, phraseAt(id.New(), "new", base), limit)
		if !errors.Is(err, home.ErrCharacterNotFound) {
			t.Errorf("got %v", err)
		}
	})

	t.Run("concurrent teaching never exceeds the limit", func(t *testing.T) {
		charID := newCharacter(t)
		seed(t, charID, limit)
		const writers = 12
		fns := make([]func() error, writers)
		for i := range writers {
			p := phraseAt(charID, fmt.Sprintf("w%d", i), base.Add(time.Hour+time.Duration(i)*time.Second))
			fns[i] = func() error { return repo.AddCompanionPhrase(ctx, p, limit) }
		}
		for _, err := range testutil.RunRace(fns...) {
			if err != nil {
				t.Errorf("concurrent teach failed: %v", err)
			}
		}
		// The timestamps are distinct, so the newest three writers must survive any commit order.
		if got := texts(t, charID); fmt.Sprint(got) != "[w11 w10 w9]" {
			t.Errorf("got %v", got)
		}
	})
}
