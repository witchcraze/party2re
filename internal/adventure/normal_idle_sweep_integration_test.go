package adventure_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

// ListRuns keeps the sweep on this test's runs; the shared DB holds other tests' runs.
func (r *normalStore) ListRuns(ctx context.Context, kind string) ([]runhost.State, error) {
	runs, err := r.Store.ListRuns(ctx, kind)
	if err != nil || r.owned == nil {
		return runs, err
	}
	return slices.DeleteFunc(runs, func(s runhost.State) bool { return !r.owned[s.ID] }), nil
}

// sweepParty enrolls both actors in a Try party in which each holds 5 conditional gold.
func sweepParty(t *testing.T) (*normalFixture, string) {
	t.Helper()
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "try")
	c := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.store.owned = map[string]bool{runID: true}
	f.earn(t, runID, 0)
	return f, runID
}

func assertSweptRun(t *testing.T, f *normalFixture, deadline time.Time) {
	t.Helper()
	ctx := context.Background()
	for i, c := range f.actors {
		if active, err := f.svc.GetActorRun(ctx, c.PlayerID, c.ID); err != nil || active != nil {
			t.Fatalf("actor %d still enrolled: %+v %v", i, active, err)
		}
		facts, err := f.svc.GetActorPenalty(ctx, c.PlayerID, c.ID)
		if err != nil || facts == nil || facts.Recipient.Disposition != "timeout" || !facts.Recipient.PenaltyUntil.Equal(deadline) || facts.Recipient.TimerPending {
			t.Fatalf("actor %d penalty = %+v %v", i, facts, err)
		}
		if facts.Recipient.Conditional.Gold != 5 {
			t.Fatalf("retained stock lost: %+v", facts.Recipient)
		}
		after, err := f.chars.FindByID(ctx, c.ID)
		if err != nil || !after.PendingWake || after.Money != c.Money {
			t.Fatalf("actor %d after sweep: %+v %v", i, after, err)
		}
	}
	if got := time.Unix(0, f.penalty.deadline.Load()).UTC(); !got.Equal(deadline) {
		t.Fatalf("projected deadline = %v, want %v", got, deadline)
	}
}

// The sweep stands in for legacy's quest-list cleanup (quest.cgi:80–91): no member acts.
func TestNormalRunIdleSweep(t *testing.T) {
	f, runID := sweepParty(t)
	ctx := context.Background()

	f.clock.add(1800 * time.Second)
	if err := f.svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if r, err := f.store.FindRun(ctx, runID); err != nil || r == nil || r.TimedOut {
		t.Fatalf("run timed out at the inclusive 1800s boundary: %+v %v", r, err)
	}

	f.clock.add(time.Nanosecond)
	deadline := f.clock.Now().Add(time.Hour)
	if err := f.svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	assertSweptRun(t, f, deadline)

	calls := f.penalty.calls.Load()
	f.clock.add(time.Minute)
	if err := f.svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if f.penalty.calls.Load() != calls {
		t.Fatal("repeated sweep projected the penalty again")
	}
}

// A failed projection stays pending with its fixed deadline; the next tick completes it.
func TestNormalRunIdleSweepRetriesProjection(t *testing.T) {
	f, _ := sweepParty(t)
	ctx := context.Background()
	f.clock.add(1801 * time.Second)
	deadline := f.clock.Now().Add(time.Hour)
	f.penalty.fail.Store(true)
	if err := f.svc.SweepIdle(ctx); err == nil {
		t.Fatal("sweep hid the projection failure")
	}
	member := f.actors[1]
	if active, err := f.svc.GetActorRun(ctx, member.PlayerID, member.ID); err != nil || active == nil || !active.Recipient.TimerPending {
		t.Fatalf("failed projection not left pending: %+v %v", active, err)
	}
	f.penalty.fail.Store(false)
	f.clock.add(10 * time.Minute)
	if err := f.svc.SweepIdle(ctx); err != nil {
		t.Fatal(err)
	}
	assertSweptRun(t, f, deadline)
}

func TestNormalRunIdleSweepRacesCommands(t *testing.T) {
	for _, phase := range []string{"live", "overdue"} {
		t.Run(phase, func(t *testing.T) {
			f, runID := sweepParty(t)
			ctx := context.Background()
			compete := f.idleCommand(t, runID, 1)
			if phase == "live" {
				f.clock.add(1700 * time.Second)
			} else {
				f.clock.add(1801 * time.Second)
			}
			stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				var err error
				if (worker+op)%2 == 0 {
					err = f.svc.SweepIdle(ctx)
				} else {
					_, err = f.svc.Command(ctx, compete)
				}
				if errors.Is(err, runhost.ErrRecovery) || errors.Is(err, runhost.ErrNotFound) || errors.Is(err, runhost.ErrForbidden) {
					return nil // the sweep disposed the run before the command
				}
				return err
			})
			if stats.Failures != 0 {
				t.Fatal(stats)
			}
			receipt, err := f.store.FindRunReceipt(ctx, runID, compete.ID)
			if err != nil {
				t.Fatal(err)
			}
			r, err := f.store.FindRun(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			swept := r == nil
			if phase == "live" && (swept || receipt == nil) {
				t.Fatalf("live run swept or command lost: run=%v receipt=%v", r, receipt)
			}
			if swept == (receipt != nil) {
				t.Fatalf("want exactly one winner: swept=%v receipt=%v", swept, receipt)
			}
			for _, before := range f.actors {
				after, err := f.chars.FindByID(ctx, before.ID)
				if err != nil {
					t.Fatal(err)
				}
				var recipient runhost.Recipient
				if r != nil {
					recipient = r.Recipients[before.ID]
				} else if facts, err := f.svc.GetActorPenalty(ctx, before.PlayerID, before.ID); err != nil || facts == nil {
					t.Fatal(facts, err)
				} else {
					recipient = facts.Recipient
				}
				// The earned 5 gold is credited to every living ally (_battle.cgi:199–274).
				if after.Money-before.Money+recipient.Pending.Gold+recipient.Conditional.Gold != 5 {
					t.Fatalf("asset conservation failed for %s: %+v %+v", before.ID, after, recipient)
				}
			}
		})
	}
}
