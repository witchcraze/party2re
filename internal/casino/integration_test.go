package casino_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
)

func TestCasinoIndianPokerDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	svc, err := casino.NewService(casinoRepo, casino.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "PokerPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Buy 500 casino coins (costs 10,000 gold)
	acc, updatedChar, err := svc.ExchangeGoldToCoins(ctx, char.ID, 500)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 500 || updatedChar.Money != 0 {
		t.Fatalf("unexpected exchange: coins=%d, money=%d", acc.Coins, updatedChar.Money)
	}

	// 3. Start Indian Poker Game with base rate 10 -> Ante 10 deducted (490 coins remaining)
	game, acc, err := svc.StartIndianPokerGame(ctx, char.ID, 10)
	if err != nil {
		t.Fatalf("StartIndianPokerGame failed: %v", err)
	}
	if acc.Coins != 490 || game.Pot != 20 {
		t.Fatalf("unexpected start state: coins=%d, pot=%d", acc.Coins, game.Pot)
	}
	if game.PlayerCard.Rank != 0 || game.PlayerCard.Suit != "?" {
		t.Errorf("expected masked player card in client view, got %+v", game.PlayerCard)
	}

	// Starting another session while active must fail with ErrActiveSessionExists
	_, _, err = svc.StartIndianPokerGame(ctx, char.ID, 10)
	if !errors.Is(err, casino.ErrActiveSessionExists) {
		t.Fatalf("expected ErrActiveSessionExists, got %v", err)
	}

	// Query active game state
	activeGame, activeAcc, err := svc.GetActiveIndianPokerGame(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetActiveIndianPokerGame failed: %v", err)
	}
	if activeGame.ID != game.ID || activeAcc.Coins != 490 {
		t.Fatalf("unexpected active game: %+v, coins=%d", activeGame, activeAcc.Coins)
	}
	if activeGame.PlayerCard.Rank != 0 {
		t.Errorf("active game player card must remain masked")
	}

	// 4. Play game through to completion using PlayIndianPokerAction
	for activeGame.Status == casino.StatusInProgress {
		action := casino.ActionCall
		if activeGame.Round >= 2 {
			action = casino.ActionShowdown
		}
		activeGame, acc, err = svc.PlayIndianPokerAction(ctx, char.ID, action)
		if err != nil {
			t.Fatalf("PlayIndianPokerAction failed: %v", err)
		}
	}

	// 5. Verify game finished and account coins are consistent
	if activeGame.Status == casino.StatusInProgress {
		t.Error("game should be finished")
	}
	// Finished game reveals player card
	if activeGame.PlayerCard.Rank == 0 {
		t.Errorf("completed game player card must be revealed")
	}

	// Querying active game now returns ErrNoActivePokerGame
	_, _, err = svc.GetActiveIndianPokerGame(ctx, char.ID)
	if !errors.Is(err, casino.ErrNoActivePokerGame) {
		t.Fatalf("expected ErrNoActivePokerGame, got %v", err)
	}

	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Coins != acc.Coins {
		t.Errorf("db coins = %d, service returned coins = %d", dbAcc.Coins, acc.Coins)
	}

	// 6. Sell all remaining coins back to gold
	if acc.Coins > 0 {
		soldAcc, finalChar, err := svc.ExchangeCoinsToGold(ctx, char.ID, acc.Coins)
		if err != nil {
			t.Fatalf("ExchangeCoinsToGold failed: %v", err)
		}
		if soldAcc.Coins != 0 || finalChar.Money <= 0 {
			t.Errorf("final state: coins=%d, money=%d", soldAcc.Coins, finalChar.Money)
		}
	}
}

func TestCasinoSlotMachineDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "SlotPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Buy 100 coins
	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 100)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 100 {
		t.Fatalf("coins = %d, want 100", acc.Coins)
	}

	// 3. Spin with 10 coins
	res, updatedAcc, err := svc.SpinSlot(ctx, char.ID, 10)
	if err != nil {
		t.Fatalf("SpinSlot failed: %v", err)
	}
	if res.BetCoins != 10 {
		t.Errorf("bet = %d, want 10", res.BetCoins)
	}
	if updatedAcc.Coins != 100+res.NetCoins {
		t.Errorf("updated coins = %d, want %d", updatedAcc.Coins, 100+res.NetCoins)
	}

	// 4. Verify durable persistence
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Coins != updatedAcc.Coins {
		t.Errorf("db coins = %d, memory coins = %d", dbAcc.Coins, updatedAcc.Coins)
	}
}

func TestCasinoDoppelDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "DoppelPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Buy 100 coins
	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 100)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 100 {
		t.Fatalf("coins = %d, want 100", acc.Coins)
	}

	// 3. Play Doppel with 20 coins, pool size 6
	res, updatedAcc, err := svc.PlayDoppel(ctx, char.ID, 20, 6, casino.MarkStar)
	if err != nil {
		t.Fatalf("PlayDoppel failed: %v", err)
	}
	if res.BetCoins != 20 || res.PoolSize != 6 {
		t.Errorf("res = %+v", res)
	}
	if updatedAcc.Coins != 100+res.NetCoins {
		t.Errorf("updated coins = %d, want %d", updatedAcc.Coins, 100+res.NetCoins)
	}

	// 4. Verify durable persistence
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Coins != updatedAcc.Coins {
		t.Errorf("db coins = %d, memory coins = %d", dbAcc.Coins, updatedAcc.Coins)
	}
}

func TestCasinoHighLowDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character
	char, err := database.CreateTestCharacter(ctx, db, "HighLowPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Buy 100 coins
	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 100)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 100 {
		t.Fatalf("coins = %d, want 100", acc.Coins)
	}

	// 3. Play High & Low with 30 coins, guessing HIGH
	res, updatedAcc, err := svc.PlayHighLow(ctx, char.ID, 30, casino.GuessHigh)
	if err != nil {
		t.Fatalf("PlayHighLow failed: %v", err)
	}
	if res.BetCoins != 30 {
		t.Errorf("res = %+v", res)
	}
	if updatedAcc.Coins != 100+res.NetCoins {
		t.Errorf("updated coins = %d, want %d", updatedAcc.Coins, 100+res.NetCoins)
	}

	// 4. Verify DB account is updated
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Coins != updatedAcc.Coins {
		t.Errorf("db coins = %d, service coins = %d", dbAcc.Coins, updatedAcc.Coins)
	}
}

func TestCasinoSlotMachine_ConcurrencyExploitPrevented(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create player with 10 coins (exactly enough for 1 spin at bet=10)
	char, err := database.CreateTestCharacter(ctx, db, "SlotConcurrencyUser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 200, char.ID); err != nil {
		t.Fatal(err)
	}

	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 10)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 10 {
		t.Fatalf("initial coins = %d, want 10", acc.Coins)
	}

	// 2. Launch 100 concurrent spin requests each betting 10 coins
	const concurrentRequests = 100
	var wg sync.WaitGroup
	var successCount int64
	var insufficientErrCount int64
	var totalPayout int64
	var totalBetDeducted int64

	startSignal := make(chan struct{})

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			res, _, err := svc.SpinSlot(ctx, char.ID, 10)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&totalPayout, res.PayoutCoins)
				atomic.AddInt64(&totalBetDeducted, res.BetCoins)
			} else if errors.Is(err, casino.ErrInsufficientCoins) {
				atomic.AddInt64(&insufficientErrCount, 1)
			} else {
				t.Errorf("unexpected error during concurrent spin: %v", err)
			}
		}()
	}

	// Trigger all requests simultaneously
	close(startSignal)
	wg.Wait()

	// 3. Verify that total requests == success + insufficient errors
	if successCount+insufficientErrCount != concurrentRequests {
		t.Errorf("total requests processed = %d, want %d (success=%d, insufficient=%d)",
			successCount+insufficientErrCount, concurrentRequests, successCount, insufficientErrCount)
	}

	// 4. Verify DB balance strictly matches: 10 initial coins - totalBetDeducted + totalPayout
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	expectedBalance := int64(10) - totalBetDeducted + totalPayout
	if dbAcc.Coins != expectedBalance {
		t.Fatalf("DB coins mismatch: got %d, expected strictly %d (initial=10, bet=%d, payout=%d, success=%d, rejected=%d)",
			dbAcc.Coins, expectedBalance, totalBetDeducted, totalPayout, successCount, insufficientErrCount)
	}

	// Balance must be non-negative
	if dbAcc.Coins < 0 {
		t.Fatalf("balance became negative: %d", dbAcc.Coins)
	}
}

func TestCasinoDoppel_ConcurrencyExploitPrevented(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create player with 20 coins (exactly enough for 1 play at bet=20)
	char, err := database.CreateTestCharacter(ctx, db, "DoppelConcurrencyUser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 400, char.ID); err != nil {
		t.Fatal(err)
	}

	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 20)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 20 {
		t.Fatalf("initial coins = %d, want 20", acc.Coins)
	}

	// 2. Launch 100 concurrent Doppel requests
	const concurrentRequests = 100
	var wg sync.WaitGroup
	var successCount int64
	var insufficientErrCount int64
	var totalPayout int64
	var totalBetDeducted int64

	startSignal := make(chan struct{})

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			res, _, err := svc.PlayDoppel(ctx, char.ID, 20, 6, casino.MarkStar)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&totalPayout, res.PayoutCoins)
				atomic.AddInt64(&totalBetDeducted, res.BetCoins)
			} else if errors.Is(err, casino.ErrInsufficientCoins) {
				atomic.AddInt64(&insufficientErrCount, 1)
			} else {
				t.Errorf("unexpected error during concurrent doppel: %v", err)
			}
		}()
	}

	// Trigger all requests simultaneously
	close(startSignal)
	wg.Wait()

	// 3. Verify total requests processed
	if successCount+insufficientErrCount != concurrentRequests {
		t.Errorf("total requests processed = %d, want %d", successCount+insufficientErrCount, concurrentRequests)
	}

	// 4. Verify DB balance consistency
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	expectedBalance := int64(20) - totalBetDeducted + totalPayout
	if dbAcc.Coins != expectedBalance {
		t.Fatalf("DB coins mismatch: got %d, expected strictly %d", dbAcc.Coins, expectedBalance)
	}
	if dbAcc.Coins < 0 {
		t.Fatalf("balance became negative: %d", dbAcc.Coins)
	}
}

