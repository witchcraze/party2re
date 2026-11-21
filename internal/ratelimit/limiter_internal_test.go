package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiter_LazyEvictsExpiredWithoutAffectingActive(t *testing.T) {
	now := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	lim := NewMemoryLimiter()
	lim.nowFunc = func() time.Time { return now }
	ctx := context.Background()

	// Seed one active key with a 5-minute window and consume 2 of 3 allowed calls.
	for i := 0; i < 2; i++ {
		res, err := lim.Allow(ctx, "active-key", 3, 5*time.Minute)
		if err != nil || !res.Allowed {
			t.Fatalf("active-key call %d failed: res=%+v err=%v", i, res, err)
		}
	}

	// Populate short-lived keys just below the sweep threshold so they expire before sweep triggers.
	for i := 0; i < memorySweepThreshold-1; i++ {
		res, err := lim.Allow(ctx, fmt.Sprintf("short-%d", i), 2, time.Second)
		if err != nil || !res.Allowed {
			t.Fatalf("short-%d failed: res=%+v err=%v", i, res, err)
		}
	}
	if got := len(lim.entries); got != memorySweepThreshold {
		t.Fatalf("expected %d entries before sweep, got %d", memorySweepThreshold, got)
	}

	// Advance past the 1s window (but well under 1m) and trigger a sweep via a new key.
	now = now.Add(2 * time.Second)
	if res, err := lim.Allow(ctx, "trigger", 5, time.Minute); err != nil || !res.Allowed {
		t.Fatalf("trigger failed: res=%+v err=%v", res, err)
	}

	// All expired short-* entries should be evicted while active-key and trigger remain.
	if got := len(lim.entries); got != 2 {
		t.Fatalf("expected 2 entries after lazy sweep, got %d", got)
	}

	// Active key must keep its exact remaining quota and then block on the 4th call.
	res3, err := lim.Allow(ctx, "active-key", 3, 5*time.Minute)
	if err != nil || !res3.Allowed || res3.Remaining != 0 {
		t.Fatalf("active-key 3rd call: got res=%+v err=%v", res3, err)
	}
	res4, err := lim.Allow(ctx, "active-key", 3, 5*time.Minute)
	if err != nil || res4.Allowed || res4.Remaining != 0 {
		t.Fatalf("active-key 4th call should be blocked: got res=%+v err=%v", res4, err)
	}

	// Re-fill up to threshold again within <1m and verify a second sweep runs once threshold is reached.
	for i := 0; i < memorySweepThreshold-2; i++ {
		if _, err := lim.Allow(ctx, fmt.Sprintf("burst-%d", i), 2, time.Second); err != nil {
			t.Fatalf("burst-%d failed: %v", i, err)
		}
	}
	now = now.Add(2 * time.Second)
	if _, err := lim.Allow(ctx, "trigger-2", 5, time.Minute); err != nil {
		t.Fatalf("trigger-2 failed: %v", err)
	}
	if got := len(lim.entries); got != 3 {
		t.Fatalf("expected 3 entries after second lazy sweep (active-key, trigger, trigger-2), got %d", got)
	}
}

func TestMemoryLimiter_LazyEvictionConcurrent(t *testing.T) {
	now := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	lim := NewMemoryLimiter()
	lim.nowFunc = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	ctx := context.Background()

	// Seed expired entries before concurrent burst.
	for i := 0; i < memorySweepThreshold; i++ {
		if _, err := lim.Allow(ctx, fmt.Sprintf("stale-%d", i), 2, time.Second); err != nil {
			t.Fatalf("stale-%d failed: %v", i, err)
		}
	}
	clockMu.Lock()
	now = now.Add(2 * time.Second)
	clockMu.Unlock()

	const sharedLimit = 15
	const workers = 40
	var wg sync.WaitGroup
	var sharedAllowed int
	var countMu sync.Mutex

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := lim.Allow(ctx, fmt.Sprintf("fresh-%d", i), 5, time.Minute); err != nil {
				t.Errorf("fresh-%d failed: %v", i, err)
			}
			res, err := lim.Allow(ctx, "shared-active", sharedLimit, time.Minute)
			if err != nil {
				t.Errorf("shared-active failed: %v", err)
				return
			}
			if res.Allowed {
				countMu.Lock()
				sharedAllowed++
				countMu.Unlock()
			}
		}()
	}
	wg.Wait()

	if sharedAllowed != sharedLimit {
		t.Fatalf("expected %d shared-active allowed calls, got %d", sharedLimit, sharedAllowed)
	}
	lim.mu.Lock()
	remaining := len(lim.entries)
	lim.mu.Unlock()
	if remaining != workers+1 {
		t.Fatalf("expected %d remaining active entries (fresh + shared-active), got %d", workers+1, remaining)
	}
}
