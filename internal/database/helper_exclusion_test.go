package database_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/helperquest"
)

// Generate a weapon request so replacements cannot accidentally request the old item.
type helperExclusionRandom struct{}

func (helperExclusionRandom) Intn(int) (int, error) { return 0, nil }

func TestHelperExclusionsFollowPersistedCompletionAndExchange(t *testing.T) {
	for _, complete := range []bool{false, true} {
		for _, duplicate := range []bool{false, true} {
			t.Run(fmt.Sprintf("complete=%t/duplicate=%t", complete, duplicate), func(t *testing.T) {
				f := newHelperClaimFixture(t, 1)
				ctx := context.Background()
				f.service.SetRandomSource(helperExclusionRandom{})
				now := f.quest.ExpiresAt.Add(time.Second)
				if duplicate {
					q := f.quest
					q.ID, q.ExpiresAt = "duplicate", now.Add(time.Hour)
					if err := f.quests.Save(ctx, q); err != nil {
						t.Fatal(err)
					}
				}
				assertExcluded := func(want bool) {
					t.Helper()
					ids, err := f.service.GetActiveHelperItemIDs(ctx, now)
					if err != nil {
						t.Fatal(err)
					}
					if slices.Contains(ids, f.quest.TargetID) != want {
						t.Fatalf("old target excluded = %t, want %t; ids=%v", slices.Contains(ids, f.quest.TargetID), want, ids)
					}
				}
				assertExcluded(true)
				targetID := ""
				if complete {
					targetID = f.quest.ID
				}
				if _, err := f.service.CompleteQuest(ctx, f.characterIDs[0], targetID, now); err != nil {
					t.Fatal(err)
				}
				stored, err := f.quests.HelperRepository.FindByID(ctx, f.quest.ID)
				if complete {
					if err != nil || stored.CompletedAt == nil {
						t.Fatalf("completed request = %+v, %v", stored, err)
					}
				} else if !errors.Is(err, helperquest.ErrQuestNotFound) {
					t.Fatalf("exchanged request still stored: %+v, %v", stored, err)
				}
				assertExcluded(duplicate)
				board, err := f.quests.ListUncompleted(ctx)
				if err != nil || len(board) == 0 {
					t.Fatalf("replacement board = %+v, %v", board, err)
				}
				ids, err := f.service.GetActiveHelperItemIDs(ctx, now)
				if err != nil || !slices.Contains(ids, "weapon-28") {
					t.Fatalf("new target not excluded: %v, %v", ids, err)
				}
				if duplicate {
					if _, err := f.service.CompleteQuest(ctx, f.characterIDs[0], "duplicate", now); err != nil {
						t.Fatal(err)
					}
					assertExcluded(false)
				}
			})
		}
	}
}
