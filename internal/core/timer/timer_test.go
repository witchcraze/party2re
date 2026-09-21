package timer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
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
	initialRem, err := svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || initialRem != 0 {
		t.Fatalf("expected 0 remaining initially, got %v, err=%v", initialRem, err)
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
	ok, err = svc.ConsumeDailyQuota(ctx, action, charID, now.Add(1*time.Minute))
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

	// SetLock
	if err := svc.SetLock(ctx, "", "id", time.Second); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if err := svc.SetLock(ctx, "cat", "", time.Second); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// IsLocked
	if _, err := svc.IsLocked(ctx, "", "id"); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if _, err := svc.IsLocked(ctx, "cat", ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// GetRemainingLock
	if _, err := svc.GetRemainingLock(ctx, "", "id"); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if _, err := svc.GetRemainingLock(ctx, "cat", ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// ReleaseLock
	if err := svc.ReleaseLock(ctx, "", "id"); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("expected ErrInvalidCategory, got %v", err)
	}
	if err := svc.ReleaseLock(ctx, "cat", ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// ConsumeDailyQuota
	if _, err := svc.ConsumeDailyQuota(ctx, "", "id", time.Now()); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected ErrInvalidAction, got %v", err)
	}
	if _, err := svc.ConsumeDailyQuota(ctx, "act", "", time.Now()); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// HasUsedDailyQuota
	if _, err := svc.HasUsedDailyQuota(ctx, "", "id"); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected ErrInvalidAction, got %v", err)
	}
	if _, err := svc.HasUsedDailyQuota(ctx, "act", ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}

	// ResetDailyQuota
	if err := svc.ResetDailyQuota(ctx, "", "id"); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected ErrInvalidAction, got %v", err)
	}
	if err := svc.ResetDailyQuota(ctx, "act", ""); !errors.Is(err, ErrEmptyID) {
		t.Fatalf("expected ErrEmptyID, got %v", err)
	}
}

func TestMemoryTimer_ExpirationBranches(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	charID := "char-mem-exp"
	action := "daily-pray"

	// GetRemainingLock when key exists in memory but has already expired
	if err := svc.SetLock(ctx, CategorySleep, charID, -1*time.Second); err != nil {
		t.Fatalf("unexpected error setting expired lock: %v", err)
	}
	rem, err := svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || rem != 0 {
		t.Fatalf("expected 0 remaining for expired lock, got %v, err=%v", rem, err)
	}

	// ConsumeDailyQuota when key exists in memory but expired (subsequent day consumption)
	yesterday := time.Now().In(JST).AddDate(0, 0, -2)
	ok, err := svc.ConsumeDailyQuota(ctx, action, charID, yesterday)
	if err != nil || !ok {
		t.Fatalf("expected success for past quota: ok=%v, err=%v", ok, err)
	}
	// Consuming today should succeed because yesterday's quota expired
	today := time.Now().In(JST)
	ok, err = svc.ConsumeDailyQuota(ctx, action, charID, today)
	if err != nil || !ok {
		t.Fatalf("expected success for renewed day quota: ok=%v, err=%v", ok, err)
	}
}

