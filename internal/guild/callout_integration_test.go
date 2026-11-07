package guild_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
)

func TestBroadcastCalloutDeliversToSenderAndMembers_RealDB(t *testing.T) {
	f := newRemovalFixture(t, false)
	ctx := context.Background()
	homes, err := database.NewHomeRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}

	// 100 double quotes pass the callout limit but count as 600 letter characters.
	err = f.service.BroadcastCallout(ctx, f.g.ID, f.leader.ID, strings.Repeat(`"`, 100))
	if !errors.Is(err, home.ErrContentTooLong) {
		t.Fatalf("expected ErrContentTooLong, got %v", err)
	}
	letters, _, err := homes.ListInboxLetters(ctx, f.leader.ID, 10, 0)
	if err != nil || len(letters) != 0 {
		t.Fatalf("expected no delivery, got %d letters (%v)", len(letters), err)
	}

	if err := f.service.BroadcastCallout(ctx, f.g.ID, f.leader.ID, "集合"); err != nil {
		t.Fatalf("BroadcastCallout failed: %v", err)
	}
	for _, recipientID := range []string{f.leader.ID, f.target.ID} {
		letters, _, err := homes.ListInboxLetters(ctx, recipientID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(letters) != 1 || letters[0].Content != "集合" {
			t.Fatalf("recipient %s: unexpected inbox %+v", recipientID, letters)
		}
	}

	if err := f.service.BroadcastCallout(ctx, f.g.ID, f.leader.ID, "もう一度"); !errors.Is(err, home.ErrLetterThrottled) {
		t.Fatalf("expected ErrLetterThrottled, got %v", err)
	}
}
