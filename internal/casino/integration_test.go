package casino_test

import (
	"context"
	"os"
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

	svc, err := casino.NewService(casinoRepo)
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

	// 4. Play game through to completion
	for game.Status == casino.StatusInProgress {
		action := casino.ActionCall
		if game.Round >= 2 {
			action = casino.ActionShowdown
		}
		acc, err = svc.PlayIndianPokerRound(ctx, char.ID, game, action)
		if err != nil {
			t.Fatalf("PlayIndianPokerRound failed: %v", err)
		}
	}

	// 5. Verify game finished and account coins are consistent
	if game.Status == casino.StatusInProgress {
		t.Error("game should be finished")
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
