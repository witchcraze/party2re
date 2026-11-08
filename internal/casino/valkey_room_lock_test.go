package casino_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

func TestValkeyRoomLock_MutualExclusion(t *testing.T) {
	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}

	roomID := "lock-room-" + id.New()[:8]
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(casino.DefaultRoomLockKeyPrefix+roomID).Build()).Error(); err != nil {
			t.Errorf("cleanup lock key: %v", err)
		}
	})

	started1 := make(chan struct{})
	release1 := make(chan struct{})
	done1 := make(chan struct{})

	var executionOrder []string
	var mu sync.Mutex
	record := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		executionOrder = append(executionOrder, name)
	}

	go func() {
		defer close(done1)
		err := repo.WithRoomLock(context.Background(), roomID, func(ctx context.Context) error {
			record("worker1-acquired")
			close(started1)
			<-release1
			record("worker1-releasing")
			return nil
		})
		if err != nil {
			t.Errorf("worker1 WithRoomLock error: %v", err)
		}
	}()

	<-started1

	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		err := repo.WithRoomLock(context.Background(), roomID, func(ctx context.Context) error {
			record("worker2-acquired")
			return nil
		})
		if err != nil {
			t.Errorf("worker2 WithRoomLock error: %v", err)
		}
	}()

	// Worker 2 must be blocked while Worker 1 holds the lock
	select {
	case <-done2:
		t.Fatal("worker2 acquired lock prematurely while worker1 held it")
	case <-time.After(50 * time.Millisecond):
		// Expected: worker 2 is waiting
	}

	close(release1)
	<-done1
	<-done2

	mu.Lock()
	order := make([]string, len(executionOrder))
	copy(order, executionOrder)
	mu.Unlock()

	expected := []string{"worker1-acquired", "worker1-releasing", "worker2-acquired"}
	if len(order) != len(expected) {
		t.Fatalf("unexpected execution order length: got %v, want %v", order, expected)
	}
	for i, exp := range expected {
		if order[i] != exp {
			t.Fatalf("at index %d: got %s, want %s (full order: %v)", i, order[i], exp, order)
		}
	}
}

func TestValkeyRoomLock_Reentrancy(t *testing.T) {
	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}

	roomID := "reentrant-room-" + id.New()[:8]
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(casino.DefaultRoomLockKeyPrefix+roomID).Build()).Error(); err != nil {
			t.Errorf("cleanup lock key: %v", err)
		}
	})

	reentrantExecuted := false
	err = repo.WithRoomLock(context.Background(), roomID, func(ctx context.Context) error {
		return repo.WithRoomLock(ctx, roomID, func(nestedCtx context.Context) error {
			reentrantExecuted = true
			return nil
		})
	})
	if err != nil {
		t.Fatalf("unexpected reentrancy error: %v", err)
	}
	if !reentrantExecuted {
		t.Fatal("reentrant lock block did not execute")
	}
}

func TestValkeyRoomLock_Cancellation(t *testing.T) {
	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}

	roomID := "cancel-room-" + id.New()[:8]
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(casino.DefaultRoomLockKeyPrefix+roomID).Build()).Error(); err != nil {
			t.Errorf("cleanup lock key: %v", err)
		}
	})

	started1 := make(chan struct{})
	release1 := make(chan struct{})
	defer close(release1)

	go func() {
		_ = repo.WithRoomLock(context.Background(), roomID, func(ctx context.Context) error {
			close(started1)
			<-release1
			return nil
		})
	}()

	<-started1

	ctx2, cancel2 := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- repo.WithRoomLock(ctx2, roomID, func(ctx context.Context) error {
			return nil
		})
	}()

	// Cancel context while waiting in retry loop
	time.Sleep(30 * time.Millisecond)
	cancel2()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WithRoomLock did not abort promptly on cancellation")
	}
}