func TestValkeyTimer_LockLifecycle(t *testing.T) {
	ctx := context.Background()
	charID := "char-valkey-123"
	expectedKey := PrefixTimer + CategorySleep + ":" + charID

	var lastCmd []string
	mock := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		name := cmd.Commands()[0]
		switch name {
		case "SET":
			return valkeytest.MakeOKResult()
		case "EXISTS":
			return valkeytest.MakeIntResult(1)
		case "TTL":
			return valkeytest.MakeIntResult(60)
		case "DEL":
			return valkeytest.MakeOKResult()
		default:
			return valkeytest.MakeOKResult()
		}
	}))
	svc := NewService(mock)

	// 1. SetLock with normal duration (>= 1s)
	if err := svc.SetLock(ctx, CategorySleep, charID, 60*time.Second); err != nil {
		t.Fatalf("unexpected error on SetLock: %v", err)
	}
	if len(lastCmd) < 5 || lastCmd[0] != "SET" || lastCmd[1] != expectedKey || lastCmd[2] != "1" || lastCmd[3] != "EX" || lastCmd[4] != "60" {
		t.Fatalf("unexpected SET command: %v", lastCmd)
	}

	// 2. SetLock with sub-second duration (secs < 1 boundary -> secs = 1)
	if err := svc.SetLock(ctx, CategorySleep, charID, 500*time.Millisecond); err != nil {
		t.Fatalf("unexpected error on SetLock boundary: %v", err)
	}
	if len(lastCmd) < 5 || lastCmd[4] != "1" {
		t.Fatalf("expected EX 1 for sub-second duration, got: %v", lastCmd)
	}

	// 3. IsLocked -> true
	locked, err := svc.IsLocked(ctx, CategorySleep, charID)
	if err != nil || !locked {
		t.Fatalf("expected locked=true, got %v, err=%v", locked, err)
	}
	if len(lastCmd) < 2 || lastCmd[0] != "EXISTS" || lastCmd[1] != expectedKey {
		t.Fatalf("unexpected EXISTS command: %v", lastCmd)
	}

	// 4. GetRemainingLock -> 60s
	rem, err := svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || rem != 60*time.Second {
		t.Fatalf("expected 60s remaining, got %v, err=%v", rem, err)
	}
	if len(lastCmd) < 2 || lastCmd[0] != "TTL" || lastCmd[1] != expectedKey {
		t.Fatalf("unexpected TTL command: %v", lastCmd)
	}

	// 5. ReleaseLock
	if err := svc.ReleaseLock(ctx, CategorySleep, charID); err != nil {
		t.Fatalf("unexpected error on ReleaseLock: %v", err)
	}
	if len(lastCmd) < 2 || lastCmd[0] != "DEL" || lastCmd[1] != expectedKey {
		t.Fatalf("unexpected DEL command: %v", lastCmd)
	}
}

func TestValkeyTimer_LockEdgeCases(t *testing.T) {
	ctx := context.Background()
	charID := "char-valkey-edge"

	// Test IsLocked false (EXISTS returns 0)
	mockNotLocked := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(0)
	}))
	svc := NewService(mockNotLocked)
	locked, err := svc.IsLocked(ctx, CategorySleep, charID)
	if err != nil || locked {
		t.Fatalf("expected locked=false, got %v, err=%v", locked, err)
	}

	// Test GetRemainingLock ttl <= 0 (negative TTL: missing or expired)
	mockTTLLessThanZero := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(-2)
	}))
	svc = NewService(mockTTLLessThanZero)
	rem, err := svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || rem != 0 {
		t.Fatalf("expected rem=0 for expired/missing key, got %v, err=%v", rem, err)
	}

	// Test GetRemainingLock ttl == 0
	mockTTLZero := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(0)
	}))
	svc = NewService(mockTTLZero)
	rem, err = svc.GetRemainingLock(ctx, CategorySleep, charID)
	if err != nil || rem != 0 {
		t.Fatalf("expected rem=0 for ttl=0, got %v, err=%v", rem, err)
	}
}

func TestValkeyTimer_LockErrors(t *testing.T) {
	ctx := context.Background()
	charID := "char-valkey-err"
	expectedErr := errors.New("valkey connection error")

	mockErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(expectedErr)
	}))
	svc := NewService(mockErr)

	if err := svc.SetLock(ctx, CategorySleep, charID, time.Minute); !errors.Is(err, expectedErr) {
		t.Fatalf("expected SetLock to return expectedErr, got %v", err)
	}
	if _, err := svc.IsLocked(ctx, CategorySleep, charID); !errors.Is(err, expectedErr) {
		t.Fatalf("expected IsLocked to return expectedErr, got %v", err)
	}
	if _, err := svc.GetRemainingLock(ctx, CategorySleep, charID); !errors.Is(err, expectedErr) {
		t.Fatalf("expected GetRemainingLock to return expectedErr, got %v", err)
	}
	if err := svc.ReleaseLock(ctx, CategorySleep, charID); !errors.Is(err, expectedErr) {
		t.Fatalf("expected ReleaseLock to return expectedErr, got %v", err)
	}
}

