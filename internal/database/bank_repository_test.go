package database

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/bank"
)

func TestBankRepositoryDepositAndWithdraw(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bankRepo, err := NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 1. Create character with 10,000 gold
	char1, err := CreateTestCharacterWithFunds(ctx, db, "Banker 1", 10000)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Deposit 4,000 gold
	updatedChar, err := bankRepo.Deposit(ctx, char1.ID, 4000)
	if err != nil {
		t.Fatalf("Deposit error: %v", err)
	}
	if updatedChar.Money != 6000 || updatedChar.Deposit != 4000 {
		t.Errorf("deposit state: money=%d, deposit=%d", updatedChar.Money, updatedChar.Deposit)
	}

	// 3. Withdraw 1,500 gold
	withdrawnChar, actual, refunded, err := bankRepo.Withdraw(ctx, char1.ID, 1500)
	if err != nil {
		t.Fatalf("Withdraw error: %v", err)
	}
	if withdrawnChar.Money != 7500 || withdrawnChar.Deposit != 2500 || actual != 1500 || refunded != 0 {
		t.Errorf("withdraw state: money=%d, deposit=%d, actual=%d, refunded=%d", withdrawnChar.Money, withdrawnChar.Deposit, actual, refunded)
	}

	// 4. Test wallet clamp at 999,999 G:
	// Set money to 800,000 and deposit to 500,000
	_, err = db.ExecContext(ctx, "UPDATE characters SET money = 800000, deposit = 500000 WHERE id = ?", char1.ID)
	if err != nil {
		t.Fatal(err)
	}

	clampedChar, actual, refunded, err := bankRepo.Withdraw(ctx, char1.ID, 300000)
	if err != nil {
		t.Fatalf("Withdraw clamp error: %v", err)
	}
	if clampedChar.Money != bank.MaxWallet {
		t.Errorf("clamped money = %d, want %d", clampedChar.Money, bank.MaxWallet)
	}
	if clampedChar.Deposit != 300001 {
		t.Errorf("clamped deposit = %d, want 300001", clampedChar.Deposit)
	}
	if actual != 199999 || refunded != 100001 {
		t.Errorf("actual = %d, refunded = %d", actual, refunded)
	}
	if int64(clampedChar.Money)+clampedChar.Deposit != 800000+500000 {
		t.Errorf("gold conservation violated: %d", int64(clampedChar.Money)+clampedChar.Deposit)
	}
}
