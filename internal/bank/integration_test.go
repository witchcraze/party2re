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

func TestBankIntegrationConcurrentDepositsAndWithdrawals(t *testing.T) {
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

	player1, err := coreplayer.New("bank_c1_"+time.Now().Format("20060102150405.000000"), "securepass", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, player1); err != nil {
		t.Fatal(err)
	}

	char1, err := corecharacter.New("Depositor Concurrency")
	if err != nil {
		t.Fatal(err)
	}
	char1.PlayerID = player1.ID
	char1.Money = 10000
	char1.Deposit = 10000
	if err := charRepo.Save(ctx, char1); err != nil {
		t.Fatal(err)
	}

	const concurrentOps = 10
	var wg sync.WaitGroup
	wg.Add(concurrentOps * 2)

	// 10 concurrent deposits of 100G
	for i := 0; i < concurrentOps; i++ {
		go func() {
			defer wg.Done()
			_, depErr := service.Deposit(ctx, char1.ID, 100)
			if depErr != nil {
				t.Errorf("concurrent deposit failed: %v", depErr)
			}
		}()
	}

	// 10 concurrent withdrawals of 100G
	for i := 0; i < concurrentOps; i++ {
		go func() {
			defer wg.Done()
			_, withErr := service.Withdraw(ctx, char1.ID, 100)
			if withErr != nil {
				t.Errorf("concurrent withdraw failed: %v", withErr)
			}
		}()
	}
	wg.Wait()

	state, err := service.GetState(ctx, char1.ID)
	if err != nil {
		t.Fatalf("GetState error: %v", err)
	}

	// Total gold must be conserved (initial 10000 + 10000 = 20000)
	totalGold := int64(state.Money) + state.Deposit
	if totalGold != 20000 {
		t.Errorf("total gold violated: got %d, want 20000 (money=%d, deposit=%d)", totalGold, state.Money, state.Deposit)
	}
}
