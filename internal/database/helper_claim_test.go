package database_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type claimQuestRepository struct {
	*database.HelperRepository
	readBarrier       sync.WaitGroup
	syncReads         bool
	mu                sync.Mutex
	replacements      []string
	failReplacement   error
	failReplacementAt int
}

// Synchronize the old unlocked read path to reproduce two stale snapshots.
func (r *claimQuestRepository) FindByID(ctx context.Context, questID string) (helperquest.Quest, error) {
	q, err := r.HelperRepository.FindByID(ctx, questID)
	if r.syncReads {
		r.readBarrier.Done()
		r.readBarrier.Wait()
	}
	return q, err
}

func (r *claimQuestRepository) Save(ctx context.Context, q helperquest.Quest) error {
	if err := r.HelperRepository.Save(ctx, q); err != nil {
		return err
	}
	if q.CompletedAt == nil {
		r.mu.Lock()
		r.replacements = append(r.replacements, q.ID)
		fail := r.failReplacementAt == 0 || len(r.replacements) == r.failReplacementAt
		r.mu.Unlock()
		if fail {
			return r.failReplacement
		}
	}
	return nil
}

type helperClaimFixture struct {
	quests       *claimQuestRepository
	chars        *database.CharacterRepository
	depots       *database.DepotRepository
	service      *helperquest.Service
	quest        helperquest.Quest
	characterIDs []string
	now          time.Time
}

func newHelperClaimFixture(t *testing.T, count int) helperClaimFixture {
	t.Helper()
	db := helperIsolatedDB(t, "players", "characters", "character_depots", "depot_items", "helper_quests")
	quests, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := helperClaimFixture{quests: &claimQuestRepository{HelperRepository: quests}, chars: chars, depots: depots, now: time.Now().UTC().Truncate(time.Second)}
	f.service = helperquest.NewService(f.quests, chars, nil, database.NewTransactionProvider(db), helperquest.WithDepotRepository(depots))
	f.quest = helperquest.Quest{ID: id.New(), Title: "Concurrent claim", Kind: helperquest.KindItem, TargetID: "item-001", RequiredCount: 2, RewardItemID: "item-128", CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour)}
	ctx := context.Background()
	if err := quests.Save(ctx, f.quest); err != nil {
		t.Fatal(err)
	}
	for range count {
		char, err := database.CreateTestCharacter(ctx, db, "Claimant")
		if err != nil {
			t.Fatal(err)
		}
		materials, err := item.NewInstance(f.quest.TargetID, 4)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestDepot(ctx, db, char.ID, 5, []item.Instance{materials}); err != nil {
			t.Fatal(err)
		}
		f.characterIDs = append(f.characterIDs, char.ID)
	}
	return f
}

func (f helperClaimFixture) assertState(t *testing.T, winner string) {
	t.Helper()
	ctx := context.Background()
	q, err := f.quests.HelperRepository.FindByID(ctx, f.quest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q.CompletedBy != winner || (q.CompletedAt != nil) != (winner != "") {
		t.Fatalf("completion = %+v, want winner %q", q, winner)
	}
	for _, characterID := range f.characterIDs {
		char, err := f.chars.FindByID(ctx, characterID)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := f.depots.FindByCharacterID(ctx, characterID)
		if err != nil {
			t.Fatal(err)
		}
		completed := 0
		if characterID == winner {
			completed = 1
		}
		if char.HelpCount != completed || dep.Quantity(f.quest.TargetID) != 4-2*completed || dep.Quantity(f.quest.RewardItemID) != completed {
			t.Errorf("character %s: help=%d materials=%d rewards=%d; completed=%d", characterID, char.HelpCount, dep.Quantity(f.quest.TargetID), dep.Quantity(f.quest.RewardItemID), completed)
		}
	}
	replacements := 0
	for _, questID := range f.quests.replacements {
		_, err := f.quests.HelperRepository.FindByID(ctx, questID)
		if err == nil {
			replacements++
		} else if !errors.Is(err, helperquest.ErrQuestNotFound) {
			t.Fatal(err)
		}
	}
	want := 0
	if winner != "" {
		want = 1
	}
	if replacements != want {
		t.Errorf("persisted replacements = %d, want %d", replacements, want)
	}
}

func TestHelperQuestConcurrentClaim(t *testing.T) {
	for _, sameCharacter := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_character=%t", sameCharacter), func(t *testing.T) {
			f := newHelperClaimFixture(t, 2)
			f.quests.syncReads = true
			f.quests.readBarrier.Add(2)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			contenders := append([]string(nil), f.characterIDs...)
			if sameCharacter {
				contenders[1] = contenders[0]
			}
			var successes atomic.Int32
			var winner string
			operations := make([]func() error, 2)
			for i, characterID := range contenders {
				operations[i] = func() error {
					_, err := f.service.CompleteQuest(ctx, characterID, f.quest.ID, f.now)
					if err == nil {
						successes.Add(1)
						f.quests.mu.Lock()
						winner = characterID
						f.quests.mu.Unlock()
					}
					return err
				}
			}
			for _, err := range testutil.RunRace(operations...) {
				if err != nil && !errors.Is(err, helperquest.ErrQuestAlreadyDone) {
					t.Errorf("claim: %v", err)
				}
			}
			if successes.Load() != 1 {
				t.Fatalf("successful claims = %d, want 1", successes.Load())
			}
			f.assertState(t, winner)
		})
	}
}

func TestHelperQuestClaimStress(t *testing.T) {
	cfg := testutil.GetStressConfig()
	f := newHelperClaimFixture(t, cfg.Workers)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var successes atomic.Int32
	var winner string
	result := testutil.RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
		characterID := f.characterIDs[workerID]
		_, err := f.service.CompleteQuest(ctx, characterID, f.quest.ID, f.now)
		if err == nil {
			successes.Add(1)
			f.quests.mu.Lock()
			winner = characterID
			f.quests.mu.Unlock()
			return nil
		}
		if errors.Is(err, helperquest.ErrQuestAlreadyDone) {
			return nil
		}
		return err
	})
	if result.Failures != 0 || successes.Load() != 1 {
		t.Fatalf("stress result=%+v, successful claims=%d", result, successes.Load())
	}
	f.assertState(t, winner)
}

func TestHelperQuestClaimRollback(t *testing.T) {
	f := newHelperClaimFixture(t, 1)
	failure := errors.New("replacement failed after insert")
	f.quests.failReplacement = failure
	result, err := f.service.CompleteQuest(context.Background(), f.characterIDs[0], f.quest.ID, f.now)
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want %v", err, failure)
	}
	if result.CompletedQuest != nil || result.NewQuest != nil || result.RenewedCount != 0 {
		t.Fatalf("failed claim returned success result: %+v", result)
	}
	f.assertState(t, "")
	f.quests.failReplacement = nil
	if _, err := f.service.CompleteQuest(context.Background(), f.characterIDs[0], f.quest.ID, f.now); err != nil {
		t.Fatal(err)
	}
	f.assertState(t, f.characterIDs[0])
}
