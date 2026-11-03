package adventure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestNormalRunPairedChestClaim(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	a := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.send(t, runID, 0, "travel10", "")
	f.send(t, runID, 0, "advance", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Boxes) == 0 {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		c := f.actors[(worker+op)%2]
		_, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "claim-" + c.ID, Version: r.Version, Action: "chest", TargetID: r.Boxes[0].ID})
		if errors.Is(err, adventure.ErrRunConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	total, inspections := 0, 0
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range f.actors {
		inv, err := f.invs.FindByCharacterID(context.Background(), c.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range inv.Items {
			total += item.Quantity
		}
		inspections += r.Recipients[c.ID].Inspected
	}
	if total != 1 || inspections != 1 || r.Boxes[0].OpenedBy == "" {
		t.Fatal("competing claims duplicated a chest", total, inspections)
	}
}
