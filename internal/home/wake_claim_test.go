package home

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/store"
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

func TestSleep_TimerSetFailurePreservesTemporaryJobMemoryAndRevertsPendingWake(t *testing.T) {
	ctx := context.Background()
	errTimerWrite := errors.New("valkey timer write failed")

	for _, useRunner := range []bool{false, true} {
		for _, failCategory := range []string{timer.CategorySleep, timer.CategoryAsleep} {
			mode := "char_updater"
			if useRunner {
				mode = "tx_runner"
			}
			t.Run(mode+"/"+failCategory, func(t *testing.T) {
				initialChar := corecharacter.Character{
					ID:       "c1",
					Name:     "Hero",
					JobID:    "warrior",
					SP:       10,
					OldJobID: "novice",
					OldSP:    5,
					JobMemory: &corecharacter.JobMemory{
						Kind:     corecharacter.JobMemoryKindTemporary,
						JobID:    "mage",
						SP:       20,
						OldJobID: "apprentice",
						OldSP:    15,
					},
				}
				chars := map[string]corecharacter.Character{"c1": initialChar}
				mt := &mockTimer{
					setLockErrs: map[string]error{failCategory: errTimerWrite},
				}
				initialJobState, _ := corejob.NewCharacterJob("c1", "warrior")
				jobRepo := &mockJobRepo{state: initialJobState}
				jobSvc, err := job.NewService(jobRepo)
				if err != nil {
					t.Fatalf("job.NewService: %v", err)
				}

				var svc *Service
				var charRepo *mockCharRepo
				var runner *mockTransactionRunner
				if useRunner {
					runner = &mockTransactionRunner{chars: chars}
					svc, err = NewService(newMockHomeRepo(chars), &mockCharReader{chars: chars}, WithTimer(mt), WithTransactionRunner(runner), WithJobStateRestorer(jobSvc))
				} else {
					charRepo = &mockCharRepo{chars: chars}
					svc, err = NewService(newMockHomeRepo(), charRepo, WithTimer(mt), WithCharacterUpdater(charRepo), WithJobStateRestorer(jobSvc))
				}
				if err != nil {
					t.Fatalf("NewService: %v", err)
				}

				_, err = svc.Sleep(ctx, "c1", "c1")
				if !errors.Is(err, errTimerWrite) {
					t.Fatalf("Sleep err = %v, want %v", err, errTimerWrite)
				}

				var got corecharacter.Character
				if useRunner {
					got = runner.chars["c1"]
				} else {
					got = charRepo.chars["c1"]
				}
				if got.PendingWake {
					t.Fatalf("expected PendingWake=false after timer write failure")
				}
				if got.JobID != "warrior" || got.SP != 10 {
					t.Fatalf("expected JobID=warrior SP=10 preserved, got JobID=%s SP=%d", got.JobID, got.SP)
				}
				if got.JobMemory == nil || got.JobMemory.JobID != "mage" || !got.JobMemory.IsTemporary() {
					t.Fatalf("expected temporary JobMemory (mage) preserved, got %+v", got.JobMemory)
				}
				if jobRepo.state.CurrentJobID != "warrior" {
					t.Fatalf("expected character_jobs.current_job_id=warrior preserved, got %s", jobRepo.state.CurrentJobID)
				}
				if locked, _ := mt.IsLocked(ctx, timer.CategorySleep, "c1"); locked {
					t.Fatalf("expected CategorySleep lock released")
				}
				if locked, _ := mt.IsLocked(ctx, timer.CategoryAsleep, "c1"); locked {
					t.Fatalf("expected CategoryAsleep lock released")
				}
				if locked, _ := mt.IsLocked(ctx, timer.CategoryWaking, "c1"); locked {
					t.Fatalf("expected CategoryWaking lock released")
				}
			})
		}
	}
}

