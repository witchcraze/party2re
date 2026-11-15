package home_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
)

func TestDeliveryNoticesPageIntegration(t *testing.T) {
	db := homeIsolatedDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := home.NewService(repo, chars)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 123456000, time.UTC)
	for _, count := range []int{0, 50, 51, 125} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			actor, err := database.CreateTestCharacter(ctx, db, id.New()[:corecharacter.MaxNameLength])
			if err != nil {
				t.Fatal(err)
			}
			other, err := database.CreateTestCharacter(ctx, db, id.New()[:corecharacter.MaxNameLength])
			if err != nil {
				t.Fatal(err)
			}
			noticeID := func(i int) string { return fmt.Sprintf("%s%03d", actor.ID[:20], i) }
			add := func(owner string, i int, at time.Time) {
				t.Helper()
				if err := repo.AddDeliveryNotice(ctx, home.DeliveryNotice{ID: noticeID(i), CharacterID: owner, CreatedAt: at}); err != nil {
					t.Fatal(err)
				}
			}
			for i := 1; i <= count; i++ {
				add(actor.ID, i, base)
			}
			add(other.ID, 900, base)
			first, err := svc.ListDeliveryNoticesPage(ctx, actor.ID, actor.PlayerID, 50, 0, "")
			if err != nil || len(first.Items) != min(count, 50) || first.HasMore != (count > 50) {
				t.Fatalf("first=%+v err=%v", first, err)
			}
			// GET is read-only, and deterministic among equal microsecond timestamps.
			again, err := svc.ListDeliveryNoticesPage(ctx, actor.ID, actor.PlayerID, 50, 0, "")
			if err != nil || len(again.Items) != len(first.Items) {
				t.Fatalf("repeat: %+v %v", again, err)
			}
			for i, n := range first.Items {
				if n.ID != noticeID(count-i) || n.IsCleared || again.Items[i].ID != n.ID {
					t.Fatalf("order/mutation: %+v", n)
				}
			}
			if !first.HasMore {
				return
			}
			add(actor.ID, 901, base.Add(time.Second))
			add(actor.ID, 902, base) // New notice also sharing the old timestamp.
			ids := make([]string, len(first.Items))
			for i, n := range first.Items {
				ids[i] = n.ID
			}
			if err := svc.ClearDeliveryNotices(ctx, other.ID, ids); err != nil {
				t.Fatal(err)
			}
			if got, err := svc.ListDeliveryNoticesPage(ctx, actor.ID, actor.PlayerID, 50, 0, ""); err != nil || len(got.Items) != 50 || got.Items[2].ID != first.Items[0].ID {
				t.Fatalf("foreign ack changed owner: %+v %v", got, err)
			}
			// Acknowledging all displayed rows also removes the cursor row itself.
			if err := svc.ClearDeliveryNotices(ctx, actor.ID, ids); err != nil {
				t.Fatal(err)
			}
			remaining := count - 50
			cursor := first.NextCursor
			for remaining > 0 {
				page, err := svc.ListDeliveryNoticesPage(ctx, actor.ID, actor.PlayerID, 50, 0, cursor)
				if err != nil || len(page.Items) != min(remaining, 50) || page.HasMore != (remaining > 50) {
					t.Fatalf("continuation: %+v %v", page, err)
				}
				for i, n := range page.Items {
					if n.ID != noticeID(remaining-i) || n.IsCleared {
						t.Fatalf("skipped/duplicate: %+v", n)
					}
				}
				remaining -= len(page.Items)
				cursor = page.NextCursor
			}
			fresh, err := svc.ListDeliveryNoticesPage(ctx, actor.ID, actor.PlayerID, 50, 0, "")
			if err != nil || len(fresh.Items) < 2 || fresh.Items[0].ID != noticeID(901) || fresh.Items[1].ID != noticeID(902) {
				t.Fatalf("unsubmitted new arrivals cleared: %+v %v", fresh, err)
			}
		})
	}
}