func TestValkeyTimer_DailyQuota(t *testing.T) {
	ctx := context.Background()
	charID := "char-daily-123"
	action := "chapel_pray"
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, JST)
	expectedKey := PrefixDaily + action + ":" + charID

	var lastCmd []string
	mock := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		lastCmd = cmd.Commands()
		name := cmd.Commands()[0]
		switch name {
		case "SET":
			return valkeytest.MakeOKResult()
		case "EXISTS":
			return valkeytest.MakeIntResult(1)
		case "DEL":
			return valkeytest.MakeOKResult()
		default:
			return valkeytest.MakeOKResult()
		}
	}))
	svc := NewService(mock)

	// 1. ConsumeDailyQuota success
	ok, err := svc.ConsumeDailyQuota(ctx, action, charID, now)
	if err != nil || !ok {
		t.Fatalf("expected success, got ok=%v, err=%v", ok, err)
	}
	// SET party2:daily:chapel_pray:char-daily-123 1 NX EX 43200
	if len(lastCmd) < 6 || lastCmd[0] != "SET" || lastCmd[1] != expectedKey || lastCmd[2] != "1" || lastCmd[3] != "NX" || lastCmd[4] != "EX" {
		t.Fatalf("unexpected SET NX command: %v", lastCmd)
	}

	// 2. Boundary: now right before midnight JST (< 1s -> secs = 1)
	almostMidnight := NextMidnightJST(now).Add(-500 * time.Millisecond)
	ok, err = svc.ConsumeDailyQuota(ctx, action, charID, almostMidnight)
	if err != nil || !ok {
		t.Fatalf("expected success, got ok=%v, err=%v", ok, err)
	}
	if len(lastCmd) < 6 || lastCmd[5] != "1" {
		t.Fatalf("expected EX 1 for sub-second remaining time, got: %v", lastCmd)
	}

	// 3. ConsumeDailyQuota when already consumed (Valkey returns Nil)
	mockNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	svcNil := NewService(mockNil)
	ok, err = svcNil.ConsumeDailyQuota(ctx, action, charID, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when quota already consumed")
	}

	// 4. HasUsedDailyQuota -> true
	used, err := svc.HasUsedDailyQuota(ctx, action, charID)
	if err != nil || !used {
		t.Fatalf("expected used=true, got %v, err=%v", used, err)
	}
	if len(lastCmd) < 2 || lastCmd[0] != "EXISTS" || lastCmd[1] != expectedKey {
		t.Fatalf("unexpected EXISTS command: %v", lastCmd)
	}

	// 5. HasUsedDailyQuota -> false
	mockNotUsed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntResult(0)
	}))
	svcNotUsed := NewService(mockNotUsed)
	used, err = svcNotUsed.HasUsedDailyQuota(ctx, action, charID)
	if err != nil || used {
		t.Fatalf("expected used=false, got %v, err=%v", used, err)
	}

	// 6. ResetDailyQuota
	if err := svc.ResetDailyQuota(ctx, action, charID); err != nil {
		t.Fatalf("unexpected error on ResetDailyQuota: %v", err)
	}
	if len(lastCmd) < 2 || lastCmd[0] != "DEL" || lastCmd[1] != expectedKey {
		t.Fatalf("unexpected DEL command: %v", lastCmd)
	}
}

func TestValkeyTimer_DailyQuotaErrors(t *testing.T) {
	ctx := context.Background()
	charID := "char-daily-err"
	action := "chapel_pray"
	expectedErr := errors.New("valkey daily error")

	mockErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, _ valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(expectedErr)
	}))
	svc := NewService(mockErr)

	if _, err := svc.ConsumeDailyQuota(ctx, action, charID, time.Now()); !errors.Is(err, expectedErr) {
		t.Fatalf("expected ConsumeDailyQuota to return expectedErr, got %v", err)
	}
	if _, err := svc.HasUsedDailyQuota(ctx, action, charID); !errors.Is(err, expectedErr) {
		t.Fatalf("expected HasUsedDailyQuota to return expectedErr, got %v", err)
	}
	if err := svc.ResetDailyQuota(ctx, action, charID); !errors.Is(err, expectedErr) {
		t.Fatalf("expected ResetDailyQuota to return expectedErr, got %v", err)
	}
}