func TestSleep_RollbackSucceedsOnCanceledContextAndSurfacesRollbackError(t *testing.T) {
	t.Run("canceled_context_still_rolls_back_pending_wake_and_locks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mt := &mockTimer{
			onSetLock: func(_ context.Context, category string) error {
				if category == timer.CategoryAsleep {
					cancel()
					return context.Canceled
				}
				return nil
			},
		}
		svc, repo := newWakeTestService(t, mt, map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero"},
		})

		_, err := svc.Sleep(ctx, "c1", "c1")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Sleep err = %v, want context.Canceled", err)
		}
		if repo.chars["c1"].PendingWake {
			t.Fatalf("expected PendingWake=false rolled back even after caller context cancellation")
		}
		checkCtx := context.Background()
		if locked, _ := mt.IsLocked(checkCtx, timer.CategorySleep, "c1"); locked {
			t.Fatalf("expected CategorySleep lock released even after caller context cancellation")
		}
		if locked, _ := mt.IsLocked(checkCtx, timer.CategoryWaking, "c1"); locked {
			t.Fatalf("expected CategoryWaking lock released even after caller context cancellation")
		}
	})

	t.Run("rollback_error_is_joined_and_surfaced", func(t *testing.T) {
		ctx := context.Background()
		errTimerWrite := errors.New("valkey sleep lock write failed")
		errRollbackDB := errors.New("sql pending_wake rollback failed")

		var repo *mockCharRepo
		mt := &mockTimer{
			onSetLock: func(_ context.Context, category string) error {
				if category == timer.CategorySleep {
					repo.updateErr = errRollbackDB
					return errTimerWrite
				}
				return nil
			},
		}
		var svc *Service
		svc, repo = newWakeTestService(t, mt, map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero"},
		})

		_, err := svc.Sleep(ctx, "c1", "c1")
		if !errors.Is(err, errTimerWrite) {
			t.Fatalf("Sleep err = %v, want errors.Is(errTimerWrite)", err)
		}
		if !errors.Is(err, errRollbackDB) {
			t.Fatalf("Sleep err = %v, want joined errors.Is(errRollbackDB)", err)
		}
	})
}

// Issue #1228: Keep-alive renewal extends the wake claim lease during slow recovery hooks.
func TestWake_KeepAliveRenewsLeaseDuringSlowHook(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	// Configure short TTL (60ms) and renewal interval (20ms)
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithWakeClaimTTL(60*time.Millisecond, 20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})
	var hookCalls atomic.Int32

	svc.SetCostumeResetter(costumeResetFunc(func(_ context.Context, _ string) error {
		hookCalls.Add(1)
		close(hookEntered)
		<-releaseHook
		return nil
	}))

	wakeDone := make(chan error, 1)
	go func() {
		_, err := svc.Wake(ctx, "c1")
		wakeDone <- err
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("hook never entered")
	}

	// Sleep for 150ms (> 2x TTL of 60ms) while hook is blocked.
	// The keeper background goroutine must actively renew the lock.
	time.Sleep(150 * time.Millisecond)

	// Claim must still be active and held
	locked, err := tm.IsLocked(ctx, timer.CategoryWaking, "c1")
	if err != nil {
		t.Fatalf("IsLocked: %v", err)
	}
	if !locked {
		t.Fatalf("expected waking claim still locked via keep-alive renewal")
	}

	// Concurrent Wake must still receive ErrWakeInProgress
	if _, err := svc.Wake(ctx, "c1"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("concurrent Wake err = %v, want ErrWakeInProgress", err)
	}

	// Release hook and let Wake finish
	close(releaseHook)
	select {
	case err := <-wakeDone:
		if err != nil {
			t.Fatalf("first Wake failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Wake timed out")
	}

	// After completion, claim must be released
	if locked, _ := tm.IsLocked(ctx, timer.CategoryWaking, "c1"); locked {
		t.Fatalf("expected waking claim released after completion")
	}
	if hookCalls.Load() != 1 {
		t.Fatalf("expected hook called exactly once, got %d", hookCalls.Load())
	}
}

