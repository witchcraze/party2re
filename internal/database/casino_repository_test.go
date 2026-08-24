package database

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestCasinoRepositoryLifecycle(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := CreateTestCharacter(ctx, db, "CasinoPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Initial casino account should be 0 coins
	initialAcc, err := casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if initialAcc.Coins != 0 {
		t.Errorf("initial coins = %d, want 0", initialAcc.Coins)
	}

	// 3. Buy 100 coins (costs 2,000 gold)
	acc, updatedChar, err := casinoRepo.ExchangeGoldToCoins(ctx, char.ID, 100, 2000)
	if err != nil {
		t.Fatalf("ExchangeGoldToCoins failed: %v", err)
	}
	if acc.Coins != 100 || updatedChar.Money != 8000 {
		t.Errorf("coins = %d, money = %d, want 100 coins and 8000 gold", acc.Coins, updatedChar.Money)
	}

	// 4. Insufficient gold to buy coins should fail
	if _, _, err := casinoRepo.ExchangeGoldToCoins(ctx, char.ID, 5000, 100000); !errors.Is(err, casino.ErrInsufficientGold) {
		t.Errorf("insufficient gold err = %v, want %v", err, casino.ErrInsufficientGold)
	}

	// 5. Sell 30 coins (rewards 600 gold)
	acc, updatedChar, err = casinoRepo.ExchangeCoinsToGold(ctx, char.ID, 30, 600)
	if err != nil {
		t.Fatalf("ExchangeCoinsToGold failed: %v", err)
	}
	if acc.Coins != 70 || updatedChar.Money != 8600 {
		t.Errorf("coins = %d, money = %d, want 70 coins and 8600 gold", acc.Coins, updatedChar.Money)
	}

	// 6. Insufficient coins to sell should fail
	if _, _, err := casinoRepo.ExchangeCoinsToGold(ctx, char.ID, 500, 10000); !errors.Is(err, casino.ErrInsufficientCoins) {
		t.Errorf("insufficient coins err = %v, want %v", err, casino.ErrInsufficientCoins)
	}

	// 7. Adjust coins (deduct 20 -> 50 remaining)
	acc, err = casinoRepo.AdjustCoins(ctx, char.ID, -20)
	if err != nil {
		t.Fatalf("AdjustCoins deduct failed: %v", err)
	}
	if acc.Coins != 50 {
		t.Errorf("coins = %d, want 50", acc.Coins)
	}

	// 8. Adjust coins (add 150 -> 200 total)
	acc, err = casinoRepo.AdjustCoins(ctx, char.ID, 150)
	if err != nil {
		t.Fatalf("AdjustCoins add failed: %v", err)
	}
	if acc.Coins != 200 {
		t.Errorf("coins = %d, want 200", acc.Coins)
	}
}
