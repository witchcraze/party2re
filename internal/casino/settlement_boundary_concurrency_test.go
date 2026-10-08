package casino_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

// concurrentFaultValkeyClient intercepts Valkey commands and injects an active-index ZADD failure
// only for designated room IDs during final showdown room reset.
type concurrentFaultValkeyClient struct {
	valkeygo.Client
	failRooms     sync.Map
	injectedCount atomic.Int64
}

func (c *concurrentFaultValkeyClient) ArmRoomZaddFailure(roomID string) {
	c.failRooms.Store(roomID, true)
}

func (c *concurrentFaultValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	cmds := cmd.Commands()
	if len(cmds) > 0 && cmds[0] == "ZADD" && len(cmds) >= 4 {
		// ZADD <key> <score> <member>
		member := cmds[3]
		if _, ok := c.failRooms.Load(member); ok {
			c.failRooms.Delete(member)
			c.injectedCount.Add(1)
			return valkeygo.NewErrorResult(errors.New("injected active-index ZADD failure on concurrent showdown reset"))
		}
	}
	return c.Client.Do(ctx, cmd)
}

func (c *concurrentFaultValkeyClient) DoMulti(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
	results := make([]valkeygo.ValkeyResult, len(multi))
	for i, cmd := range multi {
		results[i] = c.Do(ctx, cmd)
	}
	return results
}

