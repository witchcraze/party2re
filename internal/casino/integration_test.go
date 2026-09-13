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

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/database"
)

func TestCasinoRoomMultiplayerDatabaseIntegration(t *testing.T) {
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
	roomRepo, err := database.NewCasinoRoomRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	svc, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(txProvider),
		casino.WithRoomRepository(roomRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create 2 test characters with funds
	char1, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerHost", 10000)
	if err != nil {
		t.Fatal(err)
	}
	char2, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerGuest", 10000)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Buy casino coins (costs 20 gold per coin)
	if _, _, err := svc.ExchangeGoldToCoins(ctx, char1.ID, 500); err != nil {
		t.Fatalf("ExchangeGoldToCoins char1 failed: %v", err)
	}
	if _, _, err := svc.ExchangeGoldToCoins(ctx, char2.ID, 500); err != nil {
		t.Fatalf("ExchangeGoldToCoins char2 failed: %v", err)
	}

	roomName := fmt.Sprintf("IndianRoom-%d", time.Now().UnixNano()%1000000000)
	room, err := svc.CreateRoom(ctx, char1.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeIndian,
		Name:       roomName,
		Speed:      12,
		Rate:       10,
		MaxPlayers: 4,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// 4. char2 joins room
	if _, err := svc.JoinRoom(ctx, room.Room.ID, char2.ID, "", 0); err != nil {
		t.Fatalf("JoinRoom char2 failed: %v", err)
	}

	// 5. Start Indian Poker Game
	roomDetail, err := svc.StartIndianPoker(ctx, room.Room.ID, char1.ID)
	if err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}
	if roomDetail.Room.Round != 1 || roomDetail.Room.Pot != 0 {
		t.Fatalf("unexpected room state after start: round=%d pot=%d", roomDetail.Room.Round, roomDetail.Room.Pot)
	}

	// 6. Check forehead card masking: char1 cannot see own card, can see char2's
	detail1, err := svc.GetRoomDetail(ctx, room.Room.ID, char1.ID)
	if err != nil {
		t.Fatalf("GetRoomDetail char1 failed: %v", err)
	}
	for _, m := range detail1.Members {
		if m.CharacterID == char1.ID && m.Card != -1 {
			t.Errorf("expected char1's card to be masked (-1), got %d", m.Card)
		}
		if m.CharacterID == char2.ID && m.Card < 0 {
			t.Errorf("expected char2's card to be visible to char1, got %d", m.Card)
		}
	}

	// 7. char1 and char2 play Showdown action to conclude round
	_, err = svc.PlayIndianPokerAction(ctx, room.Room.ID, char1.ID, casino.ActionShowdown)
	if err != nil {
		t.Fatalf("PlayIndianPokerAction char1 showdown failed: %v", err)
	}
	roomDetail, err = svc.PlayIndianPokerAction(ctx, room.Room.ID, char2.ID, casino.ActionShowdown)
	if err != nil {
		t.Fatalf("PlayIndianPokerAction char2 showdown failed: %v", err)
	}
	if roomDetail.Room.Round != 0 {
		t.Errorf("expected round to reset to 0 after showdown, got %d", roomDetail.Room.Round)
	}

	// Verify accounts in DB
	acc1, _ := svc.GetAccount(ctx, char1.ID)
	acc2, _ := svc.GetAccount(ctx, char2.ID)
	if acc1.Coins+acc2.Coins != 1000 {
		t.Errorf("expected total 1000 coins conserved, got %d + %d = %d", acc1.Coins, acc2.Coins, acc1.Coins+acc2.Coins)
	}
}

func TestCasinoPrizeDepotDatabaseIntegration(t *testing.T) {
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
	depotRepo, err := database.NewDepotRepository(db)
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
		casino.WithDepotRepository(depotRepo),
		casino.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "PrizeBuyer", 10000)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Buy 500 casino coins (costs 10,000 gold)
	acc, _, err := svc.ExchangeGoldToCoins(ctx, char.ID, 500)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 500 {
		t.Fatalf("expected 500 coins, got %d", acc.Coins)
	}

	// 3. Exchange 100 coins for prize item-004 (まほうの小ビン)
	res, err := svc.ExchangePrize(ctx, char.ID, 100, 1)
	if err != nil {
		t.Fatalf("ExchangePrize failed: %v", err)
	}
	if res.RemainingCoins != 400 || !res.TransferredToDepot {
		t.Fatalf("unexpected result: %+v", res)
	}

	// 4. Verify item was saved in depot table
	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, char.ID)
	if err != nil {
		t.Fatalf("depot not found: %v", err)
	}
	found := false
	for _, it := range dep.Items {
		if it.DefinitionID == "item-004" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected item-004 to be in depot, got items: %+v", dep.Items)
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
	roomRepo, err := database.NewCasinoRoomRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	svc, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(txProvider),
		casino.WithRoomRepository(roomRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create 2 players with funds
	char1, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerConcurHost", 10000)
	if err != nil {
		t.Fatal(err)
	}
	char2, err := database.CreateTestCharacterWithFunds(ctx, db, "PokerConcurGuest", 10000)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := svc.ExchangeGoldToCoins(ctx, char1.ID, 500); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ExchangeGoldToCoins(ctx, char2.ID, 500); err != nil {
		t.Fatal(err)
	}

	// 2. Create room & join & start game
	roomName := fmt.Sprintf("ConcurPoker-%d", time.Now().UnixNano()%1000000000)
	room, err := svc.CreateRoom(ctx, char1.ID, casino.CreateRoomRequest{
		GameType:   casino.GameTypeIndian,
		Name:       roomName,
		Speed:      12,
		Rate:       10,
		MaxPlayers: 4,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if _, err := svc.JoinRoom(ctx, room.Room.ID, char2.ID, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartIndianPoker(ctx, room.Room.ID, char1.ID); err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}

	// 3. Concurrently launch 30 goroutines attempting action on behalf of char1
	const concurrentRequests = 30
	var wg sync.WaitGroup
	var successCount int64
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

			_, err := svc.PlayIndianPokerAction(ctx, room.Room.ID, char1.ID, action)
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, casino.ErrAlreadyActed) || errors.Is(err, casino.ErrGameNotInRound) {
				atomic.AddInt64(&rejectedCount, 1)
			} else {
				t.Errorf("unexpected error during concurrent poker action: %v", err)
			}
		}(i)
	}

	close(startSignal)
	wg.Wait()

	// 4. Assert total attempts: exactly 1 action succeeds, others rejected
	totalProcessed := successCount + rejectedCount
	if totalProcessed != concurrentRequests {
		t.Errorf("total requests processed = %d, want %d", totalProcessed, concurrentRequests)
	}
	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount)
	}

	// 5. Verify database coins balance is non-negative and consistent
	dbAcc, err := svc.GetAccount(ctx, char1.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if dbAcc.Coins < 0 {
		t.Fatalf("balance became negative: %d", dbAcc.Coins)
	}
}