func TestValkeyRoomLock_CallerDeadlineTimeout(t *testing.T) {
	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}

	roomID := "deadline-room-" + id.New()[:8]
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(casino.DefaultRoomLockKeyPrefix+roomID).Build()).Error(); err != nil {
			t.Errorf("cleanup lock key: %v", err)
		}
	})

	started1 := make(chan struct{})
	release1 := make(chan struct{})
	defer close(release1)

	go func() {
		_ = repo.WithRoomLock(context.Background(), roomID, func(ctx context.Context) error {
			close(started1)
			<-release1
			return nil
		})
	}()

	<-started1

	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()

	err = repo.WithRoomLock(ctx2, roomID, func(ctx context.Context) error {
		return nil
	})

	if !errors.Is(err, context.DeadlineExceeded) && (err == nil || !strings.Contains(err.Error(), "timeout")) {
		t.Fatalf("expected timeout or deadline exceeded, got: %v", err)
	}
}

func TestValkeyRoomLock_TokenCheckedRelease(t *testing.T) {
	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}

	roomID := "token-release-room-" + id.New()[:8]
	lockKey := casino.DefaultRoomLockKeyPrefix + roomID
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(lockKey).Build()).Error(); err != nil {
			t.Errorf("cleanup lock key: %v", err)
		}
	})

	foreignToken := "foreign-hijacked-token"

	err = repo.WithRoomLock(context.Background(), roomID, func(ctx context.Context) error {
		// Simulate lock lease expiry and another caller acquiring the key with a different token
		cmd := raw.B().Set().Key(lockKey).Value(foreignToken).Build()
		if err := raw.Do(ctx, cmd).Error(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Defer release executed. Verify foreign token was NOT deleted
	val, err := raw.Do(context.Background(), raw.B().Get().Key(lockKey).Build()).ToString()
	if err != nil {
		t.Fatalf("foreign lock was deleted or not found: %v", err)
	}
	if val != foreignToken {
		t.Fatalf("expected key to retain %q, got %q", foreignToken, val)
	}
}

// countingValkeyClient wraps valkeygo.Client to count specific command executions.
type countingValkeyClient struct {
	valkeygo.Client
	lockSetNXCount atomic.Int64
}

func (c *countingValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	tokens := cmd.Commands()
	if len(tokens) >= 5 && tokens[0] == "SET" && strings.HasPrefix(tokens[1], casino.DefaultRoomLockKeyPrefix) {
		for _, tok := range tokens[3:] {
			if tok == "NX" {
				c.lockSetNXCount.Add(1)
				break
			}
		}
	}
	return c.Client.Do(ctx, cmd)
}

func TestRoomDetail_SingleLockAcquisitionPerAction(t *testing.T) {
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup db close: %v", err)
		}
	})

	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	ctx := context.Background()
	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "PlayerA", 10000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "PlayerB", 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			"DELETE FROM casino_accounts WHERE character_id = ?",
			"DELETE FROM character_depots WHERE character_id = ?",
			"DELETE FROM characters WHERE id = ?",
		} {
			if _, err := db.ExecContext(context.Background(), query, charA.ID); err != nil {
				t.Errorf("cleanup charA (%s): %v", query, err)
			}
			if _, err := db.ExecContext(context.Background(), query, charB.ID); err != nil {
				t.Errorf("cleanup charB (%s): %v", query, err)
			}
		}
		if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id IN (?, ?)", charA.PlayerID, charB.PlayerID); err != nil {
			t.Errorf("cleanup players: %v", err)
		}
	})

	countingClient := &countingValkeyClient{Client: raw}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	roomRepo, err := casino.NewValkeyRoomRepository(countingClient, casino.WithValkeyCharacterRepository(charRepo))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoRepo,
		casino.WithCharacterRepository(charRepo),
		casino.WithTransactionProvider(database.NewTransactionProvider(db)),
		casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.ExchangeGoldToCoins(ctx, charA.ID, 500)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.ExchangeGoldToCoins(ctx, charB.ID, 500)
	if err != nil {
		t.Fatal(err)
	}

	// Test HighLow Start and Play single lock acquisition
	hlRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		Name: "HL-" + id.New()[:8], GameType: casino.GameTypeHighLow, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := roomRepo.DeleteRoom(context.Background(), hlRoom.Room.ID); err != nil {
			t.Errorf("cleanup hlRoom: %v", err)
		}
	})

	_, err = svc.JoinRoom(ctx, hlRoom.Room.ID, charB.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}

	// Reset lock acquisition count before StartHighLow
	countingClient.lockSetNXCount.Store(0)
	detail, err := svc.StartHighLow(ctx, hlRoom.Room.ID, charA.ID)
	if err != nil {
		t.Fatalf("StartHighLow error: %v", err)
	}
	if detail == nil {
		t.Fatal("StartHighLow returned nil detail")
	}
	// StartHighLow must acquire the room lock exactly once (not re-acquire in getRoomDetail)
	if count := countingClient.lockSetNXCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 lock acquisition for StartHighLow, got %d", count)
	}

	// Reset before PlayHighLowAction
	countingClient.lockSetNXCount.Store(0)
	playDetail, err := svc.PlayHighLowAction(ctx, hlRoom.Room.ID, charA.ID, casino.HighLowActionHigh)
	if err != nil {
		t.Fatalf("PlayHighLowAction error: %v", err)
	}
	if playDetail == nil {
		t.Fatal("PlayHighLowAction returned nil detail")
	}
	// PlayHighLowAction must acquire the room lock exactly once (not re-acquire in getRoomDetail)
	if count := countingClient.lockSetNXCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 lock acquisition for PlayHighLowAction, got %d", count)
	}
	if err := roomRepo.DeleteRoom(ctx, hlRoom.Room.ID); err != nil {
		t.Fatalf("cleanup hlRoom before indRoom: %v", err)
	}

	// Test Indian Poker Start and Play single lock acquisition
	indRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		Name: "Ind-" + id.New()[:8], GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := roomRepo.DeleteRoom(context.Background(), indRoom.Room.ID); err != nil {
			t.Errorf("cleanup indRoom: %v", err)
		}
	})

	_, err = svc.JoinRoom(ctx, indRoom.Room.ID, charB.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}

	countingClient.lockSetNXCount.Store(0)
	indStart, err := svc.StartIndianPoker(ctx, indRoom.Room.ID, charA.ID)
	if err != nil {
		t.Fatalf("StartIndianPoker error: %v", err)
	}
	if indStart == nil {
		t.Fatal("StartIndianPoker returned nil detail")
	}
	if count := countingClient.lockSetNXCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 lock acquisition for StartIndianPoker, got %d", count)
	}

	countingClient.lockSetNXCount.Store(0)
	indPlay, err := svc.PlayIndianPokerAction(ctx, indRoom.Room.ID, charA.ID, casino.ActionCall)
	if err != nil {
		t.Fatalf("PlayIndianPokerAction error: %v", err)
	}
	if indPlay == nil {
		t.Fatal("PlayIndianPokerAction returned nil detail")
	}
	if count := countingClient.lockSetNXCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 lock acquisition for PlayIndianPokerAction, got %d", count)
	}
	if err := roomRepo.DeleteRoom(ctx, indRoom.Room.ID); err != nil {
		t.Fatalf("cleanup indRoom before dpRoom: %v", err)
	}

	// Test Doppel Start and Play single lock acquisition
	dpRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		Name: "Dp-" + id.New()[:8], GameType: casino.GameTypeDoppel, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := roomRepo.DeleteRoom(context.Background(), dpRoom.Room.ID); err != nil {
			t.Errorf("cleanup dpRoom: %v", err)
		}
	})

	_, err = svc.JoinRoom(ctx, dpRoom.Room.ID, charB.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}

	countingClient.lockSetNXCount.Store(0)
	dpStart, err := svc.StartDoppel(ctx, dpRoom.Room.ID, charA.ID)
	if err != nil {
		t.Fatalf("StartDoppel error: %v", err)
	}
	if dpStart == nil {
		t.Fatal("StartDoppel returned nil detail")
	}
	if count := countingClient.lockSetNXCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 lock acquisition for StartDoppel, got %d", count)
	}

	countingClient.lockSetNXCount.Store(0)
	dpPlay, err := svc.PlayDoppelAction(ctx, dpRoom.Room.ID, charA.ID, 0)
	if err != nil {
		t.Fatalf("PlayDoppelAction error: %v", err)
	}
	if dpPlay == nil {
		t.Fatal("PlayDoppelAction returned nil detail")
	}
	if count := countingClient.lockSetNXCount.Load(); count != 1 {
		t.Fatalf("expected exactly 1 lock acquisition for PlayDoppelAction, got %d", count)
	}
}