// Issue #1228: If wake ownership is lost (e.g. stolen/overwritten by a successor cycle),
// the previous Wake must abort immediately, never delete successor's claim or quotas,
// and never finalize pending_wake in SQL.
func TestWake_AbortsImmediatelyWhenClaimOwnershipLost(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithWakeClaimTTL(50*time.Millisecond, 15*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})

	const successorToken = "successor-token-999"

	svc.SetCostumeResetter(costumeResetFunc(func(_ context.Context, id string) error {
		close(hookEntered)
		<-releaseHook

		// Simulate successor overwriting the claim with its own token
		// and acquiring a fresh dungeon_once lock and daily quota.
		_ = tm.ReleaseLock(ctx, timer.CategoryWaking, id)
		claimed, err := tm.TryLockWithToken(ctx, timer.CategoryWaking, id, successorToken, time.Hour)
		if err != nil || !claimed {
			t.Fatalf("successor claim setup failed: claimed=%v err=%v", claimed, err)
		}
		_ = tm.SetLock(ctx, timer.CategoryDungeonOnce, id, time.Hour)
		_, _ = tm.ConsumeDailyQuota(ctx, "dungeon_once", id, time.Now())
		return nil
	}))

	wakeDone := make(chan error, 1)
	go func() {
		_, err := svc.Wake(ctx, "c1")
		wakeDone <- err
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("hook never entered")
	}

	close(releaseHook)

	select {
	case err := <-wakeDone:
		if !errors.Is(err, ErrWakeInProgress) {
			t.Fatalf("Wake err = %v, want ErrWakeInProgress due to ownership loss", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wake timed out")
	}

	// 1. Successor's claim must NOT have been deleted by the aborted Wake's defer
	wakingLocked, err := tm.IsLocked(ctx, timer.CategoryWaking, "c1")
	if err != nil || !wakingLocked {
		t.Fatalf("successor waking lock must be retained, got locked=%v err=%v", wakingLocked, err)
	}
	// Verify successor token is still intact
	renewed, err := tm.RenewLock(ctx, timer.CategoryWaking, "c1", successorToken, time.Hour)
	if err != nil || !renewed {
		t.Fatalf("successor token must still own waking lock, got renewed=%v err=%v", renewed, err)
	}

	// 2. Successor's dungeon_once lock must NOT have been released
	dungeonLocked, _ := tm.IsLocked(ctx, timer.CategoryDungeonOnce, "c1")
	if !dungeonLocked {
		t.Fatalf("successor dungeon_once lock was erroneously released by aborted wake")
	}

	// 3. Successor's daily quota must NOT have been reset
	hasQuota, _ := tm.HasUsedDailyQuota(ctx, "dungeon_once", "c1")
	if !hasQuota {
		t.Fatalf("successor daily quota was reset, want true (still used)")
	}

	// 4. SQL PendingWake must NOT have been finalized to false
	if !repo.chars["c1"].PendingWake {
		t.Fatalf("SQL PendingWake was erroneously cleared by aborted wake")
	}
}

// Issue #1228: ReleaseLockWithToken must only delete the lock if the token matches.
func TestWake_ReleaseLockWithToken_MismatchDoesNotDelete(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)

	// Worker 1 acquires lock with token-1
	claimed, err := tm.TryLockWithToken(ctx, timer.CategoryWaking, "c1", "token-1", time.Minute)
	if err != nil || !claimed {
		t.Fatalf("TryLockWithToken: %v", err)
	}

	// Worker 2 attempts release with wrong token-2 -> should fail and NOT delete
	released, err := tm.ReleaseLockWithToken(ctx, timer.CategoryWaking, "c1", "token-2")
	if err != nil {
		t.Fatalf("ReleaseLockWithToken: %v", err)
	}
	if released {
		t.Fatalf("expected released=false for mismatched token")
	}

	// Lock must still exist
	locked, err := tm.IsLocked(ctx, timer.CategoryWaking, "c1")
	if err != nil || !locked {
		t.Fatalf("expected lock still held, got locked=%v", locked)
	}

	// Release with correct token-1 succeeds
	released, err = tm.ReleaseLockWithToken(ctx, timer.CategoryWaking, "c1", "token-1")
	if err != nil {
		t.Fatalf("ReleaseLockWithToken: %v", err)
	}
	if !released {
		t.Fatalf("expected released=true for matching token")
	}

	// Lock should now be deleted
	locked, err = tm.IsLocked(ctx, timer.CategoryWaking, "c1")
	if err != nil || locked {
		t.Fatalf("expected lock deleted, got locked=%v", locked)
	}
}

