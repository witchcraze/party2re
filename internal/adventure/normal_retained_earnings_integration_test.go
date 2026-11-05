package adventure_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// timeoutParty runs a two-actor party in which actor 0's earning command leaves
// unpaid stock for actor 1, then lets the room idle out and recovers the timers.
func (f *normalFixture) timeoutParty(t *testing.T, mode string) string {
	t.Helper()
	_, lobby := f.waitingParty(t, mode)
	leader, member := f.actors[0], f.actors[1]
	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.send(t, runID, 0, "earn", "")
	f.clock.add(1801 * time.Second)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	cmd := adventure.RunCommand{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: id.New(), Version: r.Version, Action: "cleanup"}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if r, err := f.store.FindRun(context.Background(), runID); err != nil || r != nil {
		t.Fatal("timeout retained active state", r, err)
	}
	return runID
}

func (f *normalFixture) storedRecipient(t *testing.T, runID, actor string) adventure.RunRecipient {
	t.Helper()
	var data []byte
	if err := f.db.QueryRow(`SELECT recipient_json FROM normal_adventure_recipients WHERE run_id=? AND character_id=?`, runID, actor).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var p adventure.RunRecipient
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// wake clears the post-timeout sleep for every actor after the penalty hour.
func (f *normalFixture) wake(t *testing.T) {
	t.Helper()
	f.clock.add(time.Hour)
	for _, a := range f.actors {
		c, err := f.chars.FindByID(context.Background(), a.ID)
		if err != nil {
			t.Fatal(err)
		}
		c.PendingWake = false
		if err := f.chars.Update(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
}

// nextNormalRun enrolls actor 1 in a fresh solo normal run after waking.
func (f *normalFixture) nextNormalRun(t *testing.T) string {
	t.Helper()
	f.wake(t)
	c := f.actors[1]
	r, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, id.New(), adventure.StarterAdventure, "normal", 3)
	if err != nil {
		t.Fatal(err)
	}
	return r.Command.RunID
}

func (f *normalFixture) claimCommand(t *testing.T, runID, cmdID string) adventure.RunCommand {
	t.Helper()
	f.clock.add(3 * time.Second)
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	c := f.actors[1]
	return adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: cmdID, Version: r.Version, Action: "no_effect"}
}

func TestNormalRunTimeoutEarningsClaimedOnceAfterDisposal(t *testing.T) {
	f := newNormalFixture(t, 2)
	member := f.actors[1]
	oldRun := f.timeoutParty(t, "normal")
	stored := f.storedRecipient(t, oldRun, member.ID)
	if stored.Pending != (adventure.RunEarnings{EXP: 3, Gold: 5, Crystals: 1}) || stored.TimerPending {
		t.Fatal(stored)
	}
	// Recovery and a disposed-room command neither lose nor grant the stock.
	for range 2 {
		if err := f.svc.RecoverTimeout(context.Background(), member.PlayerID, member.ID, oldRun); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: member.PlayerID, ActorID: member.ID, RunID: oldRun, ID: "late-exit", Version: 1, Action: "exit"}); !errors.Is(err, adventure.ErrNotFound) {
		t.Fatal(err)
	}
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money || c.Crystal != member.Crystal || c.Experience != member.Experience {
		t.Fatal("recovery granted retained stock", c, err)
	}

	newRun := f.nextNormalRun(t)
	cmd := f.claimCommand(t, newRun, "claim")
	first, err := f.svc.Command(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	assertClaimed := func() {
		t.Helper()
		c, err := f.chars.FindByID(context.Background(), member.ID)
		if err != nil || c.Money != member.Money+5 || c.Crystal != member.Crystal+1 || c.Experience != member.Experience+3 {
			t.Fatal("retained stock not credited exactly once", c, err)
		}
		p := f.storedRecipient(t, oldRun, member.ID)
		if p.Pending != (adventure.RunEarnings{}) || p.Applied != stored.Pending || p.Disposition != "timeout" || !p.PenaltyUntil.Equal(stored.PenaltyUntil) {
			t.Fatal("ledger did not move pending to applied without changing the penalty", p)
		}
	}
	assertClaimed()

	// Receipt replay, restart and a later releasing command are all no-ops for the old ledger.
	f.restart(t)
	if again, err := f.svc.Command(context.Background(), cmd); err != nil || again.Version != first.Version {
		t.Fatal(again, err)
	}
	next := f.claimCommand(t, newRun, "second")
	if _, err := f.svc.Command(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	assertClaimed()
}

func TestNormalRunMultipleTimeoutsClaimedTogether(t *testing.T) {
	f := newNormalFixture(t, 2)
	member := f.actors[1]
	first := f.timeoutParty(t, "normal")
	f.wake(t)
	second := f.timeoutParty(t, "normal")
	// A latest-penalty read alone cannot see the older unpaid balance.
	if facts, err := f.svc.GetActorPenalty(context.Background(), member.PlayerID, member.ID); err != nil || facts == nil || facts.RunID != second {
		t.Fatal(facts, err)
	}
	newRun := f.nextNormalRun(t)
	if _, err := f.svc.Command(context.Background(), f.claimCommand(t, newRun, "claim")); err != nil {
		t.Fatal(err)
	}
	c, err := f.chars.FindByID(context.Background(), member.ID)
	if err != nil || c.Money != member.Money+10 || c.Crystal != member.Crystal+2 || c.Experience != member.Experience+6 {
		t.Fatal(c, err)
	}
	for _, run := range []string{first, second} {
		if p := f.storedRecipient(t, run, member.ID); p.Pending != (adventure.RunEarnings{}) || p.Applied.Gold != 5 {
			t.Fatal(run, p)
		}
	}
}

func TestNormalRunRetainedClaimRollsBackWithCommand(t *testing.T) {
	f := newNormalFixture(t, 2)
	member := f.actors[1]
	oldRun := f.timeoutParty(t, "normal")
	newRun := f.nextNormalRun(t)
	cmd := f.claimCommand(t, newRun, "claim")
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("receipt failure accepted")
	}
	f.store.failWrite.Store(false)
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money {
		t.Fatal("failed command leaked assets", c, err)
	}
	if p := f.storedRecipient(t, oldRun, member.ID); p.Pending.Gold != 5 || p.Applied != (adventure.RunEarnings{}) {
		t.Fatal("failed command moved the ledger", p)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money+5 {
		t.Fatal(c, err)
	}
}

func TestNormalRunTimeoutTryStockStaysRetained(t *testing.T) {
	f := newNormalFixture(t, 2)
	member := f.actors[1]
	oldRun := f.timeoutParty(t, "try")
	newRun := f.nextNormalRun(t)
	if _, err := f.svc.Command(context.Background(), f.claimCommand(t, newRun, "claim")); err != nil {
		t.Fatal(err)
	}
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money {
		t.Fatal("conditional Try stock was paid", c, err)
	}
	if p := f.storedRecipient(t, oldRun, member.ID); p.Conditional.Gold != 5 || p.Pending != (adventure.RunEarnings{}) || p.Applied != (adventure.RunEarnings{}) {
		t.Fatal("conditional Try stock was changed", p)
	}
}

func TestNormalRunRetainedClaimConcurrentRetryAndRecovery(t *testing.T) {
	f := newNormalFixture(t, 2)
	member := f.actors[1]
	oldRun := f.timeoutParty(t, "normal")
	newRun := f.nextNormalRun(t)
	cmd := f.claimCommand(t, newRun, "claim")
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		if (worker+op)%2 == 0 {
			return f.svc.RecoverTimeout(context.Background(), member.PlayerID, member.ID, oldRun)
		}
		_, err := f.svc.Command(context.Background(), cmd)
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money+5 || c.Crystal != member.Crystal+1 {
		t.Fatal("duplicate or lost retained credit", c, err)
	}
}
