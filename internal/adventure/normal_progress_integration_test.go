package adventure_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type normalBlessings struct {
	values map[string]string
	err    error
	actor  string
	cancel context.CancelFunc
}

func (b *normalBlessings) GetActiveBlessing(_ context.Context, actor string) (string, error) {
	b.actor = actor
	if b.cancel != nil {
		b.cancel()
	}
	return b.values[actor], b.err
}

func TestNormalRunOrdinaryBossRequiresExplicitTreasureAdvance(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	runID := entry.Command.RunID
	for floor := 1; floor <= 10; floor++ {
		f.send(t, runID, 0, "advance", "")
		r, err := f.store.FindRun(context.Background(), runID)
		if err != nil || r.Floor != floor || r.EncounterFloor != floor || len(r.Enemies) == 0 || len(r.Boxes) != 0 {
			t.Fatal(r, err)
		}
		f.send(t, runID, 0, "kill", "")
		r, err = f.store.FindRun(context.Background(), runID)
		if err != nil || r.StageCleared != (floor == 10) || r.Floor != floor || len(r.Boxes) != 0 {
			t.Fatal(r, err)
		}
	}
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r.Floor != 11 || !r.StageCleared || len(r.Boxes) == 0 {
		t.Fatal(r, err)
	}
	for _, box := range r.Boxes {
		if box.OpenedBy != "" {
			t.Fatal("treasure opened automatically")
		}
	}
	f.send(t, runID, 0, "exit", "")
	stats, err := f.history.GetAggregatedStats(context.Background(), f.actors[0].ID)
	if err != nil || stats.TotalVictories != 1 || stats.TotalAdventures != 1 {
		t.Fatal(stats, err)
	}
}

func TestNormalRunCancelledBlessingReadDoesNotDraw(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	runID := entry.Command.RunID
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &normalBlessings{cancel: cancel}
	f.cfg.Blessings = b
	f.restart(t)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	beforeRNG := f.rng.calls.Load()
	c := f.actors[0]
	cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "cancelled", Version: r.Version, Action: "advance"}
	if _, err := f.svc.Command(ctx, cmd); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after, err := f.store.FindRun(context.Background(), runID)
	if err != nil || !reflect.DeepEqual(r, after) || f.rng.calls.Load() != beforeRNG {
		t.Fatal("cancelled read changed state or RNG", err)
	}
}

