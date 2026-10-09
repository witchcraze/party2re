package home

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
)

func newWakeTestService(t *testing.T, tm TimerService, chars map[string]corecharacter.Character) (*Service, *mockCharRepo) {
	t.Helper()
	repo := &mockCharRepo{chars: chars}
	svc, err := NewService(newMockHomeRepo(), repo, WithTimer(tm), WithCharacterUpdater(repo))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, repo
}

// Issue #1118: PendingWake flag in SQL ensures that even after long absence
// where Valkey keys expire, recovery is never lost and completes on next Wake.
func TestSleep_PendingWakePersistedAndSurvivesValkeyExpiry(t *testing.T) {
	ctx := context.Background()
	mt := &mockTimer{}
	svc, repo := newWakeTestService(t, mt, map[string]corecharacter.Character{
		"c1": {ID: "c1", Name: "Hero", Tired: 99, Stats: corecharacter.Stats{HP: 1, MaxHP: 100, MP: 1, MaxMP: 50}},
	})

	if _, err := svc.Sleep(ctx, "c1", "c1"); err != nil {
		t.Fatalf("Sleep: %v", err)
	}

	// Verify PendingWake is persisted in repository
	if !repo.chars["c1"].PendingWake {
		t.Fatalf("expected PendingWake=true in repository after Sleep")
	}

	// Simulate complete expiry of Valkey ephemeral timers (e.g. 24h+ absence)
	delete(mt.locks, timer.CategorySleep+":c1")
	delete(mt.locks, timer.CategoryAsleep+":c1")

	// GetSleepStatus must still recognize CanWake=true due to SQL PendingWake
	status, err := svc.GetSleepStatus(ctx, "c1")
	if err != nil || !status.CanWake {
		t.Fatalf("status = %+v, err = %v; want CanWake=true despite Valkey expiry", status, err)
	}

	// Wake must successfully recover vitality and clear PendingWake in SQL
	res, err := svc.Wake(ctx, "c1")
	if err != nil || !res.Success {
		t.Fatalf("Wake = %+v, %v", res, err)
	}
	got := repo.chars["c1"]
	if got.Stats.HP != 100 || got.Stats.MP != 50 || got.Tired != 0 {
		t.Fatalf("vitality not recovered: %+v tired=%d", got.Stats, got.Tired)
	}
	if got.PendingWake {
		t.Fatalf("expected PendingWake=false after successful Wake")
	}
}

type blockingCostume struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (b *blockingCostume) ResetCostume(ctx context.Context, _ string) error {
	if b.calls.Add(1) == 1 {
		close(b.entered)
	}
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Concurrent Wake must be serialized; concurrent attempt receives ErrWakeInProgress.
func TestWake_ConcurrentWakeIsSerialized(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	svc, _ := newWakeTestService(t, tm, map[string]corecharacter.Character{
		"c1": {ID: "c1", Name: "Hero", Tired: 50, Stats: corecharacter.Stats{HP: 1, MaxHP: 100}, PendingWake: true},
	})
	hook := &blockingCostume{entered: make(chan struct{}), release: make(chan struct{})}
	svc.SetCostumeResetter(hook)

	first := make(chan error, 1)
	go func() {
		_, err := svc.Wake(ctx, "c1")
		first <- err
	}()
	select {
	case <-hook.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first Wake never reached the recovery hook")
	}

	// Concurrent Wake during hook execution must be rejected with ErrWakeInProgress
	if _, err := svc.Wake(ctx, "c1"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("concurrent Wake err = %v, want ErrWakeInProgress", err)
	}

	close(hook.release)
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("first Wake: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Wake did not finish")
	}

	// Post-completion Wake is idempotent and reports already awake
	res, err := svc.Wake(ctx, "c1")
	if err != nil || !res.Success {
		t.Fatalf("post-completion Wake = %+v, %v", res, err)
	}
	if n := hook.calls.Load(); n != 1 {
		t.Fatalf("recovery hook ran %d times, want 1", n)
	}
}

func TestWake_HookFailureRetainsPendingWakeInSQL(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	fullness := &mockFullnessResetter{err: errors.New("transient tavern error")}
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 60, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	svc, err := NewService(newMockHomeRepo(), repo, WithTimer(tm), WithCharacterUpdater(repo), WithFullnessResetter(fullness))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Attempt 1 fails at hook
	_, err = svc.Wake(ctx, "c1")
	if err == nil {
		t.Fatalf("expected error on attempt 1")
	}

	// Verify PendingWake is NOT cleared in SQL
	if !repo.chars["c1"].PendingWake {
		t.Fatalf("expected PendingWake=true retained in SQL after hook failure")
	}

	// Error clears; Attempt 2 succeeds
	fullness.err = nil
	res, err := svc.Wake(ctx, "c1")
	if err != nil || !res.Success {
		t.Fatalf("attempt 2 Wake failed: %v", err)
	}
	if repo.chars["c1"].PendingWake {
		t.Fatalf("expected PendingWake=false cleared after attempt 2 succeeds")
	}
}
