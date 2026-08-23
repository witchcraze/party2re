package depot_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestDepotIntegrationGoldAndItemOperations(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	charService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	createdChar, err := charService.Create(ctx, "Depot Integrator")
	if err != nil {
		t.Fatal(err)
	}

	depotService, err := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial GetDepot
	initialDep, err := depotService.GetDepot(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("GetDepot error = %v", err)
	}
	if initialDep.Gold != 0 || len(initialDep.Items) != 0 {
		t.Fatalf("initial depot not empty: %#v", initialDep)
	}

	// 2. Deposit Gold
	dep, err := depotService.DepositGold(ctx, createdChar.ID, 120)
	if err != nil {
		t.Fatalf("DepositGold error = %v", err)
	}
	if dep.Gold != 120 {
		t.Errorf("depot gold = %d, want 120", dep.Gold)
	}

	// 3. Deposit an item
	inv, _ := invRepo.FindByCharacterID(ctx, createdChar.ID)
	potion, _ := item.NewInstance("item-001", 3)
	_ = inv.Add(potion)
	_ = invRepo.Save(ctx, inv)

	dep, err = depotService.DepositItem(ctx, createdChar.ID, potion.ID)
	if err != nil {
		t.Fatalf("DepositItem error = %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-001" {
		t.Fatalf("unexpected depot items: %#v", dep.Items)
	}

	// 4. Withdraw Item
	dep, err = depotService.WithdrawItem(ctx, createdChar.ID, potion.ID)
	if err != nil {
		t.Fatalf("WithdrawItem error = %v", err)
	}
	if len(dep.Items) != 0 {
		t.Fatalf("depot items count = %d, want 0", len(dep.Items))
	}

	// 5. Withdraw Gold
	dep, err = depotService.WithdrawGold(ctx, createdChar.ID, 50)
	if err != nil {
		t.Fatalf("WithdrawGold error = %v", err)
	}
	if dep.Gold != 70 {
		t.Errorf("depot gold = %d, want 70", dep.Gold)
	}

	// Verify database persistence
	restoredChar, _ := charRepo.FindByID(ctx, createdChar.ID)
	expectedMoney := 200 - 120 + 50
	if restoredChar.Money != expectedMoney {
		t.Errorf("character money = %d, want %d", restoredChar.Money, expectedMoney)
	}
}

func TestConcurrentDepotGoldWithdrawal(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, _ := database.NewCharacterRepository(db)
	invRepo, _ := database.NewInventoryRepository(db)
	depotRepo, _ := database.NewDepotRepository(db)

	char, _ := corecharacter.New("Concurrent Depot")
	char.Money = 500
	_ = charRepo.Save(ctx, char)

	depotService, _ := depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)

	// Deposit 100 gold
	_, err = depotService.DepositGold(ctx, char.ID, 100)
	if err != nil {
		t.Fatal(err)
	}

	// Attempt two concurrent withdrawals of 80 gold each (total 160 > 100)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := depotService.WithdrawGold(ctx, char.ID, 80)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	restoredDep, _ := depotRepo.FindByCharacterID(ctx, char.ID)
	if restoredDep.Gold < 0 {
		t.Fatalf("depot gold went negative: %d", restoredDep.Gold)
	}
}
