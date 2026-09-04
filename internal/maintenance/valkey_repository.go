package maintenance

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	// DefaultMaintenanceKey is the default Valkey key storing the global maintenance status.
	DefaultMaintenanceKey = "party2:maintenance:status"

	// DefaultMemoryCacheTTL is the duration for in-memory caching to eliminate socket round-trips.
	DefaultMemoryCacheTTL = 1 * time.Second
)

// ValkeyOption defines configuration options for ValkeyRepository.
type ValkeyOption func(*ValkeyRepository)

// WithFallback sets an underlying persistent fallback repository (e.g. MariaDB).
func WithFallback(fallback Repository) ValkeyOption {
	return func(r *ValkeyRepository) {
		r.fallback = fallback
	}
}

// WithKey overrides the default Valkey key.
func WithKey(key string) ValkeyOption {
	return func(r *ValkeyRepository) {
		if key != "" {
			r.key = key
		}
	}
}

// WithMemoryCacheTTL configures the in-memory cache TTL (set to 0 to disable memory caching and query Valkey directly).
func WithMemoryCacheTTL(ttl time.Duration) ValkeyOption {
	return func(r *ValkeyRepository) {
		r.ttl = ttl
	}
}

// ValkeyRepository implements Repository by mastering or caching the maintenance status in Valkey,
// with optional in-memory caching to avoid database queries on every incoming HTTP request.
type ValkeyRepository struct {
	client   valkey.Client
	fallback Repository
	key      string
	ttl      time.Duration

	mu        sync.RWMutex
	cached    Status
	cachedAt  time.Time
	hasCached bool
}

// NewValkeyRepository constructs a new ValkeyRepository.
func NewValkeyRepository(client valkey.Client, opts ...ValkeyOption) *ValkeyRepository {
	r := &ValkeyRepository{
		client: client,
		key:    DefaultMaintenanceKey,
		ttl:    DefaultMemoryCacheTTL,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// GetStatus returns the maintenance status from in-memory cache, Valkey, or fallback repository.
func (r *ValkeyRepository) GetStatus(ctx context.Context) (Status, error) {
	// 1. Check in-memory cache
	if r.ttl > 0 {
		r.mu.RLock()
		if r.hasCached && time.Since(r.cachedAt) < r.ttl {
			st := r.cached
			r.mu.RUnlock()
			return st, nil
		}
		r.mu.RUnlock()
	}

	// 2. Query Valkey if client is available
	if r.client != nil {
		cmd := r.client.B().Get().Key(r.key).Build()
		val, err := r.client.Do(ctx, cmd).ToString()
		if err == nil {
			var st Status
			if jsonErr := json.Unmarshal([]byte(val), &st); jsonErr == nil {
				r.setMemoryCache(st)
				return st, nil
			}
		}
	}

	// 3. Query fallback repository (MariaDB) if available
	if r.fallback != nil {
		st, err := r.fallback.GetStatus(ctx)
		if err == nil {
			// Backfill Valkey so subsequent requests hit Valkey
			if r.client != nil {
				if data, jsonErr := json.Marshal(st); jsonErr == nil {
					setCmd := r.client.B().Set().Key(r.key).Value(string(data)).Build()
					_ = r.client.Do(ctx, setCmd).Error()
				}
			}
			r.setMemoryCache(st)
			return st, nil
		}
	}

	// 4. If everything failed, check if we have a stale in-memory cached value
	r.mu.RLock()
	if r.hasCached {
		st := r.cached
		r.mu.RUnlock()
		return st, nil
	}
	r.mu.RUnlock()

	// 5. Fail-open default: system is operating normally
	defaultStatus := Status{
		Enabled:   false,
		Message:   "System is operating normally.",
		UpdatedAt: time.Now().UTC(),
	}
	return defaultStatus, nil
}

// SetStatus updates the maintenance status in MariaDB (fallback), Valkey, and in-memory cache.
func (r *ValkeyRepository) SetStatus(ctx context.Context, status Status) error {
	// 1. Persist to fallback repository (MariaDB) if present
	if r.fallback != nil {
		if err := r.fallback.SetStatus(ctx, status); err != nil {
			return err
		}
	}

	// 2. Persist to Valkey
	if r.client != nil {
		data, err := json.Marshal(status)
		if err != nil {
			return err
		}
		cmd := r.client.B().Set().Key(r.key).Value(string(data)).Build()
		if err := r.client.Do(ctx, cmd).Error(); err != nil && r.fallback == nil {
			return err
		}
	}

	// 3. Immediately update local in-memory cache
	r.setMemoryCache(status)
	return nil
}

// InvalidateMemoryCache clears the local in-memory cache, forcing the next GetStatus to query Valkey/fallback.
func (r *ValkeyRepository) InvalidateMemoryCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hasCached = false
}

func (r *ValkeyRepository) setMemoryCache(st Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cached = st
	r.cachedAt = time.Now()
	r.hasCached = true
}
