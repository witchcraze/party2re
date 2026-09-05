package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
)

func TestBankRepositoryDepositWithdrawAndTransfer(t *testing.T) {
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

	// 1. Create two players and character
	player2, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	char1, err := CreateTestCharacterWithFunds(ctx, db, "Banker 1", 1000)
	if err != nil {
		t.Fatal(err)
	}
	player1ID := char1.PlayerID

	// 2. Deposit
	acc1, updatedChar1, err := bankRepo.Deposit(ctx, player1ID, char1.ID, 400)
	if err != nil {
		t.Fatalf("Deposit error: %v", err)
	}
	if acc1.Balance != 400 || updatedChar1.Money != 600 {
		t.Errorf("deposit state: acc1=%d, char1=%d", acc1.Balance, updatedChar1.Money)
	}

	// 3. Transfer from player1 to player2
	transferID := "t_" + char1.ID[:8]
	record := bank.TransferRecord{
		ID:           transferID,
		FromPlayerID: player1ID,
		ToPlayerID:   player2.ID,
		Amount:       250,
		CreatedAt:    time.Now().UTC(),
	}
	fromAcc, toAcc, err := bankRepo.Transfer(ctx, record)
	if err != nil {
		t.Fatalf("Transfer error: %v", err)
	}
	if fromAcc.Balance != 150 {
		t.Errorf("from balance = %d, want 150", fromAcc.Balance)
	}
	if toAcc.Balance != 250 {
		t.Errorf("to balance = %d, want 250", toAcc.Balance)
	}

	// 4. Withdraw from player1
	acc1AfterWithdraw, char1AfterWithdraw, err := bankRepo.Withdraw(ctx, player1ID, char1.ID, 100)
	if err != nil {
		t.Fatalf("Withdraw error: %v", err)
	}
	if acc1AfterWithdraw.Balance != 50 || char1AfterWithdraw.Money != 700 {
		t.Errorf("withdraw state: acc1=%d, char1=%d", acc1AfterWithdraw.Balance, char1AfterWithdraw.Money)
	}

	// 5. Check transfer history
	transfers, err := bankRepo.ListTransfers(ctx, player1ID, 10)
	if err != nil {
		t.Fatalf("ListTransfers error: %v", err)
	}
	if len(transfers) == 0 || transfers[0].ID != transferID {
		t.Fatalf("unexpected transfer history: %#v", transfers)
	}
}