// TestSettlementBoundary_ConcurrencyStress verifies coin conservation and absence of deadlocks
// across concurrent Indian Poker games with interleaved secondary synchronization failures.
func TestSettlementBoundary_ConcurrencyStress(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rawValkey := openIsolatedTestValkey(t, 12)
	defer rawValkey.Close()

	ctx := context.Background()
	_ = rawValkey.Do(ctx, rawValkey.B().Flushdb().Build())

	faultClient := &concurrentFaultValkeyClient{Client: rawValkey}
	roomRepo, err := casino.NewValkeyRoomRepository(faultClient)
	if err != nil {
		t.Fatal(err)
	}
	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(txProvider),
		casino.WithCharacterRepository(charRepo),
		casino.WithRoomRepository(roomRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	cfg := testutil.GetStressConfig()
	numWorkers := cfg.Workers

	type charPair struct {
		charAID string
		charBID string
	}
	pairs := make([]charPair, numWorkers)
	var allCharIDs []string
	var allPlayerIDs []string

	const initialCoins = int64(100000)
	for w := 0; w < numWorkers; w++ {
		cA, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("ConcPokerA_%d_%d", w, time.Now().UnixNano()%1000000), 2000000)
		if err != nil {
			t.Fatal(err)
		}
		cB, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("ConcPokerB_%d_%d", w, time.Now().UnixNano()%1000000), 2000000)
		if err != nil {
			t.Fatal(err)
		}
		pairs[w] = charPair{charAID: cA.ID, charBID: cB.ID}
		allCharIDs = append(allCharIDs, cA.ID, cB.ID)
		allPlayerIDs = append(allPlayerIDs, cA.PlayerID, cB.PlayerID)

		if _, _, err := svc.ExchangeGoldToCoins(ctx, cA.ID, initialCoins); err != nil {
			t.Fatal(err)
		}
		if _, _, err := svc.ExchangeGoldToCoins(ctx, cB.ID, initialCoins); err != nil {
			t.Fatal(err)
		}
	}

	defer func() {
		for _, cID := range allCharIDs {
			//lint:ignore error-swallow test cleanup
			_, _ = db.ExecContext(ctx, "DELETE FROM casino_accounts WHERE character_id = ?", cID)
			//lint:ignore error-swallow test cleanup
			_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", cID)
		}
		for _, pID := range allPlayerIDs {
			//lint:ignore error-swallow test cleanup
			_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", pID)
		}
		//lint:ignore error-swallow test cleanup
		_ = rawValkey.Do(ctx, rawValkey.B().Flushdb().Build())
	}()

	totalInitialExpectedCoins := initialCoins * 2 * int64(numWorkers)

	res := testutil.RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		p := pairs[workerID]
		roomName := fmt.Sprintf("StressPoker-%d-%d-%d", workerID, op, time.Now().UnixNano()%1000000)
		createdRoom, err := svc.CreateRoom(ctx, p.charAID, casino.CreateRoomRequest{
			GameType:   casino.GameTypeIndian,
			Name:       roomName,
			Speed:      12,
			Rate:       10,
			MaxPlayers: 2,
		})
		if err != nil {
			return fmt.Errorf("worker %d op %d CreateRoom failed: %w", workerID, op, err)
		}
		roomID := createdRoom.Room.ID
		defer func() {
			//lint:ignore error-swallow test room cleanup
			_ = roomRepo.DeleteRoom(ctx, roomID)
		}()

		if _, err := svc.JoinRoom(ctx, roomID, p.charBID, "", 0); err != nil {
			return fmt.Errorf("worker %d op %d JoinRoom failed: %w", workerID, op, err)
		}

		if _, err := svc.StartIndianPoker(ctx, roomID, p.charAID); err != nil {
			return fmt.Errorf("worker %d op %d StartIndianPoker failed: %w", workerID, op, err)
		}

		// Fixed cards so charA always wins
		mA, err := roomRepo.GetMember(ctx, roomID, p.charAID)
		if err != nil {
			return err
		}
		mA.Card = 12
		if err := roomRepo.UpdateMember(ctx, *mA); err != nil {
			return err
		}

		mB, err := roomRepo.GetMember(ctx, roomID, p.charBID)
		if err != nil {
			return err
		}
		mB.Card = 1
		if err := roomRepo.UpdateMember(ctx, *mB); err != nil {
			return err
		}

		shouldInjectFault := (workerID+op)%2 == 1
		if shouldInjectFault {
			faultClient.ArmRoomZaddFailure(roomID)
		}

		// Run concurrent Showdown actions for charA and charB in parallel
		var showdownErrA, showdownErrB error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, showdownErrA = svc.PlayIndianPokerAction(ctx, roomID, p.charAID, casino.ActionShowdown)
		}()
		go func() {
			defer wg.Done()
			_, showdownErrB = svc.PlayIndianPokerAction(ctx, roomID, p.charBID, casino.ActionShowdown)
		}()
		wg.Wait()

		if shouldInjectFault {
			// One of the actions must have returned ErrSecondarySync during showdown resolution
			if !errors.Is(showdownErrA, casino.ErrSecondarySync) && !errors.Is(showdownErrB, casino.ErrSecondarySync) {
				return fmt.Errorf("expected ErrSecondarySync on faulted run, got errA=%v, errB=%v", showdownErrA, showdownErrB)
			}
		} else {
			if showdownErrA != nil {
				return fmt.Errorf("unexpected charA showdown err: %w", showdownErrA)
			}
			if showdownErrB != nil {
				return fmt.Errorf("unexpected charB showdown err: %w", showdownErrB)
			}
		}

		// Verify pair coin conservation immediately
		accA, err := casinoRepo.GetAccount(ctx, p.charAID)
		if err != nil {
			return err
		}
		accB, err := casinoRepo.GetAccount(ctx, p.charBID)
		if err != nil {
			return err
		}
		if accA.Coins+accB.Coins != initialCoins*2 {
			return fmt.Errorf("worker %d op %d coins not conserved: A=%d B=%d sum=%d expected=%d",
				workerID, op, accA.Coins, accB.Coins, accA.Coins+accB.Coins, initialCoins*2)
		}

		return nil
	})

	if res.Deadlocks > 0 {
		t.Fatalf("concurrency stress test encountered %d deadlocks", res.Deadlocks)
	}
	if res.Failures > 0 {
		t.Fatalf("concurrency stress test encountered %d failures", res.Failures)
	}

	injected := faultClient.injectedCount.Load()
	if injected == 0 {
		t.Fatalf("expected injected secondary sync failures, but none occurred")
	}

	// Final global coin conservation check across all characters in MariaDB
	var totalFinalCoins int64
	for _, cID := range allCharIDs {
		acc, err := casinoRepo.GetAccount(ctx, cID)
		if err != nil {
			t.Fatal(err)
		}
		totalFinalCoins += acc.Coins
	}

	if totalFinalCoins != totalInitialExpectedCoins {
		t.Fatalf("total coins not conserved: expected %d, got %d", totalInitialExpectedCoins, totalFinalCoins)
	}

	t.Logf("Concurrency Stress Test Succeeded: %d ops (%d workers, %d injected secondary sync failures), total coins %d strictly conserved with 0 deadlocks in %v",
		res.TotalOps, numWorkers, injected, totalFinalCoins, res.Duration)
}