// P1 regression test: If claim is lost immediately before dungeon_once reset,
// ResetDungeonOnceWithClaim must atomically fail, leaving successor dungeon_once lock
// and quota intact, and halting Wake before SQL pending_wake or asleep lock are touched.
func TestWake_ClaimLostImmediatelyBeforeDungeonReset_AbortsWithoutAlteringState(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithWakeClaimTTL(1*time.Second, 200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	const successorToken = "successor-token-dungeon-guard"

	// During the costume hook (the last hook before dungeon reset), simulate
	// the successor taking over the waking claim and setting a fresh dungeon lock and quota.
	svc.SetCostumeResetter(costumeResetFunc(func(_ context.Context, id string) error {
		_ = tm.ReleaseLock(ctx, timer.CategoryWaking, id)
		claimed, err := tm.TryLockWithToken(ctx, timer.CategoryWaking, id, successorToken, time.Hour)
		if err != nil || !claimed {
			t.Fatalf("successor claim: %v", err)
		}
		_ = tm.SetLock(ctx, timer.CategoryDungeonOnce, id, time.Hour)
		_, _ = tm.ConsumeDailyQuota(ctx, "dungeon_once", id, time.Now())
		return nil
	}))

	_, err = svc.Wake(ctx, "c1")
	if !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("Wake err = %v, want ErrWakeInProgress", err)
	}

	// 1. Successor's dungeon_once lock must NOT have been released
	dungeonLocked, _ := tm.IsLocked(ctx, timer.CategoryDungeonOnce, "c1")
	if !dungeonLocked {
		t.Fatal("successor dungeon_once lock was erroneously deleted")
	}

	// 2. Successor's daily quota must NOT have been reset
	hasQuota, _ := tm.HasUsedDailyQuota(ctx, "dungeon_once", "c1")
	if !hasQuota {
		t.Fatal("successor daily quota was erroneously reset")
	}

	// 3. SQL PendingWake must remain true
	if !repo.chars["c1"].PendingWake {
		t.Fatal("SQL PendingWake was erroneously cleared")
	}

	// 4. Successor's waking lock must remain held
	wakingLocked, _ := tm.IsLocked(ctx, timer.CategoryWaking, "c1")
	if !wakingLocked {
		t.Fatal("successor waking lock was erroneously released")
	}
}

// Issue #1228: If a recovery hook pauses/blocks, loses claim ownership to a successor Wake,
// and the successor completes and player resumes play (modifying state), the resumed old hook
// must detect ownership loss and never corrupt the successor's refreshed state.
type blockingClaimCostumeResetter struct {
	inner       CostumeClaimResetter
	hookEntered chan struct{}
	releaseHook chan struct{}
}

func (b *blockingClaimCostumeResetter) ResetCostume(ctx context.Context, id string) error {
	return b.inner.ResetCostume(ctx, id)
}

func (b *blockingClaimCostumeResetter) ResetCostumeWithClaim(ctx context.Context, id, claimCategory, claimToken string) (bool, error) {
	close(b.hookEntered)
	<-b.releaseHook
	return b.inner.ResetCostumeWithClaim(ctx, id, claimCategory, claimToken)
}

// Issue #1228: If a recovery hook pauses/blocks, loses claim ownership to a successor Wake,
// and the successor completes and player resumes play (modifying state), the resumed old hook
// must detect ownership loss and never corrupt the successor's refreshed state.
func TestWake_HookPaused_OwnershipLossDoesNotCorruptSuccessorState(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithWakeClaimTTL(50*time.Millisecond, 15*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	costumeRepo := costume.NewMemoryCostumeRepository()
	storeSvc := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo), store.WithTimer(tm))

	claimResetter := &blockingClaimCostumeResetter{
		inner:       storeSvc,
		hookEntered: make(chan struct{}),
		releaseHook: make(chan struct{}),
	}
	svc.SetCostumeResetter(claimResetter)

	wakeDone := make(chan error, 1)
	go func() {
		_, err := svc.Wake(ctx, "c1")
		wakeDone <- err
	}()

	select {
	case <-claimResetter.hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("hook never entered")
	}

	// While old Wake hook is paused:
	// 1. Successor takes over claim
	const successorToken = "successor-token-final"
	_ = tm.ReleaseLock(ctx, timer.CategoryWaking, "c1")
	claimed, err := tm.TryLockWithToken(ctx, timer.CategoryWaking, "c1", successorToken, time.Hour)
	if err != nil || !claimed {
		t.Fatalf("successor claim setup failed: %v", err)
	}

	// 2. Successor completes wake: marks character awake in DB
	c := repo.chars["c1"]
	c.PendingWake = false
	c.Stats.HP = 100
	c.Tired = 0
	repo.chars["c1"] = c

	// 3. Player resumes play and rents a costume in timer quota and MemoryCostumeRepository
	used, err := tm.ConsumeDailyQuota(ctx, "costume", "c1", time.Now())
	if err != nil || !used {
		t.Fatalf("player failed to rent costume: %v", err)
	}
	err = costumeRepo.SaveActiveCostume(ctx, costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      46,
		ItemName:    "cat-ears",
		Icon:        "icon",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)
	if err != nil {
		t.Fatalf("failed to save successor active costume: %v", err)
	}

	// Now resume the old hook
	close(claimResetter.releaseHook)

	select {
	case err := <-wakeDone:
		if err == nil {
			t.Fatal("expected old Wake to abort with error after claim loss")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wake timed out")
	}

	// Verification: The player's post-wake state MUST NOT be corrupted by the old hook
	hasCostume, err := tm.HasUsedDailyQuota(ctx, "costume", "c1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCostume {
		t.Fatal("player's rented costume quota was erroneously deleted by old hook")
	}

	activeCostume, err := costumeRepo.GetActiveCostume(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if activeCostume == nil || activeCostume.ItemNo != 46 {
		t.Fatalf("player's active costume in MemoryCostumeRepository was erroneously deleted by old hook: %+v", activeCostume)
	}

	// Character must remain in awake state
	if repo.chars["c1"].PendingWake {
		t.Fatal("character PendingWake was reverted to true")
	}
}

// Issue #1228: In in-memory configuration with actual store.Service and MemoryCostumeRepository,
// successful Wake must reset daily quota and clear active costume from MemoryCostumeRepository.
func TestWake_CostumeClearedOnSuccessfulWake_MemoryRepository(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	costumeRepo := costume.NewMemoryCostumeRepository()
	storeSvc := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo), store.WithTimer(tm))

	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithCostumeResetter(storeSvc),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Pre-seed active costume and daily quota
	_, _ = tm.ConsumeDailyQuota(ctx, "costume", "c1", time.Now())
	_ = costumeRepo.SaveActiveCostume(ctx, costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      46,
		ItemName:    "cat-ears",
		Icon:        "icon",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)

	res, err := svc.Wake(ctx, "c1")
	if err != nil || !res.Success {
		t.Fatalf("Wake failed: %+v, %v", res, err)
	}

	// Verify both daily quota and MemoryCostumeRepository are cleared
	hasQuota, err := tm.HasUsedDailyQuota(ctx, "costume", "c1")
	if err != nil || hasQuota {
		t.Errorf("expected costume daily quota cleared, got %v (err=%v)", hasQuota, err)
	}
	active, err := costumeRepo.GetActiveCostume(ctx, "c1")
	if err != nil || active != nil {
		t.Errorf("expected MemoryCostumeRepository costume cleared, got %+v (err=%v)", active, err)
	}
}

