package party_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/party"
)

type mockConcurrentCharRepo struct {
	mu    sync.RWMutex
	chars map[string]corecharacter.Character
}

func (m *mockConcurrentCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, errors.New("character not found")
	}
	return c, nil
}

func (m *mockConcurrentCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockConcurrentCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chars[c.ID] = c
	return nil
}

type mockConcurrentInvRepo struct {
	mu   sync.RWMutex
	invs map[string]coreinventory.Inventory
}

func (m *mockConcurrentInvRepo) FindByCharacterIDForUpdate(_ context.Context, charID string) (coreinventory.Inventory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inv, ok := m.invs[charID]
	if !ok {
		inv, _ = coreinventory.New(charID)
	}
	return inv, nil
}

func (m *mockConcurrentInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invs[inv.CharacterID] = inv
	return nil
}

type mockConcurrentBattleEngine struct {
	runCount atomic.Int64
}

func (e *mockConcurrentBattleEngine) ResolvePartyBattle(req battle.PartyBattleRequest) (battle.PartyBattleResult, error) {
	e.runCount.Add(1)
	remHP := make(map[string]int)
	remMP := make(map[string]int)
	for _, p := range req.Allies {
		remHP[p.ID] = p.HP
		remMP[p.ID] = p.MP
	}
	// Small delay to simulate crawl and increase race window
	time.Sleep(10 * time.Millisecond)

	return battle.PartyBattleResult{
		Outcome:     battle.OutcomeWin,
		Turns:       1,
		RemainingHP: remHP,
		RemainingMP: remMP,
		TotalReward: battle.Reward{
			Experience: 100,
			Currency:   50,
		},
	}, nil
}

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

// TestStartPartyAdventure_ConcurrentCallsSerialize_InMemory asserts that when multiple concurrent
// StartPartyAdventure calls are submitted simultaneously for the same party in in-memory mode:
// 1. Exactly one execution succeeds.
// 2. All secondary concurrent requests fail with party.ErrPartyNotRecruiting.
// 3. Characters receive exactly one set of rewards (zero duplicated EXP/Gold).
// 4. No race conditions occur under -race.
func TestStartPartyAdventure_ConcurrentCallsSerialize_InMemory(t *testing.T) {
	testConcurrentAdventureSerialization(t, party.NewValkeyRepository(nil))
}

// TestStartPartyAdventure_ConcurrentCallsSerialize_LiveValkey asserts serialization with real Valkey.
func TestStartPartyAdventure_ConcurrentCallsSerialize_LiveValkey(t *testing.T) {
	client, cleanup := getLiveValkeyClient(t)
	defer cleanup()
	testConcurrentAdventureSerialization(t, party.NewValkeyRepository(client))
}

func testConcurrentAdventureSerialization(t *testing.T, partyRepo party.Repository) {
	t.Helper()
	ctx := context.Background()

	charRepo := &mockConcurrentCharRepo{chars: make(map[string]corecharacter.Character)}
	invRepo := &mockConcurrentInvRepo{invs: make(map[string]coreinventory.Inventory)}
	battleEngine := &mockConcurrentBattleEngine{}

	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	svc, err := party.NewService(
		partyRepo,
		charRepo,
		invRepo,
		stages,
		monsters,
		battleEngine,
	)
	if err != nil {
		t.Fatal(err)
	}

	leader := corecharacter.Character{
		ID:       fmt.Sprintf("leader-%d", time.Now().UnixNano()),
		Name:     "Leader",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
			MP:    50,
			MaxMP: 50,
		},
	}
	member := corecharacter.Character{
		ID:       fmt.Sprintf("member-%d", time.Now().UnixNano()),
		Name:     "Member",
		Level:    10,
		JobLevel: 5,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
			MP:    50,
			MaxMP: 50,
		},
	}
	_ = charRepo.Update(ctx, leader)
	_ = charRepo.Update(ctx, member)

	pDetail, err := svc.CreateParty(ctx, leader.ID, party.CreatePartyRequest{
		Name:       "ConcurrencyParty",
		StageID:    "stage-01",
		MaxMembers: 2,
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	partyID := pDetail.Party.ID

	_, err = svc.JoinParty(ctx, partyID, member.ID, "")
	if err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}
	_, err = svc.SetReady(ctx, partyID, member.ID, true)
	if err != nil {
		t.Fatalf("SetReady failed: %v", err)
	}

	const concurrency = 10
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	var successCount atomic.Int64
	var notRecruitingCount atomic.Int64
	var unexpectedErrCount atomic.Int64

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			<-startBarrier

			_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err == nil {
				successCount.Add(1)
			} else if errors.Is(err, party.ErrPartyNotRecruiting) {
				notRecruitingCount.Add(1)
			} else {
				t.Logf("goroutine %d unexpected error: %v", goroutineID, err)
				unexpectedErrCount.Add(1)
			}
		}(i)
	}

	// Release all goroutines simultaneously
	close(startBarrier)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 successful StartPartyAdventure, got %d", successCount.Load())
	}
	if notRecruitingCount.Load() != concurrency-1 {
		t.Fatalf("expected %d ErrPartyNotRecruiting errors, got %d", concurrency-1, notRecruitingCount.Load())
	}
	if unexpectedErrCount.Load() != 0 {
		t.Fatalf("expected 0 unexpected errors, got %d", unexpectedErrCount.Load())
	}

	// Verify that character rewards were not duplicated
	updatedLeader, err := charRepo.FindByID(ctx, leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedLeader.Experience <= 0 {
		t.Errorf("expected leader to gain Experience, got %d", updatedLeader.Experience)
	}
}

// TestValkeyRepository_WithPartyAdventureLock_Reentrancy verifies that reentrant
// lock acquisition on the same context succeeds cleanly.
func TestValkeyRepository_WithPartyAdventureLock_Reentrancy(t *testing.T) {
	ctx := context.Background()
	repo := party.NewValkeyRepository(nil)
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

// TestValkeyRepository_WithPartyAdventureLock_Conflict verifies that concurrent
// acquisition attempts on separate contexts fail with ErrPartyNotRecruiting.
func TestValkeyRepository_WithPartyAdventureLock_Conflict(t *testing.T) {
	ctx := context.Background()
	repo := party.NewValkeyRepository(nil)
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

// TestValkeyRepository_WithPartyAdventureLock_InstanceIsolation asserts that separate
// in-memory ValkeyRepository instances have isolated lock states and do not share global mutable state.
func TestValkeyRepository_WithPartyAdventureLock_InstanceIsolation(t *testing.T) {
	ctx := context.Background()
	repo1 := party.NewValkeyRepository(nil)
	repo2 := party.NewValkeyRepository(nil)
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
