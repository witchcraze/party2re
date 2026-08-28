package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

// Common errors
var (
	ErrInvalidLimit  = errors.New("limit must be greater than zero")
	ErrInvalidWindow = errors.New("window must be greater than zero")
)

// Result contains the outcome of a rate limit check.
type Result struct {
	Allowed    bool          `json:"allowed"`
	Limit      int64         `json:"limit"`
	Remaining  int64         `json:"remaining"`
	ResetAfter time.Duration `json:"reset_after"`
}

// Limiter defines the rate limiting interface.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (Result, error)
}

// memoryEntry stores count and expiration for in-memory rate limiting.
type memoryEntry struct {
	count     int64
	expiresAt time.Time
}

// MemoryLimiter provides an in-memory thread-safe rate limiter.
type MemoryLimiter struct {
	mu      sync.Mutex
	entries map[string]memoryEntry
	nowFunc func() time.Time
}

// NewMemoryLimiter creates a new in-memory rate limiter.
func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{
		entries: make(map[string]memoryEntry),
		nowFunc: time.Now,
	}
}

// Allow checks if an action under key is permitted within the limit and window.
func (m *MemoryLimiter) Allow(_ context.Context, key string, limit int64, window time.Duration) (Result, error) {
	if limit <= 0 {
		return Result{}, ErrInvalidLimit
	}
	if window <= 0 {
		return Result{}, ErrInvalidWindow
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.nowFunc()
	entry, exists := m.entries[key]

	if !exists || now.After(entry.expiresAt) {
		m.entries[key] = memoryEntry{
			count:     1,
			expiresAt: now.Add(window),
		}
		return Result{
			Allowed:    true,
			Limit:      limit,
			Remaining:  limit - 1,
			ResetAfter: window,
		}, nil
	}

	if entry.count >= limit {
		resetAfter := entry.expiresAt.Sub(now)
		if resetAfter < 0 {
			resetAfter = 0
		}
		return Result{
			Allowed:    false,
			Limit:      limit,
			Remaining:  0,
			ResetAfter: resetAfter,
		}, nil
	}

	entry.count++
	m.entries[key] = entry

	resetAfter := entry.expiresAt.Sub(now)
	if resetAfter < 0 {
		resetAfter = 0
	}

	return Result{
		Allowed:    true,
		Limit:      limit,
		Remaining:  limit - entry.count,
		ResetAfter: resetAfter,
	}, nil
}

const (
	defaultKeyPrefix = "party2:ratelimit:"
	rateLimitLua     = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])

local current = redis.call('INCR', key)
if current == 1 then
    redis.call('PEXPIRE', key, window_ms)
end
local ttl = redis.call('PTTL', key)
if ttl < 0 then
    redis.call('PEXPIRE', key, window_ms)
    ttl = window_ms
end

return {current, ttl}
`
)

// ValkeyLimiter provides a distributed atomic rate limiter powered by Valkey.
type ValkeyLimiter struct {
	client    valkey.Client
	script    *valkey.Lua
	keyPrefix string
	fallback  *MemoryLimiter
	failOpen  bool
}

// ValkeyOption configures optional parameters on ValkeyLimiter.
type ValkeyOption func(*ValkeyLimiter)

// WithKeyPrefix overrides the default key prefix in Valkey.
func WithKeyPrefix(prefix string) ValkeyOption {
	return func(v *ValkeyLimiter) {
		v.keyPrefix = prefix
	}
}

// WithFailOpen specifies whether the limiter should allow requests when Valkey is down.
func WithFailOpen(failOpen bool) ValkeyOption {
	return func(v *ValkeyLimiter) {
		v.failOpen = failOpen
	}
}

// NewValkeyLimiter creates a new Valkey-backed distributed rate limiter.
func NewValkeyLimiter(client valkey.Client, opts ...ValkeyOption) *ValkeyLimiter {
	lim := &ValkeyLimiter{
		client:    client,
		script:    valkey.NewLuaScript(rateLimitLua),
		keyPrefix: defaultKeyPrefix,
		fallback:  NewMemoryLimiter(),
		failOpen:  true,
	}
	for _, opt := range opts {
		opt(lim)
	}
	return lim
}

// Allow executes an atomic rate limit check in Valkey using Lua scripting.
func (v *ValkeyLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (Result, error) {
	if limit <= 0 {
		return Result{}, ErrInvalidLimit
	}
	if window <= 0 {
		return Result{}, ErrInvalidWindow
	}

	if v.client == nil {
		if v.fallback != nil {
			return v.fallback.Allow(ctx, key, limit, window)
		}
		if v.failOpen {
			return Result{Allowed: true, Limit: limit, Remaining: limit, ResetAfter: 0}, nil
		}
		return Result{}, errors.New("valkey client is not configured")
	}

	fullKey := v.keyPrefix + key
	windowMs := window.Milliseconds()
	if windowMs < 1 {
		windowMs = 1
	}

	res := v.script.Exec(ctx, v.client, []string{fullKey}, []string{
		strconv.FormatInt(limit, 10),
		strconv.FormatInt(windowMs, 10),
	})

	if err := res.Error(); err != nil {
		// If Valkey error occurs, fallback to in-memory limiter or fail-open
		if v.fallback != nil {
			return v.fallback.Allow(ctx, key, limit, window)
		}
		if v.failOpen {
			return Result{Allowed: true, Limit: limit, Remaining: limit, ResetAfter: 0}, nil
		}
		return Result{}, fmt.Errorf("valkey rate limit check failed: %w", err)
	}

	vals, err := res.AsIntSlice()
	if err != nil || len(vals) < 2 {
		if v.failOpen {
			return Result{Allowed: true, Limit: limit, Remaining: limit, ResetAfter: 0}, nil
		}
		return Result{}, fmt.Errorf("invalid valkey lua result: %v (err: %w)", vals, err)
	}

	current := vals[0]
	ttlMs := vals[1]
	if ttlMs < 0 {
		ttlMs = 0
	}

	resetAfter := time.Duration(ttlMs) * time.Millisecond
	if current <= limit {
		return Result{
			Allowed:    true,
			Limit:      limit,
			Remaining:  limit - current,
			ResetAfter: resetAfter,
		}, nil
	}

	return Result{
		Allowed:    false,
		Limit:      limit,
		Remaining:  0,
		ResetAfter: resetAfter,
	}, nil
}