// Issue #1228: If ownership cannot be verified atomically (e.g. timer is nil),
// Wake must refuse unsafe deletion rather than falling back to unconditional deletion.
func TestWake_CostumeRefusesUnsafeDeletionWhenTimerNil(t *testing.T) {
	ctx := context.Background()
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	costumeRepo := costume.NewMemoryCostumeRepository()
	storeSvc := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo)) // timer is nil

	tm := timer.NewService(nil)
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithCostumeResetter(storeSvc),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_ = costumeRepo.SaveActiveCostume(ctx, costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      46,
		ItemName:    "cat-ears",
		Icon:        "icon",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)

	_, err = svc.Wake(ctx, "c1")
	if !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("Wake err = %v, want ErrWakeInProgress (refuse unsafe deletion)", err)
	}

	// Active costume must remain intact in MemoryCostumeRepository
	active, err := costumeRepo.GetActiveCostume(ctx, "c1")
	if err != nil || active == nil || active.ItemNo != 46 {
		t.Fatalf("costume in MemoryCostumeRepository was erroneously deleted when timer was nil")
	}
}

type legacyCostumeHook struct {
	called atomic.Bool
}

func (l *legacyCostumeHook) ResetCostume(ctx context.Context, characterID string) error {
	l.called.Store(true)
	return nil
}

// Verifies that a legacy CostumeResetter (without CostumeClaimResetter) is safely called
// after atomic quota reset succeeds on normal Wake.
func TestWake_LegacyCostumeResetter_ExecutesOnValidWake(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	legacyHook := &legacyCostumeHook{}
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithCostumeResetter(legacyHook),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	res, err := svc.Wake(ctx, "c1")
	if err != nil || !res.Success {
		t.Fatalf("Wake failed: %+v, %v", res, err)
	}
	if !legacyHook.called.Load() {
		t.Fatal("expected legacy ResetCostume to be called on valid wake")
	}
}