func TestValkeyRoomLock_LeaseSafetyAndContentionHoldTiming(t *testing.T) {
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("cleanup db close: %v", err)
		}
	})

	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	ctx := context.Background()
	charA, err := database.CreateTestCharacterWithFunds(ctx, db, "LeaseSafeA", 50000)
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacterWithFunds(ctx, db, "LeaseSafeB", 50000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			"DELETE FROM casino_accounts WHERE character_id = ?",
			"DELETE FROM character_depots WHERE character_id = ?",
			"DELETE FROM characters WHERE id = ?",
		} {
			if _, err := db.ExecContext(context.Background(), query, charA.ID); err != nil {
				t.Errorf("cleanup charA (%s): %v", query, err)
			}
			if _, err := db.ExecContext(context.Background(), query, charB.ID); err != nil {
				t.Errorf("cleanup charB (%s): %v", query, err)
			}
		}
		if _, err := db.ExecContext(context.Background(), "DELETE FROM players WHERE id IN (?, ?)", charA.PlayerID, charB.PlayerID); err != nil {
			t.Errorf("cleanup players: %v", err)
		}
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	roomRepo, err := casino.NewValkeyRoomRepository(raw, casino.WithValkeyCharacterRepository(charRepo))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoRepo,
		casino.WithCharacterRepository(charRepo),
		casino.WithTransactionProvider(database.NewTransactionProvider(db)),
		casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.ExchangeGoldToCoins(ctx, charA.ID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.ExchangeGoldToCoins(ctx, charB.ID, 1000)
	if err != nil {
		t.Fatal(err)
	}

	hlRoom, err := svc.CreateRoom(ctx, charA.ID, casino.CreateRoomRequest{
		Name: "LeaseSafety-" + id.New()[:8], GameType: casino.GameTypeHighLow, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := roomRepo.DeleteRoom(context.Background(), hlRoom.Room.ID); err != nil {
			t.Errorf("cleanup hlRoom: %v", err)
		}
	})

	_, err = svc.JoinRoom(ctx, hlRoom.Room.ID, charB.ID, "", 0)
	if err != nil {
		t.Fatal(err)
	}

	type timingMetrics struct {
		mu                      sync.Mutex
		count                   int64
		maxHold                 time.Duration
		totalHold               time.Duration
		maxWait                 time.Duration
		totalWait               time.Duration
		lockAcquisitionFailures int64
	}
	var metrics timingMetrics

	cleanupObserver := casino.SetRoomLockObserverForTest(func(waitDur, holdDur time.Duration, acquired bool, err error) {
		metrics.mu.Lock()
		defer metrics.mu.Unlock()
		if !acquired {
			metrics.lockAcquisitionFailures++
			return
		}
		metrics.count++
		if holdDur > metrics.maxHold {
			metrics.maxHold = holdDur
		}
		metrics.totalHold += holdDur
		if waitDur > metrics.maxWait {
			metrics.maxWait = waitDur
		}
		metrics.totalWait += waitDur
	})
	t.Cleanup(cleanupObserver)

	// Run 20 rounds of HighLow with competing wagers per round
	for round := 0; round < 20; round++ {
		_, err := svc.StartHighLow(ctx, hlRoom.Room.ID, charA.ID)
		if err != nil {
			t.Fatalf("round %d StartHighLow: %v", round, err)
		}

		// Fixed cards so Actor 0 wins
		for i, id := range []string{charA.ID, charB.ID} {
			member, err := roomRepo.GetMember(ctx, hlRoom.Room.ID, id)
			if err != nil {
				t.Fatal(err)
			}
			member.Card = []int{12, 0}[i]
			if err := roomRepo.UpdateMember(ctx, *member); err != nil {
				t.Fatal(err)
			}
		}

		// Competing plays: 2 turns from Actor 0, 1 from Actor 1
		errs := make([]error, 3)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, errs[0] = svc.PlayHighLowAction(ctx, hlRoom.Room.ID, charA.ID, casino.HighLowActionHigh)
		}()
		go func() {
			defer wg.Done()
			_, errs[1] = svc.PlayHighLowAction(ctx, hlRoom.Room.ID, charA.ID, casino.HighLowActionHigh)
		}()
		go func() {
			defer wg.Done()
			_, errs[2] = svc.PlayHighLowAction(ctx, hlRoom.Room.ID, charB.ID, casino.HighLowActionHigh)
		}()
		wg.Wait()

		for _, e := range errs {
			if e != nil && !errors.Is(e, casino.ErrAlreadyActed) && !errors.Is(e, casino.ErrGameNotInRound) {
				t.Fatalf("round %d unexpected play error: %v", round, e)
			}
		}
	}

	metrics.mu.Lock()
	count := metrics.count
	maxHold := metrics.maxHold
	avgHold := time.Duration(0)
	maxWait := metrics.maxWait
	avgWait := time.Duration(0)
	if count > 0 {
		avgHold = metrics.totalHold / time.Duration(count)
		avgWait = metrics.totalWait / time.Duration(count)
	}
	failures := metrics.lockAcquisitionFailures
	metrics.mu.Unlock()

	t.Logf("Room lock timing metrics (20 rounds, %d acquisitions): maxHold=%v, avgHold=%v, maxWait=%v, avgWait=%v, lockAcquisitionFailures=%d",
		count, maxHold, avgHold, maxWait, avgWait, failures)

	if failures != 0 {
		t.Fatalf("expected 0 lock acquisition failures, got %d", failures)
	}

	// Timing metrics above are logged for informational verification of critical section
	// hold durations and wait times. Per .agents/rules/01-development-workflow.md, wall-clock
	// performance thresholds are omitted to prevent test flakiness under CI scheduling load.
}

func TestValkeyRoomLock_HonorsCallerDeadlineBeyondDefault(t *testing.T) {
	raw := openIsolatedTestValkey(t, 11)
	t.Cleanup(raw.Close)

	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}
	roomID := "long-deadline-room-" + id.New()[:8]
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(casino.DefaultRoomLockKeyPrefix+roomID).Build()).Error(); err != nil {
			t.Errorf("cleanup lock key: %v", err)
		}
	})

	held := make(chan struct{})
	holderErr := make(chan error, 1)
	go func() {
		holderErr <- repo.WithRoomLock(context.Background(), roomID, func(context.Context) error {
			close(held)
			time.Sleep(casino.DefaultRoomLockTimeout + 300*time.Millisecond)
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-holderErr:
		t.Fatalf("holder returned before running its callback: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("holder did not acquire the lock")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := repo.WithRoomLock(ctx, roomID, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("waiter with 30s deadline must outwait the default timeout, got: %v", err)
	}
	if err := <-holderErr; err != nil {
		t.Fatalf("holder: %v", err)
	}
}
