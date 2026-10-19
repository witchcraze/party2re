package database_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestHelperExpiredTargetCompletesWithMaterials(t *testing.T) {
	f := newHelperClaimFixture(t, 1)
	ctx := context.Background()
	// Save only updates completion fields; change the clock to expire the stored target.
	now := f.quest.ExpiresAt.Add(time.Second)
	res, err := f.service.CompleteQuest(ctx, f.characterIDs[0], f.quest.ID, now)
	if err != nil {
		t.Fatalf("expired target with materials: %v", err)
	}
	if res.CompletedQuest.ID != f.quest.ID {
		t.Fatalf("completed wrong target: %+v", res)
	}
	f.assertState(t, f.characterIDs[0])
}

func TestHelperFailedTurnInPreservesBoardAndAssets(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("expired_target=%t", expired), func(t *testing.T) {
			f := newHelperClaimFixture(t, 1)
			ctx := context.Background()
			other := f.quest
			other.ID = "expired-other"
			other.ExpiresAt = f.now.Add(-time.Hour)
			if err := f.quests.Save(ctx, other); err != nil {
				t.Fatal(err)
			}
			dep, err := f.depots.FindByCharacterID(ctx, f.characterIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			dep.Items = dep.Items[:0]
			if err := f.depots.Save(ctx, dep); err != nil {
				t.Fatal(err)
			}
			before, err := f.quests.ListUncompleted(ctx)
			if err != nil {
				t.Fatal(err)
			}
			now := f.now
			if expired {
				now = f.quest.ExpiresAt.Add(time.Second)
			}
			_, err = f.service.CompleteQuest(ctx, f.characterIDs[0], f.quest.ID, now)
			if !errors.Is(err, helperquest.ErrInsufficientItems) {
				t.Fatalf("error = %v, want insufficient items", err)
			}
			after, err := f.quests.ListUncompleted(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected turn-in changed board: before=%+v after=%+v", before, after)
			}
			char, err := f.chars.FindByID(ctx, f.characterIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			gotDepot, err := f.depots.FindByCharacterID(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			if char.HelpCount != 0 || !reflect.DeepEqual(dep, gotDepot) {
				t.Fatalf("rejected turn-in changed assets: help=%d depot=%+v", char.HelpCount, gotDepot)
			}
		})
	}
}

func TestHelperCompletionAndRenewalRollbackTogether(t *testing.T) {
	// Fail after the target replacement or either non-target replacement was inserted.
	for _, failAt := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("replacement=%d", failAt), func(t *testing.T) {
			f := newHelperClaimFixture(t, 1)
			ctx := context.Background()
			for _, id := range []string{"expired-a", "expired-b"} {
				q := f.quest
				q.ID, q.ExpiresAt = id, f.now.Add(-time.Hour)
				if err := f.quests.Save(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
			before, err := f.quests.ListUncompleted(ctx)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("save failed after insert")
			f.quests.replacements = nil // Count operation replacements, excluding seeded expired rows.
			f.quests.failReplacement, f.quests.failReplacementAt = failure, failAt
			res, err := f.service.CompleteQuest(ctx, f.characterIDs[0], f.quest.ID, f.now)
			if !errors.Is(err, failure) || res.CompletedQuest != nil || res.RenewedCount != 0 {
				t.Fatalf("failed operation result = %+v, %v", res, err)
			}
			after, err := f.quests.ListUncompleted(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rollback changed board: before=%+v after=%+v", before, after)
			}
			f.assertState(t, "")
			f.quests.failReplacement = nil
			res, err = f.service.CompleteQuest(ctx, f.characterIDs[0], f.quest.ID, f.now)
			if err != nil || res.RenewedCount != 2 {
				t.Fatalf("retry result = %+v, %v", res, err)
			}
			// One completion replacement and two non-target replacements persist.
			board, err := f.quests.ListUncompleted(ctx)
			if err != nil || len(board) != 3 {
				t.Fatalf("board after retry = %+v, %v", board, err)
			}
			char, err := f.chars.FindByID(ctx, f.characterIDs[0])
			if err != nil || char.HelpCount != 1 {
				t.Fatalf("character after retry = %+v, %v", char, err)
			}
			dep, err := f.depots.FindByCharacterID(ctx, char.ID)
			if err != nil || dep.Quantity(f.quest.TargetID) != 2 || dep.Quantity(f.quest.RewardItemID) != 1 {
				t.Fatalf("assets after retry = %+v, %v", dep, err)
			}
		})
	}
}

func TestHelperConcurrentCompletionAndRenewalConserveAssets(t *testing.T) {
	cfg := testutil.GetStressConfig()
	f := newHelperClaimFixture(t, cfg.Workers)
	ctx := context.Background()
	for _, id := range []string{"live-other", "expired-a", "expired-b"} {
		q := f.quest
		q.ID = id
		if id != "live-other" {
			q.ExpiresAt = f.now.Add(-time.Hour)
		}
		if err := f.quests.Save(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var completed, renewed atomic.Int32
	result := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
		questID := ""
		switch (worker + op) % 3 {
		case 0:
			questID = f.quest.ID
		case 1:
			questID = "live-other"
		}
		res, err := f.service.CompleteQuest(ctx, f.characterIDs[worker], questID, f.now)
		if errors.Is(err, helperquest.ErrQuestAlreadyDone) {
			return nil
		}
		if err != nil {
			return err
		}
		if res.CompletedQuest != nil {
			completed.Add(1)
		}
		renewed.Add(int32(res.RenewedCount))
		return nil
	})
	if result.Failures != 0 || completed.Load() != 2 || renewed.Load() != 2 {
		t.Fatalf("concurrent result=%+v completions=%d renewals=%d", result, completed.Load(), renewed.Load())
	}
	materials, rewards, help := 0, 0, 0
	for _, id := range f.characterIDs {
		char, err := f.chars.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := f.depots.FindByCharacterID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		materials += dep.Quantity(f.quest.TargetID)
		rewards += dep.Quantity(f.quest.RewardItemID)
		help += char.HelpCount
	}
	if materials != 4*cfg.Workers-4 || rewards != 2 || help != 2 {
		t.Fatalf("assets: materials=%d rewards=%d help=%d", materials, rewards, help)
	}
	board, err := f.quests.ListUncompleted(ctx)
	if err != nil || len(board) != 4 {
		t.Fatalf("board = %+v, %v", board, err)
	}
	for _, q := range board {
		if !q.ExpiresAt.After(f.now) {
			t.Fatalf("expired row survived: %+v", q)
		}
	}
}
