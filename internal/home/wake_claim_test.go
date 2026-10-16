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

// Issue #1229: Sleep must be rejected while Wake is in progress, and an older
// Wake must never clear a newly accepted sleep cycle's recovery obligation.
func TestSleep_RejectedDuringWakeAndPreservesSubsequentSleepCycle(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	svc, repo := newWakeTestService(t, tm, map[string]corecharacter.Character{
		"c1": {ID: "c1", Name: "Hero", Tired: 70, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}},
	})

	// 1. Start first sleep cycle and simulate duration expiry.
	if _, err := svc.Sleep(ctx, "c1", "c1"); err != nil {
		t.Fatalf("first Sleep: %v", err)
	}
	if err := tm.ReleaseLock(ctx, timer.CategorySleep, "c1"); err != nil {
		t.Fatalf("ReleaseLock(CategorySleep): %v", err)
	}

	// 2. While PendingWake/CategoryAsleep remains before Wake, another Sleep is rejected.
	if _, err := svc.Sleep(ctx, "c1", "c1"); !errors.Is(err, ErrAlreadySleeping) {
		t.Fatalf("Sleep while PendingWake pending err = %v, want ErrAlreadySleeping", err)
	}

	// 3. Start first Wake and pause inside recovery hook.
	hook := &blockingCostume{entered: make(chan struct{}), release: make(chan struct{})}
	svc.SetCostumeResetter(hook)

	firstWake := make(chan error, 1)
	go func() {
		_, err := svc.Wake(ctx, "c1")
		firstWake <- err
	}()
	select {
	case <-hook.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first Wake never entered costume hook")
	}

	// 4. Sleep during in-flight Wake must be rejected with ErrWakeInProgress.
	if _, err := svc.Sleep(ctx, "c1", "c1"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("Sleep during Wake err = %v, want ErrWakeInProgress", err)
	}

	// 5. Let first Wake complete.
	close(hook.release)
	select {
	case err := <-firstWake:
		if err != nil {
			t.Fatalf("first Wake failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Wake did not finish")
	}

	// 6. Start a new sleep cycle after first Wake completes; injure character first to verify 2nd wake recovery.
	svc.SetCostumeResetter(nil)
	c := repo.chars["c1"]
	c.Stats.HP = 20
	c.Stats.MP = 10
	c.Tired = 45
	repo.chars["c1"] = c

	if _, err := svc.Sleep(ctx, "c1", "c1"); err != nil {
		t.Fatalf("second Sleep after Wake: %v", err)
	}
	if !repo.chars["c1"].PendingWake {
		t.Fatalf("expected PendingWake=true after second Sleep")
	}

	// 7. After second sleep duration expires, CanWake must be true and second Wake must recover vitality.
	if err := tm.ReleaseLock(ctx, timer.CategorySleep, "c1"); err != nil {
		t.Fatalf("ReleaseLock(CategorySleep) second cycle: %v", err)
	}
	status, err := svc.GetSleepStatus(ctx, "c1")
	if err != nil || !status.CanWake {
		t.Fatalf("status after second Sleep expiry = %+v, err = %v; want CanWake=true", status, err)
	}
	res, err := svc.Wake(ctx, "c1")
	if err != nil || !res.Success {
		t.Fatalf("second Wake = %+v, %v", res, err)
	}
	got := repo.chars["c1"]
	if got.Stats.HP != 100 || got.Stats.MP != 50 || got.Tired != 0 || got.PendingWake {
		t.Fatalf("second Wake state = %+v; want full vitality and PendingWake=false", got)
	}
}

// Defense-in-depth: if an active CategorySleep lock appears while Wake is in a
// hook, Wake must abort finalization with ErrStillSleeping rather than clearing
// PendingWake, CategoryAsleep, or dungeon_once.
func TestWake_AbortsFinalizationIfNewSleepTimerActive(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	svc, repo := newWakeTestService(t, tm, map[string]corecharacter.Character{
		"c1": {ID: "c1", Name: "Hero", Tired: 50, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
	})
	if err := tm.SetLock(ctx, timer.CategoryAsleep, "c1", time.Hour); err != nil {
		t.Fatalf("SetLock(CategoryAsleep): %v", err)
	}
	if err := tm.SetLock(ctx, timer.CategoryDungeonOnce, "c1", time.Hour); err != nil {
		t.Fatalf("SetLock(CategoryDungeonOnce): %v", err)
	}

	svc.SetCostumeResetter(costumeResetFunc(func(hookCtx context.Context, id string) error {
		// Simulate a new sleep cycle becoming active during hook execution.
		return tm.SetLock(hookCtx, timer.CategorySleep, id, time.Minute)
	}))

	_, err := svc.Wake(ctx, "c1")
	if !errors.Is(err, ErrStillSleeping) {
		t.Fatalf("Wake err = %v, want ErrStillSleeping", err)
	}
	if !repo.chars["c1"].PendingWake {
		t.Fatalf("expected PendingWake=true preserved when new sleep cycle is active")
	}
	if asleep, _ := tm.IsLocked(ctx, timer.CategoryAsleep, "c1"); !asleep {
		t.Fatalf("expected CategoryAsleep lock preserved when new sleep cycle is active")
	}
	if dungeonLocked, _ := tm.IsLocked(ctx, timer.CategoryDungeonOnce, "c1"); !dungeonLocked {
		t.Fatalf("expected CategoryDungeonOnce lock preserved when new sleep cycle is active")
	}
}

type costumeResetFunc func(context.Context, string) error

func (f costumeResetFunc) ResetCostume(ctx context.Context, id string) error { return f(ctx, id) }
