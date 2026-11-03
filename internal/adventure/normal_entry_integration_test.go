package adventure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/timer"
)

func TestNormalRunDailyAdmissionRollbackAndRetry(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := f.actors[0]
	c.Level = 80
	if err := f.chars.Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	f.actors[0] = c
	locks := timer.NewService(nil)
	f.cfg.EntryLocks = locks
	f.restart(t)
	f.store.failWrite.Store(true)
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "stage-26", "normal", 3); err == nil {
		t.Fatal("failed admission succeeded")
	}
	if locked, err := locks.IsLocked(context.Background(), timer.CategoryDungeonOnce, c.ID); err != nil || locked {
		t.Fatal("aborted admission retained daily claim", locked, err)
	}
	f.store.failWrite.Store(false)
	entry, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "stage-26", "normal", 3)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "daily", "stage-26", "normal", 3); err != nil {
		t.Fatal("committed daily admission could not be retried", err)
	}
	f.send(t, entry.Command.RunID, 0, "exit", "")
	for _, stage := range []string{"stage-26", " stage-26 "} {
		if _, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "another", stage, "normal", 3); !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
			t.Fatal(stage, err)
		}
	}
}

func TestNormalRunAdmissionSnapshotIsImmutable(t *testing.T) {
	f := newNormalFixture(t, 1)
	entry := f.enter(t, "normal")
	r, err := f.store.FindRun(context.Background(), entry.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	r.Admission.StageID = "stage-01"
	r.Version++
	err = f.store.WithRunForUpdate(context.Background(), r.ID, false, func(ctx context.Context) error { return f.store.SaveRun(ctx, *r, r.Version-1) })
	if !errors.Is(err, adventure.ErrRunConflict) {
		t.Fatal("admission snapshot changed", err)
	}
	if _, err := f.svc.EnterSolo(context.Background(), f.actors[0].PlayerID, f.actors[0].ID, "start", adventure.StarterAdventure, "normal", 3); err != nil {
		t.Fatal("original admission receipt lost", err)
	}
}