func TestNormalRunAdvanceUsesExecutingActorAndRequiredReads(t *testing.T) {
	for _, memberWish := range []bool{false, true} {
		t.Run(fmt.Sprint(memberWish), func(t *testing.T) {
			f := newNormalFixture(t, 2)
			_, lobby := f.waitingParty(t, "normal")
			leader, member := f.actors[0], f.actors[1]
			entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
			if err != nil {
				t.Fatal(err)
			}
			runID := entry.Command.RunID
			f.send(t, runID, 0, "advance", "")
			f.send(t, runID, 0, "kill", "")
			f.send(t, runID, 1, "merchant", "")
			f.send(t, runID, 1, "travel10", "")
			b := &normalBlessings{values: map[string]string{}, err: errors.New("blessing storage unavailable")}
			if memberWish {
				b.values[member.ID] = "4"
			} else {
				b.values[leader.ID] = "4"
			}
			f.cfg.Blessings = b
			f.restart(t)
			r, err := f.store.FindRun(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			f.clock.add(3 * time.Second)
			cmd := adventure.RunCommand{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: "treasure", Version: r.Version, Action: "advance"}
			beforeRNG := f.rng.calls.Load()
			if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, b.err) {
				t.Fatal(err)
			}
			after, err := f.store.FindRun(context.Background(), runID)
			if err != nil || !reflect.DeepEqual(r, after) || f.rng.calls.Load() != beforeRNG {
				t.Fatal("failed blessing read changed run or RNG", err)
			}
			if receipt, err := f.svc.GetRunReceipt(context.Background(), member.PlayerID, member.ID, runID, cmd.ID); err != nil || receipt != nil {
				t.Fatal(receipt, err)
			}
			b.err = nil
			result, err := f.svc.Command(context.Background(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			after, err = f.store.FindRun(context.Background(), runID)
			want := 3
			if memberWish {
				want++
			}
			if err != nil || len(after.Boxes) != want || b.actor != member.ID {
				t.Fatal("borrowed another participant's bonus", after, err)
			}
			beforeRNG = f.rng.calls.Load()
			if retry, err := f.svc.Command(context.Background(), cmd); err != nil || !reflect.DeepEqual(result, retry) || f.rng.calls.Load() != beforeRNG {
				t.Fatal("committed advance rerolled", retry, err)
			}
		})
	}
}

func TestNormalRunActivityClassificationAndBoundaries(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	runID := entry.Command.RunID
	c := f.actors[0]
	for _, tc := range []struct {
		action          string
		activity, actor bool
	}{
		{"comment", true, false}, {"unknown_text", true, false}, {"ignored_skill", true, false}, {"whisper", true, false}, {"no_effect", true, true}, {"screenshot", false, false}, {"invite", false, false}, {"reject", false, false},
	} {
		t.Run(tc.action, func(t *testing.T) {
			before, err := f.store.FindRun(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			f.clock.add(4 * time.Second)
			cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: before.Version, Action: tc.action}
			_, err = f.svc.Command(context.Background(), cmd)
			if (err != nil) != (tc.action == "reject") {
				t.Fatal(err)
			}
			after, readErr := f.store.FindRun(context.Background(), runID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.activity != !before.LastActivity.Equal(after.LastActivity) || tc.actor != !before.LastActions[c.ID].Equal(after.LastActions[c.ID]) {
				t.Fatal("activity mismatch", before.LastActivity, after.LastActivity)
			}
			if tc.action == "reject" && !reflect.DeepEqual(before, after) {
				t.Fatal("rejection mutated state")
			}
			if tc.action != "reject" {
				f.clock.add(time.Second)
				if _, err := f.svc.Command(context.Background(), cmd); err != nil {
					t.Fatal(err)
				}
				retry, err := f.store.FindRun(context.Background(), runID)
				if err != nil || !reflect.DeepEqual(after, retry) {
					t.Fatal("retry renewed activity", err)
				}
			}
		})
	}
	first := f.send(t, runID, 0, "advance", "")
	cmd := first.Command
	cmd.ID, cmd.Version, cmd.Action = "exit-boundary", first.Version, "exit"
	f.clock.add(3*time.Second - time.Nanosecond)
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, adventure.ErrRunCooldown) {
		t.Fatal("before interval boundary accepted", err)
	}
	f.clock.add(time.Nanosecond)
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal("exact interval boundary rejected", err)
	}
}

func TestNormalRunPairedCommandExitAndCleanupStress(t *testing.T) {
	for _, action := range []string{"exit", "cleanup"} {
		t.Run(action, func(t *testing.T) {
			f := newNormalFixture(t, 2)
			_, lobby := f.waitingParty(t, "normal")
			c := f.actors[0]
			entry, err := f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, lobby)
			if err != nil {
				t.Fatal(err)
			}
			runID := entry.Command.RunID
			f.send(t, runID, 0, "earn", "")
			if action == "cleanup" {
				f.clock.add(1801 * time.Second)
			} else {
				f.clock.add(3 * time.Second)
			}
			r, err := f.store.FindRun(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "race-exit", Version: r.Version, Action: action}
			other := f.actors[1]
			compete := adventure.RunCommand{PlayerID: other.PlayerID, ActorID: other.ID, RunID: runID, ID: "race-command", Version: r.Version, Action: "no_effect"}
			stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				request := cmd
				if (worker+op)%2 == 0 {
					request = compete
				}
				_, err := f.svc.Command(context.Background(), request)
				if errors.Is(err, adventure.ErrRunConflict) || errors.Is(err, adventure.ErrNotFound) || errors.Is(err, adventure.ErrRunForbidden) || errors.Is(err, adventure.ErrRunRecovery) {
					return nil
				}
				return err
			})
			if stats.Failures != 0 {
				t.Fatal(stats)
			}
			state, err := f.store.FindRun(context.Background(), runID)
			if err != nil {
				t.Fatal(err)
			}
			for _, before := range f.actors {
				after, err := f.chars.FindByID(context.Background(), before.ID)
				if err != nil {
					t.Fatal(err)
				}
				var recipient adventure.RunRecipient
				if state != nil {
					recipient = state.Recipients[before.ID]
				} else {
					facts, err := f.svc.GetActorPenalty(context.Background(), before.PlayerID, before.ID)
					if err != nil || facts == nil {
						t.Fatal(facts, err)
					}
					recipient = facts.Recipient
				}
				if after.Money-before.Money+recipient.Pending.Gold+recipient.Conditional.Gold != 5 {
					t.Fatal("asset conservation failed", after, err)
				}
				history, err := f.history.GetAggregatedStats(context.Background(), before.ID)
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if recipient.Disposition != "" {
					want = 1
				}
				if history.TotalAdventures != want {
					t.Fatal("history repeated or missing", history)
				}
			}
		})
	}
}