// Issue #1228: Immediately after ownership verification, if processing is stopped,
// claim expires, successor Wake completes, and player registers a new costume,
// resuming the old processing must never delete the successor's newly registered costume.
func TestWake_PauseImmediatelyAfterOwnershipVerification_PreservesSuccessorNewCostume(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 80, Stats: corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50}, PendingWake: true},
		},
	}
	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(tm),
		WithCharacterUpdater(repo),
		WithWakeClaimTTL(50*time.Millisecond, 15*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	costumeRepo := costume.NewMemoryCostumeRepository()
	// Pre-seed initial costume (item 46)
	_ = costumeRepo.SaveActiveCostume(ctx, costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      46,
		ItemName:    "old-cat-ears",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)

	storeSvc := store.NewService(nil, nil, nil, nil, nil, store.WithCostumeRepository(costumeRepo), store.WithTimer(tm))

	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})

	// Intercept claim executor to pause processing before atomic execution
	costumeRepo.SetClaimExecutor(func(chkCtx context.Context, category, targetID, token string, action func() error) (bool, error) {
		// Stop processing before atomic execution
		close(hookEntered)
		<-releaseHook
		// Delegate directly to production ExecuteWithClaim without test-side re-verification
		return tm.ExecuteWithClaim(chkCtx, category, targetID, token, action)
	})

	svc.SetCostumeResetter(storeSvc)

	wakeDone := make(chan error, 1)
	go func() {
		_, err := svc.Wake(ctx, "c1")
		wakeDone <- err
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("hook never entered")
	}

	// 1. Claim expires; successor Wake takes over claim
	const successorToken = "successor-token-step3"
	_ = tm.ReleaseLock(ctx, timer.CategoryWaking, "c1")
	claimed, err := tm.TryLockWithToken(ctx, timer.CategoryWaking, "c1", successorToken, time.Hour)
	if err != nil || !claimed {
		t.Fatalf("successor claim setup failed: %v", err)
	}

	// 2. Successor completes wake
	c := repo.chars["c1"]
	c.PendingWake = false
	c.Stats.HP = 100
	c.Tired = 0
	repo.chars["c1"] = c

	// 3. Player resumes play and registers a NEW costume (item 99)
	err = costumeRepo.SaveActiveCostume(ctx, costume.ActiveCostume{
		CharacterID: "c1",
		ItemNo:      99,
		ItemName:    "new-dragon-wings",
		ExpiresAt:   time.Now().Add(time.Hour),
	}, time.Hour)
	if err != nil {
		t.Fatalf("failed to save successor new costume: %v", err)
	}

	// 4. Resume old processing
	close(releaseHook)

	select {
	case err := <-wakeDone:
		if err == nil {
			t.Fatal("expected old Wake to abort with error after claim loss")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wake timed out")
	}

	// 5. Verification: Successor's new costume (item 99) MUST NOT be deleted by the resumed old Wake!
	activeCostume, err := costumeRepo.GetActiveCostume(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if activeCostume == nil || activeCostume.ItemNo != 99 {
		t.Fatalf("successor's new costume (item 99) was corrupted by resumed old Wake: %+v", activeCostume)
	}
}

type interceptTimerService struct {
	TimerService
	onSetLock func(ctx context.Context, category string) error
}

func (i *interceptTimerService) SetLock(ctx context.Context, category, targetID string, duration time.Duration) error {
	if i.onSetLock != nil {
		if err := i.onSetLock(ctx, category); err != nil {
			return err
		}
	}
	return i.TimerService.SetLock(ctx, category, targetID, duration)
}

type jobRestoreHook func(ctx context.Context, characterID, jobID string) error

func (f jobRestoreHook) RestoreActiveJob(ctx context.Context, characterID, jobID string) error {
	return f(ctx, characterID, jobID)
}

// Issue #1450: Sleep must renew its waking claim lease during slow startup steps.
func TestSleep_KeepAliveRenewsLeaseDuringSlowStartup(t *testing.T) {
	ctx := context.Background()
	tm := timer.NewService(nil)
	repo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"c1": {ID: "c1", Name: "Hero", Tired: 40, Stats: corecharacter.Stats{HP: 10, MaxHP: 100}},
		},
	}

	enteredAsleep := make(chan struct{})
	releaseAsleep := make(chan struct{})
	it := &interceptTimerService{
		TimerService: tm,
		onSetLock: func(_ context.Context, category string) error {
			if category == timer.CategoryAsleep {
				close(enteredAsleep)
				<-releaseAsleep
			}
			return nil
		},
	}

	svc, err := NewService(
		newMockHomeRepo(),
		repo,
		WithTimer(it),
		WithCharacterUpdater(repo),
		WithWakeClaimTTL(60*time.Millisecond, 20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	sleepDone := make(chan error, 1)
	go func() {
		_, err := svc.Sleep(ctx, "c1", "c1")
		sleepDone <- err
	}()

	select {
	case <-enteredAsleep:
	case <-time.After(5 * time.Second):
		t.Fatal("Sleep never reached SetLock(CategoryAsleep)")
	}

	// Wait > 2x TTL while Sleep is paused
	time.Sleep(150 * time.Millisecond)

	if locked, err := tm.IsLocked(ctx, timer.CategoryWaking, "c1"); err != nil || !locked {
		t.Fatalf("expected CategoryWaking still locked via Sleep keep-alive renewal, got %v err=%v", locked, err)
	}

	// Concurrent Wake (if CategorySleep expired) or Sleep must be rejected with ErrWakeInProgress
	_ = tm.ReleaseLock(ctx, timer.CategorySleep, "c1")
	if _, err := svc.Wake(ctx, "c1"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("concurrent Wake err = %v, want ErrWakeInProgress", err)
	}
	if _, err := svc.Sleep(ctx, "c1", "c1"); !errors.Is(err, ErrWakeInProgress) {
		t.Fatalf("concurrent Sleep err = %v, want ErrWakeInProgress", err)
	}

	// Re-establish CategorySleep before letting first Sleep finish
	_ = tm.SetLock(ctx, timer.CategorySleep, "c1", time.Minute)
	close(releaseAsleep)

	select {
	case err := <-sleepDone:
		if err != nil {
			t.Fatalf("first Sleep failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Sleep timed out")
	}

	if locked, _ := tm.IsLocked(ctx, timer.CategoryWaking, "c1"); locked {
		t.Fatal("expected CategoryWaking released after Sleep completion")
	}
	if !repo.chars["c1"].PendingWake {
		t.Fatal("expected PendingWake=true after Sleep completion")
	}
}

// Issue #1450: When an earlier Sleep's waking claim expires while paused before SetLock(CategoryAsleep)
// or revertSleepJobMemory, and a successor Wake + second Sleep complete, resuming the earlier Sleep
// (whether the paused step errors or attempts to proceed) must NOT roll back or overwrite the
// successor cycle's SQL pending_wake, CategorySleep / CategoryAsleep locks, or temporary JobMemory.
func TestSleep_ExpiredClaimPreservesSuccessorSleepState(t *testing.T) {
	ctx := context.Background()

	for _, pausePoint := range []string{"asleep_error", "asleep_proceed", "job_revert_error", "job_revert_proceed"} {
		t.Run(pausePoint, func(t *testing.T) {
			tm := timer.NewService(nil)
			repo := &mockCharRepo{
				chars: map[string]corecharacter.Character{
					"c1": {
						ID:       "c1",
						Name:     "Hero",
						Tired:    60,
						Stats:    corecharacter.Stats{HP: 10, MaxHP: 100, MP: 5, MaxMP: 50},
						JobID:    "warrior",
						SP:       10,
						OldJobID: "novice",
						OldSP:    5,
						JobMemory: &corecharacter.JobMemory{
							Kind:     corecharacter.JobMemoryKindTemporary,
							JobID:    "mage",
							SP:       20,
							OldJobID: "apprentice",
							OldSP:    15,
						},
					},
				},
			}
			initialJobState, _ := corejob.NewCharacterJob("c1", "warrior")
			jobRepo := &mockJobRepo{state: initialJobState}
			jobSvc, err := job.NewService(jobRepo)
			if err != nil {
				t.Fatalf("job.NewService: %v", err)
			}

			paused := make(chan struct{})
			resume := make(chan struct{})
			var pauseOnce atomic.Bool

			it := &interceptTimerService{
				TimerService: tm,
				onSetLock: func(_ context.Context, category string) error {
					if (pausePoint == "asleep_error" || pausePoint == "asleep_proceed") &&
						category == timer.CategoryAsleep && pauseOnce.CompareAndSwap(false, true) {
						close(paused)
						<-resume
						if pausePoint == "asleep_error" {
							return errors.New("late asleep write failure")
						}
					}
					return nil
				},
			}

			restorer := jobRestoreHook(func(rCtx context.Context, characterID, jobID string) error {
				if (pausePoint == "job_revert_error" || pausePoint == "job_revert_proceed") &&
					pauseOnce.CompareAndSwap(false, true) {
					close(paused)
					<-resume
					if pausePoint == "job_revert_error" {
						return errors.New("late job restore failure")
					}
				}
				return jobSvc.RestoreActiveJob(rCtx, characterID, jobID)
			})

			svc1, err := NewService(
				newMockHomeRepo(),
				repo,
				WithTimer(it),
				WithCharacterUpdater(repo),
				WithJobStateRestorer(restorer),
				WithWakeClaimTTL(1*time.Second, 250*time.Millisecond),
			)
			if err != nil {
				t.Fatalf("NewService svc1: %v", err)
			}

			svc2, err := NewService(
				newMockHomeRepo(),
				repo,
				WithTimer(tm),
				WithCharacterUpdater(repo),
				WithJobStateRestorer(jobSvc),
			)
			if err != nil {
				t.Fatalf("NewService svc2: %v", err)
			}

			firstSleepDone := make(chan error, 1)
			go func() {
				_, err := svc1.Sleep(ctx, "c1", "c1")
				firstSleepDone <- err
			}()

			select {
			case <-paused:
			case <-time.After(5 * time.Second):
				t.Fatal("first Sleep never reached pause point")
			}

			// Simulate first Sleep's waking claim TTL and sleep duration expiring
			if err := tm.ReleaseLock(ctx, timer.CategoryWaking, "c1"); err != nil {
				t.Fatal(err)
			}
			if err := tm.ReleaseLock(ctx, timer.CategorySleep, "c1"); err != nil {
				t.Fatal(err)
			}

			// Successor Wake acquires a new waking claim and completes recovery
			wakeRes, err := svc2.Wake(ctx, "c1")
			if err != nil || !wakeRes.Success {
				t.Fatalf("successor Wake failed: %+v, %v", wakeRes, err)
			}

			// Player equips a new temporary job memory before 2nd Sleep, or enters 2nd Sleep
			c := repo.chars["c1"]
			c.Stats.HP = 25
			c.Tired = 40
			c.JobID = "paladin"
			c.SP = 30
			c.OldJobID = "knight"
			c.OldSP = 25
			c.JobMemory = &corecharacter.JobMemory{
				Kind:     corecharacter.JobMemoryKindTemporary,
				JobID:    "sage",
				SP:       50,
				OldJobID: "priest",
				OldSP:    40,
			}
			repo.chars["c1"] = c
			_ = jobSvc.RestoreActiveJob(ctx, "c1", "paladin")

			// Successor 2nd Sleep runs to completion
			if _, err := svc2.Sleep(ctx, "c1", "c1"); err != nil {
				t.Fatalf("second Sleep failed: %v", err)
			}

			// Verify 2nd Sleep established PendingWake=true, locks, and reverted temporary JobMemory to sage
			if !repo.chars["c1"].PendingWake {
				t.Fatal("expected PendingWake=true after second Sleep")
			}
			if repo.chars["c1"].JobID != "sage" || jobRepo.state.CurrentJobID != "sage" {
				t.Fatalf("expected 2nd Sleep to revert job to sage, got char=%s jobState=%s", repo.chars["c1"].JobID, jobRepo.state.CurrentJobID)
			}

			// Resume the paused 1st Sleep
			close(resume)
			select {
			case err := <-firstSleepDone:
				if err == nil {
					t.Fatal("expected first Sleep to fail after losing claim ownership")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("first Sleep timed out")
			}

			// Successor 2nd Sleep state must remain completely intact!
			got := repo.chars["c1"]
			if !got.PendingWake {
				t.Fatal("successor SQL PendingWake was erroneously cleared by stale first Sleep rollback")
			}
			if got.JobID != "sage" || got.SP != 50 || got.JobMemory != nil {
				t.Fatalf("successor character job state was corrupted by stale first Sleep: %+v (jobMemory=%+v)", got, got.JobMemory)
			}
			if jobRepo.state.CurrentJobID != "sage" {
				t.Fatalf("successor character_jobs.current_job_id was corrupted: got %s, want sage", jobRepo.state.CurrentJobID)
			}
			if rem, err := tm.GetRemainingLock(ctx, timer.CategorySleep, "c1"); err != nil || rem <= 0 {
				t.Fatalf("successor CategorySleep lock was deleted by stale first Sleep: rem=%v err=%v", rem, err)
			}
			if asleep, err := tm.IsLocked(ctx, timer.CategoryAsleep, "c1"); err != nil || !asleep {
				t.Fatalf("successor CategoryAsleep lock was deleted by stale first Sleep: asleep=%v err=%v", asleep, err)
			}

			// While 2nd Sleep timer is active, GetSleepStatus must report Sleeping=true, CanWake=false
			status, err := svc2.GetSleepStatus(ctx, "c1")
			if err != nil || !status.Sleeping || status.CanWake || status.RemainingSeconds <= 0 {
				t.Fatalf("GetSleepStatus during 2nd Sleep = %+v, err=%v; want Sleeping=true CanWake=false RemainingSeconds>0", status, err)
			}
		})
	}
}
