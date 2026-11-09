package player_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Deleting an identity removes only that identity's mailbox copies (#1094).
func TestIdentityDeletionPreservesSurvivingMailboxCopies(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	chars, err := character.NewService(charRepo, character.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := player.NewService(players, player.NewValkeySessionRepository(nil),
		player.WithCharacterService(chars), player.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}

	newChar := func(t *testing.T) corecharacter.Character {
		t.Helper()
		c, err := database.CreateTestCharacter(ctx, db, "Mail")
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	n := 0
	send := func(t *testing.T, from, to corecharacter.Character, mutate func(*home.Letter)) home.Letter {
		t.Helper()
		n++
		l := home.Letter{
			ID: id.New(), SenderCharacterID: from.ID, SenderName: from.Name,
			RecipientCharacterID: to.ID, RecipientName: to.Name,
			Content: fmt.Sprintf("letter %d", n), Color: "#123456", CreatedAt: base.Add(time.Duration(n) * time.Minute),
		}
		if mutate != nil {
			mutate(&l)
		}
		if err := homes.CreateLetter(ctx, l, 100); err != nil {
			t.Fatal(err)
		}
		return l
	}
	readAt := base.Add(-time.Minute)
	read := func(l *home.Letter) { l.IsRead, l.ReadAt = true, &readAt }
	gone := func(t *testing.T, letterID string) {
		t.Helper()
		if _, err := homes.GetLetterByID(ctx, letterID); !errors.Is(err, home.ErrLetterNotFound) {
			t.Fatalf("letter %s should be purged, got %v", letterID, err)
		}
	}
	ids := func(ls []home.Letter) []string {
		out := make([]string, len(ls))
		for i, l := range ls {
			out[i] = l.ID
		}
		return out
	}
	assertBox := func(t *testing.T, got []home.Letter, total int, want ...home.Letter) {
		t.Helper()
		if total != len(want) || len(got) != len(want) {
			t.Fatalf("box = %v (total %d), want %v", ids(got), total, ids(want))
		}
		for i := range want { // boxes list newest first
			w, g := want[len(want)-1-i], got[i]
			if g.ID != w.ID || g.SenderName != w.SenderName || g.RecipientName != w.RecipientName ||
				g.Content != w.Content || g.Color != w.Color || g.IsRead != w.IsRead || !g.CreatedAt.Equal(w.CreatedAt) {
				t.Fatalf("box[%d] = %+v, want %+v", i, g, w)
			}
		}
	}

	run := func(t *testing.T, deleteA func(a corecharacter.Character) error) {
		a, b, c := newChar(t), newChar(t), newChar(t)
		toB := send(t, a, b, nil)      // unread inbox copy
		toBRead := send(t, a, b, read) // read inbox copy
		toBHiddenForB := send(t, a, b, func(l *home.Letter) { l.IsDeletedByRecipient = true })
		toBHiddenForA := send(t, a, b, func(l *home.Letter) { l.IsDeletedBySender = true })
		fromB := send(t, b, a, nil) // B's outbox copy
		fromBHiddenForB := send(t, b, a, func(l *home.Letter) { l.IsDeletedBySender = true })
		self := send(t, a, a, nil)
		unrelated := send(t, c, a, nil)

		if err := deleteA(a); err != nil {
			t.Fatal(err)
		}
		if _, err := charRepo.FindByID(ctx, a.ID); !errors.Is(err, corecharacter.ErrNotFound) {
			t.Fatalf("character A still exists: %v", err)
		}

		inbox, total, err := homes.ListInboxLetters(ctx, b.ID, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertBox(t, inbox, total, toB, toBRead, toBHiddenForA)
		outbox, total, err := homes.ListOutboxLetters(ctx, b.ID, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertBox(t, outbox, total, fromB)
		if unread, err := homes.GetUnreadLetterCount(ctx, b.ID); err != nil || unread != 2 {
			t.Fatalf("unread = %d, %v; want 2", unread, err)
		}
		cOut, total, err := homes.ListOutboxLetters(ctx, c.ID, 20, 0)
		if err != nil {
			t.Fatal(err)
		}
		assertBox(t, cOut, total, unrelated)
		for _, l := range []home.Letter{toBHiddenForB, fromBHiddenForB, self} {
			gone(t, l.ID)
		}

		// Survivors keep owner-scoped permissions; nothing opens to others.
		if err := homes.MarkLetterAsRead(ctx, toB.ID, b.ID, base); err != nil {
			t.Fatal(err)
		}
		// The recipient copy is hidden, so this reports not-found before the ownership check.
		if err := homes.MarkLetterAsRead(ctx, unrelated.ID, b.ID, base); !errors.Is(err, home.ErrForbidden) && !errors.Is(err, home.ErrLetterNotFound) {
			t.Fatalf("mark unrelated = %v, want denial", err)
		}
		if err := homes.DeleteLetter(ctx, unrelated.ID, b.ID); !errors.Is(err, home.ErrForbidden) {
			t.Fatalf("delete unrelated = %v, want forbidden", err)
		}
		if err := homes.DeleteLetter(ctx, toB.ID, b.ID); err != nil {
			t.Fatal(err)
		}
		gone(t, toB.ID)
		if err := homes.DeleteLetter(ctx, fromB.ID, b.ID); err != nil {
			t.Fatal(err)
		}
		gone(t, fromB.ID)

		// Removing the second owner purges whatever both owners have left.
		if err := chars.Delete(ctx, b.PlayerID, b.ID); err != nil {
			t.Fatal(err)
		}
		gone(t, toBRead.ID)
		gone(t, toBHiddenForA.ID)
		if err := chars.Delete(ctx, c.PlayerID, c.ID); err != nil {
			t.Fatal(err)
		}
		gone(t, unrelated.ID)
	}

	t.Run("character deletion", func(t *testing.T) {
		run(t, func(a corecharacter.Character) error { return chars.Delete(ctx, a.PlayerID, a.ID) })
	})
	t.Run("account deletion", func(t *testing.T) {
		run(t, func(a corecharacter.Character) error { return accounts.DeleteAccount(ctx, a.PlayerID, "") })
	})
}

// A letter delivered while its recipient is being deleted must not outlive both owners' copies.
func TestCharacterDeletionRacingLetterDelivery(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := character.NewService(charRepo, character.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	newChar := func() corecharacter.Character {
		c, err := database.CreateTestCharacter(ctx, db, "Race")
		if err != nil {
			t.Fatal(err)
		}
		return c
	}

	const rounds, senders = 3, 4
	for round := 0; round < rounds; round++ {
		target := newChar()
		from := make([]corecharacter.Character, senders)
		for i := range from {
			from[i] = newChar()
		}
		fns := []func() error{func() error { return chars.Delete(ctx, target.PlayerID, target.ID) }}
		for _, s := range from {
			fns = append(fns, func() error {
				err := homes.CreateLetter(ctx, home.Letter{
					ID: id.New(), SenderCharacterID: s.ID, SenderName: s.Name,
					RecipientCharacterID: target.ID, RecipientName: target.Name,
					Content: "race", Color: "#000000", CreatedAt: time.Now().UTC(),
				}, 30)
				if errors.Is(err, home.ErrCharacterNotFound) {
					return nil
				}
				return err
			})
		}
		for i, err := range testutil.RunRace(fns...) {
			if err != nil {
				t.Fatalf("round %d fn %d: %v", round, i, err)
			}
		}
		var leaked int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM character_letters WHERE recipient_character_id = ? AND is_deleted_by_recipient = FALSE`, target.ID).Scan(&leaked); err != nil {
			t.Fatal(err)
		}
		if leaked != 0 {
			t.Fatalf("round %d: %d letters keep a copy for the deleted recipient", round, leaked)
		}
		for _, s := range from {
			if err := chars.Delete(ctx, s.PlayerID, s.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
}
