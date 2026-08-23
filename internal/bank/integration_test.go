package bank_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
)

func TestBankIntegrationConcurrentDepositsAndTransfers(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bankRepo, err := database.NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	playerRepo, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := bank.NewService(bankRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	char1, err := corecharacter.New("Depositor Concurrency")
	if err != nil {
		t.Fatal(err)
	}
	char1.Money = 10000
	if err := charRepo.Save(ctx, char1); err != nil {
		t.Fatal(err)
	}

	// Create player and characters
	player1, err := coreplayer.New("bank_c1_"+char1.ID[:8], "securepass", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, player1); err != nil {
		t.Fatal(err)
	}
	player2, err := coreplayer.New("bank_c2_"+char1.ID[:8], "securepass", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, player2); err != nil {
		t.Fatal(err)
	}

	// Deposit initial 5000
	_, _, err = service.Deposit(ctx, player1.ID, char1.ID, 5000)
	if err != nil {
		t.Fatalf("initial deposit error: %v", err)
	}

	// Run 10 concurrent transfers of 100G each from player1 to player2
	const concurrentTransfers = 10
	var wg sync.WaitGroup
	wg.Add(concurrentTransfers)

	for i := 0; i < concurrentTransfers; i++ {
		go func() {
			defer wg.Done()
			_, _, _, transferErr := service.Transfer(ctx, player1.ID, player2.ID, 100)
			if transferErr != nil {
				t.Errorf("concurrent transfer failed: %v", transferErr)
			}
		}()
	}
	wg.Wait()

	acc1, err := service.GetAccount(ctx, player1.ID)
	if err != nil {
		t.Fatalf("GetAccount(player1) error: %v", err)
	}
	acc2, err := service.GetAccount(ctx, player2.ID)
	if err != nil {
		t.Fatalf("GetAccount(player2) error: %v", err)
	}

	expectedAcc1 := int64(5000 - concurrentTransfers*100)
	expectedAcc2 := int64(concurrentTransfers * 100)

	if acc1.Balance != expectedAcc1 {
		t.Errorf("player1 balance = %d, want %d", acc1.Balance, expectedAcc1)
	}
	if acc2.Balance != expectedAcc2 {
		t.Errorf("player2 balance = %d, want %d", acc2.Balance, expectedAcc2)
	}
}
