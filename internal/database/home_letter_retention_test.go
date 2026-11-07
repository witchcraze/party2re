package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestLetterThrottleAndRetention(t *testing.T) {
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
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	newChar := func(t *testing.T) corecharacter.Character {
		t.Helper()
		char, err := database.CreateTestCharacter(ctx, db, "Mail_"+id.New()[:8])
		if err != nil {
			t.Fatal(err)
		}
		return char
	}
	letterAt := func(from, to corecharacter.Character, at time.Time) home.Letter {
		return home.Letter{
			ID: id.New(), SenderCharacterID: from.ID, SenderName: from.Name,
			RecipientCharacterID: to.ID, RecipientName: to.Name,
			Content: "hello", Color: "#000000", CreatedAt: at,
		}
	}
	send := func(t *testing.T, from, to corecharacter.Character, at time.Time, limit int) home.Letter {
		t.Helper()
		letter := letterAt(from, to, at)
		if err := repo.CreateLetter(ctx, letter, limit); err != nil {
			t.Fatalf("CreateLetter at %s: %v", at, err)
		}
		return letter
	}
	inboxIDs := func(t *testing.T, owner corecharacter.Character) []string {
		t.Helper()
		letters, _, err := repo.ListInboxLetters(ctx, owner.ID, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(letters))
		for _, l := range letters {
			ids = append(ids, l.ID)
		}
		return ids
	}
	outboxIDs := func(t *testing.T, owner corecharacter.Character) []string {
		t.Helper()
		letters, _, err := repo.ListOutboxLetters(ctx, owner.ID, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(letters))
		for _, l := range letters {
			ids = append(ids, l.ID)
		}
		return ids
	}
	assertIDs := func(t *testing.T, got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("expected %d letters, got %d", len(want), len(got))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("letter %d: expected %s, got %s", i, want[i], got[i])
			}
		}
	}
	const limit = home.DefaultLetterLogLimit

	t.Run("wait boundary", func(t *testing.T) {
		a, b := newChar(t), newChar(t)
		first := send(t, a, b, t0, limit)
		err := repo.CreateLetter(ctx, letterAt(a, b, t0.Add(29*time.Second)), limit)
		if !errors.Is(err, home.ErrLetterThrottled) {
			t.Fatalf("expected ErrLetterThrottled at 29s, got %v", err)
		}
		assertIDs(t, inboxIDs(t, b), first.ID)
		second := send(t, a, b, t0.Add(30*time.Second), limit)
		assertIDs(t, inboxIDs(t, b), second.ID, first.ID)
	})

	t.Run("another sender resets the check", func(t *testing.T) {
		a, b, c := newChar(t), newChar(t), newChar(t)
		send(t, a, b, t0, limit)
		send(t, c, b, t0.Add(time.Second), limit)
		send(t, a, b, t0.Add(2*time.Second), limit)
	})

	t.Run("same sender to another recipient is allowed", func(t *testing.T) {
		a, b, c := newChar(t), newChar(t), newChar(t)
		send(t, a, b, t0, limit)
		send(t, a, c, t0, limit)
	})

	t.Run("sender deleting their copy does not bypass the check", func(t *testing.T) {
		a, b := newChar(t), newChar(t)
		first := send(t, a, b, t0, limit)
		if err := repo.DeleteLetter(ctx, first.ID, a.ID); err != nil {
			t.Fatal(err)
		}
		err := repo.CreateLetter(ctx, letterAt(a, b, t0.Add(time.Second)), limit)
		if !errors.Is(err, home.ErrLetterThrottled) {
			t.Fatalf("expected ErrLetterThrottled, got %v", err)
		}
	})

	t.Run("recipient deleting the latest letter removes it from the check", func(t *testing.T) {
		a, b := newChar(t), newChar(t)
		first := send(t, a, b, t0, limit)
		if err := repo.DeleteLetter(ctx, first.ID, b.ID); err != nil {
			t.Fatal(err)
		}
		send(t, a, b, t0.Add(time.Second), limit)
	})

	t.Run("inbox keeps the newest letters and the sender's copy", func(t *testing.T) {
		a, b, c, d := newChar(t), newChar(t), newChar(t), newChar(t)
		oldest := send(t, a, d, t0, 2)
		middle := send(t, b, d, t0.Add(time.Second), 2)
		newest := send(t, c, d, t0.Add(2*time.Second), 2)
		assertIDs(t, inboxIDs(t, d), newest.ID, middle.ID)
		assertIDs(t, outboxIDs(t, a), oldest.ID)
		count, err := repo.GetUnreadLetterCount(ctx, d.ID)
		if err != nil || count != 2 {
			t.Fatalf("expected 2 unread letters, got %d (%v)", count, err)
		}
	})

	t.Run("outbox keeps the newest letters and the recipient's copy", func(t *testing.T) {
		a, b, c, d := newChar(t), newChar(t), newChar(t), newChar(t)
		oldest := send(t, a, b, t0, 2)
		middle := send(t, a, c, t0.Add(time.Second), 2)
		newest := send(t, a, d, t0.Add(2*time.Second), 2)
		assertIDs(t, outboxIDs(t, a), newest.ID, middle.ID)
		assertIDs(t, inboxIDs(t, b), oldest.ID)
	})

	t.Run("a letter evicted from both boxes is purged", func(t *testing.T) {
		a, b := newChar(t), newChar(t)
		oldest := send(t, a, b, t0, 1)
		send(t, a, b, t0.Add(30*time.Second), 1)
		if _, err := repo.GetLetterByID(ctx, oldest.ID); !errors.Is(err, home.ErrLetterNotFound) {
			t.Fatalf("expected purged letter, got %v", err)
		}
	})

	t.Run("a lowered limit trims on the next send", func(t *testing.T) {
		a, b := newChar(t), newChar(t)
		send(t, a, b, t0, limit)
		send(t, a, b, t0.Add(30*time.Second), limit)
		middle := send(t, a, b, t0.Add(60*time.Second), limit)
		newest := send(t, a, b, t0.Add(90*time.Second), 2)
		assertIDs(t, inboxIDs(t, b), newest.ID, middle.ID)
		assertIDs(t, outboxIDs(t, a), newest.ID, middle.ID)
	})

	t.Run("a rejected letter does not trim", func(t *testing.T) {
		a, b, c := newChar(t), newChar(t), newChar(t)
		first := send(t, c, b, t0, limit)
		second := send(t, a, b, t0.Add(time.Second), limit)
		err := repo.CreateLetter(ctx, letterAt(a, b, t0.Add(2*time.Second)), 1)
		if !errors.Is(err, home.ErrLetterThrottled) {
			t.Fatalf("expected ErrLetterThrottled, got %v", err)
		}
		assertIDs(t, inboxIDs(t, b), second.ID, first.ID)
	})

	t.Run("concurrent letters from one sender deliver once", func(t *testing.T) {
		a, b := newChar(t), newChar(t)
		fns := make([]func() error, 8)
		for i := range fns {
			fns[i] = func() error { return repo.CreateLetter(ctx, letterAt(a, b, t0), limit) }
		}
		delivered := 0
		for _, err := range testutil.RunRace(fns...) {
			switch {
			case err == nil:
				delivered++
			case !errors.Is(err, home.ErrLetterThrottled):
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if delivered != 1 {
			t.Fatalf("expected 1 delivered letter, got %d", delivered)
		}
		if got := inboxIDs(t, b); len(got) != 1 {
			t.Fatalf("expected 1 inbox letter, got %d", len(got))
		}
	})

	t.Run("concurrent crossing letters keep both limits without deadlock", func(t *testing.T) {
		const logLimit = 3
		chars := []corecharacter.Character{newChar(t), newChar(t), newChar(t), newChar(t)}
		var fns []func() error
		for offset := range 5 {
			for _, from := range chars {
				for _, to := range chars {
					if from.ID == to.ID {
						continue
					}
					at := t0.Add(time.Duration(offset) * time.Minute)
					fns = append(fns, func() error { return repo.CreateLetter(ctx, letterAt(from, to, at), logLimit) })
				}
			}
		}
		for _, err := range testutil.RunRace(fns...) {
			if err != nil && !errors.Is(err, home.ErrLetterThrottled) {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		for _, char := range chars {
			if got := inboxIDs(t, char); len(got) > logLimit {
				t.Errorf("inbox holds %d letters", len(got))
			}
			if got := outboxIDs(t, char); len(got) > logLimit {
				t.Errorf("outbox holds %d letters", len(got))
			}
		}
	})
}
