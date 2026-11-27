package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ratelimit"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestMemoryLimiter_BasicAndExpiry(t *testing.T) {
	lim := ratelimit.NewMemoryLimiter()
	ctx := context.Background()

	// 1. Initial request allowed
	res, err := lim.Allow(ctx, "user-1", 2, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed || res.Remaining != 1 {
		t.Errorf("expected allowed with 1 remaining, got %+v", res)
	}

	// 2. Second request allowed
	res, err = lim.Allow(ctx, "user-1", 2, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed || res.Remaining != 0 {
		t.Errorf("expected allowed with 0 remaining, got %+v", res)
	}

	// 3. Third request rejected
	res, err = lim.Allow(ctx, "user-1", 2, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed || res.Remaining != 0 {
		t.Errorf("expected blocked, got %+v", res)
	}

	// 4. Distinct key allowed
	res, err = lim.Allow(ctx, "user-2", 2, 100*time.Millisecond)
	if err != nil || !res.Allowed {
		t.Errorf("expected user-2 allowed, got %+v (err: %v)", res, err)
	}

	// 5. Expiry resets allowance
	time.Sleep(120 * time.Millisecond)
	res, err = lim.Allow(ctx, "user-1", 2, 100*time.Millisecond)
	if err != nil || !res.Allowed || res.Remaining != 1 {
		t.Errorf("expected allowed after window expiration, got %+v (err: %v)", res, err)
	}
}

func TestMemoryLimiter_InvalidParams(t *testing.T) {
	lim := ratelimit.NewMemoryLimiter()
	ctx := context.Background()

	_, err := lim.Allow(ctx, "k", 0, time.Second)
	if err != ratelimit.ErrInvalidLimit {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}

	_, err = lim.Allow(ctx, "k", 5, 0)
	if err != ratelimit.ErrInvalidWindow {
		t.Errorf("expected ErrInvalidWindow, got %v", err)
	}
}

func TestMemoryLimiter_ConcurrentAccess(t *testing.T) {
	lim := ratelimit.NewMemoryLimiter()
	ctx := context.Background()

	const workers = 50
	const limit = 20
	var wg sync.WaitGroup
	var allowedCount int64
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := lim.Allow(ctx, "concurrent-key", limit, time.Minute)
			if err == nil && res.Allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowedCount != limit {
		t.Errorf("expected exactly %d allowed requests under concurrent load, got %d", limit, allowedCount)
	}
}

func TestValkeyLimiter_Integration(t *testing.T) {
	client := testutil.OpenValkey(t)

	lim := ratelimit.NewValkeyLimiter(client, ratelimit.WithKeyPrefix("test:ratelimit:"))
	ctx := context.Background()
	testKey := "test-ip-1"
	cleanupKey := "test:ratelimit:" + testKey
	defer func() {
		if err := client.Do(ctx, client.B().Del().Key(cleanupKey).Build()).Error(); err != nil {
			t.Errorf("cleanup DEL %s: %v", cleanupKey, err)
		}
	}()

	// 1. Initial hit
	res, err := lim.Allow(ctx, testKey, 3, 2*time.Second)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !res.Allowed || res.Remaining != 2 {
		t.Errorf("expected allowed remaining 2, got %+v", res)
	}

	// 2. Hit 2 and 3
	_, _ = lim.Allow(ctx, testKey, 3, 2*time.Second)
	res, _ = lim.Allow(ctx, testKey, 3, 2*time.Second)
	if !res.Allowed || res.Remaining != 0 {
		t.Errorf("expected allowed remaining 0, got %+v", res)
	}

	// 3. Hit 4 (Blocked)
	res, err = lim.Allow(ctx, testKey, 3, 2*time.Second)
	if err != nil {
		t.Fatalf("Allow error: %v", err)
	}
	if res.Allowed || res.Remaining != 0 || res.ResetAfter <= 0 {
		t.Errorf("expected blocked with positive resetAfter, got %+v", res)
	}
}