func TestCasinoHighLow_ConcurrencyExploitPrevented(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create player with 50 coins (enough for 1 bet=50 game)
	char, err := database.CreateTestCharacter(ctx, db, "HighLowConcurrencyUser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 1000, char.ID); err != nil {
		t.Fatal(err)
	}

	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 50)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 50 {
		t.Fatalf("initial coins = %d, want 50", acc.Coins)
	}

	// 2. Launch 100 concurrent HighLow requests
	const concurrentRequests = 100
	var wg sync.WaitGroup
	var successCount int64
	var insufficientErrCount int64
	var totalPayout int64
	var totalBetDeducted int64

	startSignal := make(chan struct{})

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			res, _, err := svc.PlayHighLow(ctx, char.ID, 50, casino.GuessHigh)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&totalPayout, res.PayoutCoins)
				atomic.AddInt64(&totalBetDeducted, res.BetCoins)
			} else if errors.Is(err, casino.ErrInsufficientCoins) {
				atomic.AddInt64(&insufficientErrCount, 1)
			} else {
				t.Errorf("unexpected error during concurrent highlow: %v", err)
			}
		}()
	}

	// Trigger all requests simultaneously
	close(startSignal)
	wg.Wait()

	// 3. Verify total requests processed
	if successCount+insufficientErrCount != concurrentRequests {
		t.Errorf("total requests processed = %d, want %d", successCount+insufficientErrCount, concurrentRequests)
	}

	// 4. Verify DB balance consistency
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}

	expectedBalance := int64(50) - totalBetDeducted + totalPayout
	if dbAcc.Coins != expectedBalance {
		t.Fatalf("DB coins mismatch: got %d, expected strictly %d", dbAcc.Coins, expectedBalance)
	}
	if dbAcc.Coins < 0 {
		t.Fatalf("balance became negative: %d", dbAcc.Coins)
	}
}

func TestCasinoIndianPoker_ConcurrencyExploitPrevented(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	svc, err := casino.NewService(casinoRepo, casino.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create player with 500 coins
	char, err := database.CreateTestCharacter(ctx, db, "PokerConcurrencyUser")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 500)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 500 {
		t.Fatalf("initial coins = %d, want 500", acc.Coins)
	}

	// 2. Start game with rate 10 (ante 10 deducted, 490 coins remaining)
	_, acc, err = svc.StartIndianPokerGame(ctx, char.ID, 10)
	if err != nil {
		t.Fatalf("StartIndianPokerGame failed: %v", err)
	}
	if acc.Coins != 490 {
		t.Fatalf("coins after ante = %d, want 490", acc.Coins)
	}

	// 3. Concurrently launch 50 goroutines attempting actions on the same poker session
	const concurrentRequests = 50
	var wg sync.WaitGroup
	var callSuccessCount int64
	var showdownSuccessCount int64
	var rejectedCount int64

	startSignal := make(chan struct{})

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startSignal

			action := casino.ActionCall
			if idx%2 == 0 {
				action = casino.ActionShowdown
			}

			_, _, err := svc.PlayIndianPokerAction(ctx, char.ID, action)
			if err == nil {
				if action == casino.ActionCall {
					atomic.AddInt64(&callSuccessCount, 1)
				} else {
					atomic.AddInt64(&showdownSuccessCount, 1)
				}
			} else if errors.Is(err, casino.ErrNoActivePokerGame) || errors.Is(err, casino.ErrGameAlreadyOver) {
				atomic.AddInt64(&rejectedCount, 1)
			} else {
				t.Errorf("unexpected error during concurrent poker action: %v", err)
			}
		}(i)
	}

	close(startSignal)
	wg.Wait()

	// 4. Assert total attempts
	totalProcessed := callSuccessCount + showdownSuccessCount + rejectedCount
	if totalProcessed != concurrentRequests {
		t.Errorf("total requests processed = %d, want %d", totalProcessed, concurrentRequests)
	}

	// 5. Verify database coins balance is non-negative and consistent
	dbAcc, err := svc.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Coins < 0 {
		t.Fatalf("balance became negative: %d", dbAcc.Coins)
	}

	// Active game should either be in progress or completed without error
	activeGame, _, err := svc.GetActiveIndianPokerGame(ctx, char.ID)
	if err != nil && !errors.Is(err, casino.ErrNoActivePokerGame) {
		t.Fatalf("unexpected error checking active game: %v", err)
	}
	_ = activeGame
}
