package timer

import (
	"context"
	"testing"
	"time"
)

func TestNextMidnightJST(t *testing.T) {
	// 2026-09-09 15:30:00 JST -> next midnight should be 2026-09-10 00:00:00 JST
	now := time.Date(2026, 9, 9, 15, 30, 0, 0, JST)
	next := NextMidnightJST(now)

	expected := time.Date(2026, 9, 10, 0, 0, 0, 0, JST)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}

	// 2026-09-09 23:59:59 JST -> next midnight should be 2026-09-10 00:00:00 JST
	late := time.Date(2026, 9, 9, 23, 59, 59, 0, JST)
	nextLate := NextMidnightJST(late)
	if !nextLate.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, nextLate)
	}
}

func TestMemoryTimer_LockLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	charID := "char-123"

	// Initially not locked
	locked, err := svc.IsLocked(ctx, CategorySleep, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locked {
		t.Fatal("expected not locked initially")
	}

	// Set lock for 100ms
	if err := svc.SetLock(ctx, CategorySleep, charID, 100*time.Millisecond); err != nil {
		t.Fatalf("failed to set lock: %v", err)
	}

	// Should be locked
	locked, err = svc.IsLocked(ctx, CategorySleep, charID)
	if err != nil || !locked {
		t.Fatalf("expected locked, got locked=%v, err=%v", locked, err)
	}

	rem, err := svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || rem <= 0 {
		t.Fatalf("expected positive remaining, got %v, err=%v", rem, err)
	}

	// Wait for expiry
	time.Sleep(120 * time.Millisecond)

	locked, err = svc.IsLocked(ctx, CategorySleep, charID)
	if err != nil || locked {
		t.Fatalf("expected expired, got locked=%v, err=%v", locked, err)
	}

	rem, err = svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || rem != 0 {
		t.Fatalf("expected 0 remaining, got %v, err=%v", rem, err)
	}

	// Set lock and release early
	if err := svc.SetLock(ctx, CategorySleep, charID, 10*time.Second); err != nil {
		t.Fatalf("failed to set lock: %v", err)
	}
	if err := svc.ReleaseLock(ctx, CategorySleep, charID); err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}
	locked, _ = svc.IsLocked(ctx, CategorySleep, charID)
	if locked {
		t.Fatal("expected lock to be released")
	}
}

func TestMemoryTimer_DailyQuota(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	charID := "char-456"
	action := "chapel_pray"
	now := time.Now().In(JST)

	// Initially not used
	used, err := svc.HasUsedDailyQuota(ctx, action, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used {
		t.Fatal("expected daily quota not used")
	}

	// First consumption succeeds
	ok, err := svc.ConsumeDailyQuota(ctx, action, charID, now)
	if err != nil || !ok {
		t.Fatalf("expected consumption success, got ok=%v, err=%v", ok, err)
	}

	// Second consumption on same day fails
	ok, err = svc.ConsumeDailyQuota(ctx, action, charID, now.Add(2*time.Hour))
	if err != nil || ok {
		t.Fatalf("expected consumption denied, got ok=%v, err=%v", ok, err)
	}

	// HasUsedDailyQuota returns true
	used, err = svc.HasUsedDailyQuota(ctx, action, charID)
	if err != nil || !used {
		t.Fatalf("expected used=true, got used=%v, err=%v", used, err)
	}

	// Reset quota
	if err := svc.ResetDailyQuota(ctx, action, charID); err != nil {
		t.Fatalf("failed to reset quota: %v", err)
	}
	used, _ = svc.HasUsedDailyQuota(ctx, action, charID)
	if used {
		t.Fatal("expected quota cleared after reset")
	}
}

func TestTimer_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	if err := svc.SetLock(ctx, "", "id", time.Second); err != ErrInvalidCategory {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if err := svc.SetLock(ctx, "cat", "", time.Second); err != ErrEmptyID {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}
	if _, err := svc.ConsumeDailyQuota(ctx, "", "id", time.Now()); err != ErrInvalidAction {
		t.Fatalf("expected ErrInvalidAction, got %v", err)
	}
	if _, err := svc.ConsumeDailyQuota(ctx, "act", "", time.Now()); err != ErrEmptyID {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}
}
