package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyLimiter_InvalidParams(t *testing.T) {
	lim := NewValkeyLimiter(nil)
	ctx := context.Background()

	if _, err := lim.Allow(ctx, "k", 0, time.Second); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("expected ErrInvalidLimit, got %v", err)
	}
	if _, err := lim.Allow(ctx, "k", -5, time.Second); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("expected ErrInvalidLimit for negative limit, got %v", err)
	}
	if _, err := lim.Allow(ctx, "k", 10, 0); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("expected ErrInvalidWindow, got %v", err)
	}
	if _, err := lim.Allow(ctx, "k", 10, -time.Second); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("expected ErrInvalidWindow for negative window, got %v", err)
	}
}

func TestValkeyLimiter_NilClient(t *testing.T) {
	ctx := context.Background()

	// 1. Nil client with default fallback (MemoryLimiter)
	limWithFallback := NewValkeyLimiter(nil)
	res, err := limWithFallback.Allow(ctx, "key-fb", 2, time.Second)
	if err != nil {
		t.Fatalf("unexpected error with fallback: %v", err)
	}
	if !res.Allowed || res.Remaining != 1 {
		t.Fatalf("expected allowed with remaining 1, got %+v", res)
	}

	// 2. Nil client without fallback, failOpen = true
	limFailOpen := NewValkeyLimiter(nil, WithFailOpen(true))
	limFailOpen.fallback = nil
	res, err = limFailOpen.Allow(ctx, "key-failopen", 5, time.Second)
	if err != nil {
		t.Fatalf("unexpected error when failOpen=true: %v", err)
	}
	if !res.Allowed || res.Remaining != 5 || res.ResetAfter != 0 {
		t.Fatalf("expected failOpen allowed result, got %+v", res)
	}

	// 3. Nil client without fallback, failOpen = false
	limFailClosed := NewValkeyLimiter(nil, WithFailOpen(false))
	limFailClosed.fallback = nil
	_, err = limFailClosed.Allow(ctx, "key-failclosed", 5, time.Second)
	if err == nil {
		t.Fatal("expected error when client is nil, fallback is nil, and failOpen is false")
	}
}

func TestValkeyLimiter_ValkeyExecError(t *testing.T) {
	ctx := context.Background()
	errValkeyDown := errors.New("valkey connection refused")

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errValkeyDown)
	}))

	// 1. With fallback: should invoke fallback MemoryLimiter and succeed
	limWithFallback := NewValkeyLimiter(client)
	res, err := limWithFallback.Allow(ctx, "user-err-1", 3, time.Second)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if !res.Allowed || res.Remaining != 2 {
		t.Fatalf("expected fallback allowed with remaining 2, got %+v", res)
	}

	// 2. Without fallback, failOpen = true: should allow
	limFailOpen := NewValkeyLimiter(client, WithFailOpen(true))
	limFailOpen.fallback = nil
	res, err = limFailOpen.Allow(ctx, "user-err-2", 3, time.Second)
	if err != nil {
		t.Fatalf("expected failOpen to allow without error, got %v", err)
	}
	if !res.Allowed || res.Remaining != 3 {
		t.Fatalf("expected failOpen allowed with remaining 3, got %+v", res)
	}

	// 3. Without fallback, failOpen = false: should return error
	limFailClosed := NewValkeyLimiter(client, WithFailOpen(false))
	limFailClosed.fallback = nil
	_, err = limFailClosed.Allow(ctx, "user-err-3", 3, time.Second)
	if err == nil || !errors.Is(err, errValkeyDown) {
		t.Fatalf("expected wrapped errValkeyDown, got %v", err)
	}
}

func TestValkeyLimiter_MalformedLuaResult(t *testing.T) {
	ctx := context.Background()

	// 1. Too few elements (len < 2)
	clientShort := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntSliceResult([]int64{1})
	}))

	// Fail-open: allows
	limFailOpen := NewValkeyLimiter(clientShort, WithFailOpen(true))
	res, err := limFailOpen.Allow(ctx, "k", 5, time.Second)
	if err != nil || !res.Allowed {
		t.Fatalf("expected allowed when failOpen=true, got res=%+v err=%v", res, err)
	}

	// Fail-closed: returns error
	limFailClosed := NewValkeyLimiter(clientShort, WithFailOpen(false))
	_, err = limFailClosed.Allow(ctx, "k", 5, time.Second)
	if err == nil {
		t.Fatal("expected error on malformed result when failOpen=false")
	}

	// 2. Empty result (len == 0)
	clientEmpty := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntSliceResult([]int64{})
	}))
	limEmptyClosed := NewValkeyLimiter(clientEmpty, WithFailOpen(false))
	if _, err := limEmptyClosed.Allow(ctx, "k", 5, time.Second); err == nil {
		t.Fatal("expected error on empty result when failOpen=false")
	}
}

func TestValkeyLimiter_SuccessAllowedAndBlocked(t *testing.T) {
	ctx := context.Background()

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntSliceResult([]int64{2, 1500})
	}))

	lim := NewValkeyLimiter(client, WithKeyPrefix("custom:rl:"))

	// Allowed check
	res, err := lim.Allow(ctx, "client-123", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected allowed=true, got %+v", res)
	}
	if res.Limit != 5 || res.Remaining != 3 {
		t.Fatalf("expected limit=5 remaining=3, got %+v", res)
	}
	if res.ResetAfter != 1500*time.Millisecond {
		t.Fatalf("expected resetAfter=1500ms, got %v", res.ResetAfter)
	}

	// Verify key passed to Lua has custom prefix
	recordedCmd := client.LastCommandStrings()
	if len(recordedCmd) < 4 || recordedCmd[3] != "custom:rl:client-123" {
		t.Fatalf("expected custom key prefix in command, got %v", recordedCmd)
	}

	// Blocked check (current > limit)
	clientBlocked := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntSliceResult([]int64{6, 800})
	}))
	limBlocked := NewValkeyLimiter(clientBlocked)
	res, err = limBlocked.Allow(ctx, "client-blocked", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed || res.Remaining != 0 {
		t.Fatalf("expected blocked with remaining=0, got %+v", res)
	}
	if res.ResetAfter != 800*time.Millisecond {
		t.Fatalf("expected resetAfter=800ms, got %v", res.ResetAfter)
	}

	// Negative TTL check (clamped to 0)
	clientNegTTL := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntSliceResult([]int64{1, -50})
	}))
	limNegTTL := NewValkeyLimiter(clientNegTTL)
	res, err = limNegTTL.Allow(ctx, "client-neg-ttl", 5, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ResetAfter != 0 {
		t.Fatalf("expected resetAfter=0 for negative TTL, got %v", res.ResetAfter)
	}
}

func TestValkeyLimiter_SubMillisecondWindow(t *testing.T) {
	ctx := context.Background()

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeIntSliceResult([]int64{1, 1})
	}))

	lim := NewValkeyLimiter(client)
	// 500 microseconds has Milliseconds() == 0, triggering windowMs = 1 clamp
	_, err := lim.Allow(ctx, "sub-ms", 5, 500*time.Microsecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Last argument is windowMs, should be "1"
	recordedCmd := client.LastCommandStrings()
	if len(recordedCmd) < 5 || recordedCmd[len(recordedCmd)-1] != "1" {
		t.Fatalf("expected windowMs clamped to 1, got %v", recordedCmd)
	}
}
