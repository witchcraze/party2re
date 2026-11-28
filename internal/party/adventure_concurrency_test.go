package party_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/party"
)

func getLiveValkeyClient(t *testing.T) (valkey.Client, func()) {
	t.Helper()
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "127.0.0.1:6379"
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{valkeyAddr},
	})
	if err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		t.Skipf("skipping live Valkey test (cannot ping Valkey at %s): %v", valkeyAddr, err)
	}

	return client, func() { client.Close() }
}

// TestMemoryRepository_WithPartyAdventureLock_Reentrancy verifies that reentrant
// lock acquisition on the same context succeeds cleanly.
func TestMemoryRepository_WithPartyAdventureLock_Reentrancy(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()
	partyID := "test-party-reentrant"

	innerCalled := false
	err := repo.WithPartyAdventureLock(ctx, partyID, func(lockedCtx context.Context) error {
		// Reentrant call with same locked context must succeed
		return repo.WithPartyAdventureLock(lockedCtx, partyID, func(_ context.Context) error {
			innerCalled = true
			return nil
		})
	})

	if err != nil {
		t.Fatalf("expected reentrant lock call to succeed, got %v", err)
	}
	if !innerCalled {
		t.Fatalf("expected inner reentrant function to be called")
	}
}

// TestMemoryRepository_WithPartyAdventureLock_Conflict verifies that concurrent
// acquisition attempts on separate contexts fail with ErrPartyNotRecruiting.
func TestMemoryRepository_WithPartyAdventureLock_Conflict(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()
	partyID := fmt.Sprintf("test-party-conflict-%d", time.Now().UnixNano())

	holdLock := make(chan struct{})
	lockAcquired := make(chan struct{})

	var errConflict error
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1 acquires and holds lock
	go func() {
		defer wg.Done()
		_ = repo.WithPartyAdventureLock(ctx, partyID, func(_ context.Context) error {
			close(lockAcquired)
			<-holdLock
			return nil
		})
	}()

	// Goroutine 2 tries to acquire same lock
	go func() {
		defer wg.Done()
		<-lockAcquired
		errConflict = repo.WithPartyAdventureLock(ctx, partyID, func(_ context.Context) error {
			return nil
		})
		close(holdLock)
	}()

	wg.Wait()

	if !errors.Is(errConflict, party.ErrPartyNotRecruiting) {
		t.Fatalf("expected ErrPartyNotRecruiting, got %v", errConflict)
	}

	// After lock is released, subsequent acquisition must succeed
	errSubsequent := repo.WithPartyAdventureLock(ctx, partyID, func(_ context.Context) error {
		return nil
	})
	if errSubsequent != nil {
		t.Fatalf("expected subsequent lock acquisition to succeed, got %v", errSubsequent)
	}
}

// TestMemoryRepository_WithPartyAdventureLock_InstanceIsolation asserts that separate
// memory repositories keep independent lock state.
func TestMemoryRepository_WithPartyAdventureLock_InstanceIsolation(t *testing.T) {
	ctx := context.Background()
	repo1 := party.NewMemoryRepository()
	repo2 := party.NewMemoryRepository()
	partyID := fmt.Sprintf("isolated-party-%d", time.Now().UnixNano())

	holdLock := make(chan struct{})
	repo1Acquired := make(chan struct{})

	go func() {
		_ = repo1.WithPartyAdventureLock(ctx, partyID, func(_ context.Context) error {
			close(repo1Acquired)
			<-holdLock
			return nil
		})
	}()

	<-repo1Acquired
	defer close(holdLock)

	// repo2 has an independent in-memory state; locking the same partyID must succeed on repo2.
	repo2Called := false
	err := repo2.WithPartyAdventureLock(ctx, partyID, func(_ context.Context) error {
		repo2Called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected repo2 lock acquisition to succeed on separate repository instance, got %v", err)
	}
	if !repo2Called {
		t.Fatalf("expected repo2 lock body to be executed")
	}
}
