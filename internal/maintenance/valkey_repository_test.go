package maintenance_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/maintenance"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type countingFallbackRepo struct {
	mu           sync.Mutex
	status       maintenance.Status
	getCalls     int64
	setCalls     int64
	failGetCalls bool
	failSetCalls bool
}

func (c *countingFallbackRepo) GetStatus(ctx context.Context) (maintenance.Status, error) {
	atomic.AddInt64(&c.getCalls, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failGetCalls {
		return maintenance.Status{}, errors.New("db connection failure")
	}
	return c.status, nil
}

func (c *countingFallbackRepo) SetStatus(ctx context.Context, status maintenance.Status) error {
	atomic.AddInt64(&c.setCalls, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failSetCalls {
		return errors.New("db write failure")
	}
	c.status = status
	return nil
}

func TestValkeyRepository_MemoryCache_AvoidsFallbackQueries(t *testing.T) {
	ctx := context.Background()
	fallback := &countingFallbackRepo{
		status: maintenance.Status{
			Enabled:   true,
			Message:   "Scheduled maintenance in progress",
			UpdatedAt: time.Now().UTC(),
		},
	}

	repo := maintenance.NewValkeyRepository(nil,
		maintenance.WithFallback(fallback),
		maintenance.WithMemoryCacheTTL(500*time.Millisecond),
	)

	// First call queries fallback
	st1, err := repo.GetStatus(ctx)
	if err != nil {
		t.Fatalf("first GetStatus failed: %v", err)
	}
	if !st1.Enabled || st1.Message != "Scheduled maintenance in progress" {
		t.Errorf("unexpected status: %+v", st1)
	}
	if atomic.LoadInt64(&fallback.getCalls) != 1 {
		t.Errorf("expected 1 call to fallback, got %d", atomic.LoadInt64(&fallback.getCalls))
	}

	// Subsequent 10 calls within TTL do NOT query fallback
	for i := 0; i < 10; i++ {
		st, err := repo.GetStatus(ctx)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if !st.Enabled {
			t.Errorf("expected enabled=true")
		}
	}

	if calls := atomic.LoadInt64(&fallback.getCalls); calls != 1 {
		t.Errorf("expected still exactly 1 call to fallback, got %d", calls)
	}

	// Invalidate cache and verify fallback is queried again
	repo.InvalidateMemoryCache()
	_, _ = repo.GetStatus(ctx)
	if calls := atomic.LoadInt64(&fallback.getCalls); calls != 2 {
		t.Errorf("expected 2 calls to fallback after cache invalidation, got %d", calls)
	}
}

func TestValkeyRepository_SetStatus_UpdatesImmediately(t *testing.T) {
	ctx := context.Background()
	fallback := &countingFallbackRepo{
		status: maintenance.Status{
			Enabled:   false,
			Message:   "System is operating normally.",
			UpdatedAt: time.Now().UTC(),
		},
	}

	repo := maintenance.NewValkeyRepository(nil,
		maintenance.WithFallback(fallback),
		maintenance.WithMemoryCacheTTL(10*time.Second),
	)

	// Pre-populate memory cache
	_, _ = repo.GetStatus(ctx)

	// Update status
	endTime := time.Now().Add(time.Hour).UTC()
	newStatus := maintenance.Status{
		Enabled:          true,
		Message:          "Admin emergency shutdown",
		EstimatedEndTime: &endTime,
		UpdatedAt:        time.Now().UTC(),
	}

	if err := repo.SetStatus(ctx, newStatus); err != nil {
		t.Fatalf("SetStatus failed: %v", err)
	}

	// Verify fallback received update
	if atomic.LoadInt64(&fallback.setCalls) != 1 {
		t.Errorf("expected 1 set call to fallback")
	}

	// Immediately check GetStatus: must return newStatus without waiting for TTL
	got, err := repo.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !got.Enabled || got.Message != "Admin emergency shutdown" {
		t.Errorf("expected immediate update, got: %+v", got)
	}
}

func TestValkeyRepository_FailOpen_WhenFallbackErrors(t *testing.T) {
	ctx := context.Background()
	fallback := &countingFallbackRepo{
		failGetCalls: true,
	}

	repo := maintenance.NewValkeyRepository(nil,
		maintenance.WithFallback(fallback),
		maintenance.WithMemoryCacheTTL(0), // disable memory cache
	)

	st, err := repo.GetStatus(ctx)
	if err != nil {
		t.Fatalf("expected fail-open without error, got: %v", err)
	}
	if st.Enabled {
		t.Errorf("expected fail-open enabled=false, got true")
	}
	if st.Message != "System is operating normally." {
		t.Errorf("unexpected message: %s", st.Message)
	}
}

func TestValkeyRepository_RealValkeyIntegration(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not set, skipping real Valkey test")
	}

	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("open valkey client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	testKey := "party2:test:maintenance:status"
	defer client.Do(ctx, client.B().Del().Key(testKey).Build())

	fallback := &countingFallbackRepo{
		status: maintenance.Status{
			Enabled:   true,
			Message:   "Initial DB maintenance status",
			UpdatedAt: time.Now().UTC(),
		},
	}

	repo := maintenance.NewValkeyRepository(client,
		maintenance.WithFallback(fallback),
		maintenance.WithKey(testKey),
		maintenance.WithMemoryCacheTTL(0), // test direct Valkey reads
	)

	// 1. Initial read: Valkey is empty -> should read fallback and backfill Valkey
	st, err := repo.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !st.Enabled || st.Message != "Initial DB maintenance status" {
		t.Errorf("unexpected status from fallback: %+v", st)
	}
	if atomic.LoadInt64(&fallback.getCalls) != 1 {
		t.Errorf("expected 1 call to fallback, got %d", atomic.LoadInt64(&fallback.getCalls))
	}

	// Verify key in Valkey
	rawJSON, err := client.Do(ctx, client.B().Get().Key(testKey).Build()).ToString()
	if err != nil {
		t.Fatalf("expected testKey in Valkey: %v", err)
	}
	var stored maintenance.Status
	if err := json.Unmarshal([]byte(rawJSON), &stored); err != nil {
		t.Fatalf("unmarshal Valkey JSON: %v", err)
	}
	if !stored.Enabled || stored.Message != "Initial DB maintenance status" {
		t.Errorf("unexpected stored JSON: %+v", stored)
	}

	// 2. Second read: should read directly from Valkey, fallback count remains 1
	st2, err := repo.GetStatus(ctx)
	if err != nil {
		t.Fatalf("second GetStatus: %v", err)
	}
	if !st2.Enabled || st2.Message != "Initial DB maintenance status" {
		t.Errorf("unexpected status from Valkey: %+v", st2)
	}
	if calls := atomic.LoadInt64(&fallback.getCalls); calls != 1 {
		t.Errorf("expected fallback calls to remain 1 (zero SQL on repeated reads), got %d", calls)
	}

	// 3. SetStatus: updates both fallback and Valkey
	updatedStatus := maintenance.Status{
		Enabled:   false,
		Message:   "System is operating normally.",
		UpdatedAt: time.Now().UTC(),
	}
	if err := repo.SetStatus(ctx, updatedStatus); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	st3, err := repo.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus after update: %v", err)
	}
	if st3.Enabled {
		t.Errorf("expected enabled=false after update")
	}
	if calls := atomic.LoadInt64(&fallback.getCalls); calls != 1 {
		t.Errorf("expected fallback get calls to still be 1, got %d", calls)
	}
}
