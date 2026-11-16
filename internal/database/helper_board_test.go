package database_test

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestHelperBoardRenewal(t *testing.T) {
	ctx := context.Background()
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprintf("expired=%d", count), func(t *testing.T) {
			f := newRenewalFixture(t)
			for i := range count {
				f.seed(t, fmt.Sprintf("expired-%d", i), f.now.Add(-time.Hour), "")
			}
			before, err := f.service.ListQuests(ctx, f.now)
			if err != nil {
				t.Fatal(err)
			}
			if len(before) != count || f.count(t) != count {
				t.Fatalf("reads must expose all expired requests without writes: listed=%d stored=%d want=%d", len(before), f.count(t), count)
			}
			for _, q := range before {
				if !q.ExpiresAt.Before(f.now) {
					t.Fatalf("read renewed expired request: %+v", q)
				}
			}
			for i, want := range []int{count, 0} { // a retry must not exchange fresh replacements
				res, err := f.service.CompleteQuest(ctx, "helper-character", "", f.now)
				if err != nil || len(res.Renewed) != want || res.CompletedQuest != nil {
					t.Fatalf("renewal-only %d: %+v %v", i, res, err)
				}
			}
			after, err := f.service.ListQuests(ctx, f.now)
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != count || f.count(t) != count {
				t.Fatalf("one-for-one renewal: listed=%d stored=%d want=%d", len(after), f.count(t), count)
			}
			for _, q := range after {
				if !q.ExpiresAt.After(f.now) || q.CompletedAt != nil {
					t.Fatalf("expected fresh uncompleted request: %+v", q)
				}
			}
		})
	}
}

func TestHelperBoardListsUncompletedRegardlessOfDeadline(t *testing.T) {
	f := newRenewalFixture(t)
	f.seed(t, "expired", f.now.Add(-time.Hour), "")
	f.seed(t, "boundary", f.now, "")
	f.seed(t, "live", f.now.Add(time.Hour), "")
	f.seed(t, "done", f.now.Add(time.Hour), "someone")
	board, err := f.service.ListQuests(context.Background(), f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(board) != 3 || board[0].ID != "boundary" || board[1].ID != "expired" || board[2].ID != "live" {
		t.Fatalf("board = %+v, want boundary, expired, live in stable order", board)
	}
	active, err := f.service.GetActiveHelperItemIDs(context.Background(), f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 3 {
		t.Fatalf("shop exclusion must include every uncompleted request: %v", active)
	}
	if f.count(t) != 4 {
		t.Fatalf("reads changed stored rows: %d", f.count(t))
	}
}
