package shop_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/shop"
)

func TestShopIntegrationPurchaseAndSell(t *testing.T) {
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

	charService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	createdChar, err := charService.Create(ctx, player.ID, "Shop Integrator")
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	shopService, err := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	// Initial character has starting money of 200
	// Let's purchase a club (price in weapons.json) or herb (price in consumables.json)
	defs := catalog.Definitions()
	if len(defs) == 0 {
		t.Fatal("empty item catalog")
	}
	var testItem item.Definition
	for _, d := range defs {
		if d.Price > 0 && d.Price <= 100 {
			testItem = d
			break
		}
	}
	if testItem.ID == "" {
		t.Fatal("no suitable test item found under 100 gold")
	}

	// 1. Purchase item
	purchResult, err := shopService.Purchase(ctx, createdChar.ID, testItem.ID, 1)
	if err != nil {
		t.Fatalf("Purchase() error = %v", err)
	}
	expectedMoney := 200 - testItem.Price
	if purchResult.Character.Money != expectedMoney {
		t.Fatalf("purchased Character.Money = %d, want %d", purchResult.Character.Money, expectedMoney)
	}

	// Verify database persistence
	restoredChar, err := charRepo.FindByID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredChar.Money != expectedMoney {
		t.Errorf("restored Character.Money = %d, want %d", restoredChar.Money, expectedMoney)
	}

	restoredInv, err := invRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredInv.Items) != 1 || restoredInv.Items[0].DefinitionID != testItem.ID {
		t.Fatalf("restored Inventory = %#v", restoredInv)
	}

	// 2. Sell item back
	itemInstanceID := restoredInv.Items[0].ID
	sellResult, err := shopService.Sell(ctx, createdChar.ID, itemInstanceID, 1)
	if err != nil {
		t.Fatalf("Sell() error = %v", err)
	}
	expectedSellPayout := testItem.Price / 2
	expectedAfterSellMoney := expectedMoney + expectedSellPayout
	if sellResult.Character.Money != expectedAfterSellMoney {
		t.Fatalf("sell Character.Money = %d, want %d", sellResult.Character.Money, expectedAfterSellMoney)
	}

	// Verify database persistence after sale
	restoredCharAfterSell, err := charRepo.FindByID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredCharAfterSell.Money != expectedAfterSellMoney {
		t.Errorf("restored Character.Money after sell = %d, want %d", restoredCharAfterSell.Money, expectedAfterSellMoney)
	}

	restoredInvAfterSell, err := invRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredInvAfterSell.Items) != 0 {
		t.Errorf("restored Inventory after sell should be empty, got %d items", len(restoredInvAfterSell.Items))
	}
}

func TestConcurrentShopPurchasesPreventOverdraft(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)

	char, _ := database.CreateTestCharacter(ctx, db, "Concurrent Buyer")
	char.Money = 150
	_ = charRepo.Update(ctx, char)

	sword, _ := item.NewEquipmentDefinition("con_sword", "Con Sword", 100, item.SlotMainHand)
	catalog, _ := item.NewCatalog([]item.Definition{sword})

	shopService, _ := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider))

	// Attempt two concurrent purchases of a 100G item with only 150G total
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := shopService.Purchase(ctx, char.ID, "con_sword", 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var successes, failures int
	for err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}

	// Exactly 1 purchase must succeed and 1 must fail due to overdraft
	if successes != 1 {
		t.Errorf("successes = %d, want 1", successes)
	}
	if failures != 1 {
		t.Errorf("failures = %d, want 1", failures)
	}

	restoredChar, _ := charRepo.FindByID(ctx, char.ID)
	if restoredChar.Money != 50 {
		t.Errorf("restored Character.Money = %d, want 50", restoredChar.Money)
	}

	restoredInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if len(restoredInv.Items) != 1 {
		t.Errorf("restored Inventory item count = %d, want 1", len(restoredInv.Items))
	}
}

func TestConcurrentShopSalesPreventDoubleSpending(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)

	char, _ := database.CreateTestCharacter(ctx, db, "Concurrent Seller")
	char.Money = 0
	_ = charRepo.Update(ctx, char)

	potion, _ := item.NewDefinition("con_potion", "Con Potion", 100) // sell price = 50
	catalog, _ := item.NewCatalog([]item.Definition{potion})
	shopService, _ := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider))

	// Setup inventory with 1 potion
	inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	potionInst, _ := item.NewInstance("con_potion", 1)
	_ = inv.Add(potionInst)
	_ = invRepo.Save(ctx, inv)

	// Attempt two concurrent sales of the same item instance of quantity 1
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := shopService.Sell(ctx, char.ID, potionInst.ID, 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var successes, failures int
	for err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}

	// Exactly 1 sale must succeed and 1 must fail due to unowned / insufficient quantity
	if successes != 1 {
		t.Errorf("successes = %d, want 1", successes)
	}
	if failures != 1 {
		t.Errorf("failures = %d, want 1", failures)
	}

	restoredChar, _ := charRepo.FindByID(ctx, char.ID)
	if restoredChar.Money != 50 {
		t.Errorf("restored Character.Money = %d, want 50", restoredChar.Money)
	}

	restoredInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if len(restoredInv.Items) != 0 {
		t.Errorf("restored Inventory item count = %d, want 0", len(restoredInv.Items))
	}
}

func TestShopIntegration_BulkPurchaseBounds(t *testing.T) {
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
	txProvider := database.NewTransactionProvider(db)

	char, _ := database.CreateTestCharacter(ctx, db, "Bulk Buyer")
	char.Money = 1_000_000
	_ = charRepo.Update(ctx, char)

	potion, _ := item.NewDefinition("bulk_potion", "Bulk Potion", 10)
	catalog, _ := item.NewCatalog([]item.Definition{potion})
	shopService, _ := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider))

	// 1. Purchase MaxTransactionQuantity (9999)
	res, err := shopService.Purchase(ctx, char.ID, "bulk_potion", shop.MaxTransactionQuantity)
	if err != nil {
		t.Fatalf("Purchase(9999) failed: %v", err)
	}
	if res.TotalPrice != 99990 {
		t.Errorf("TotalPrice = %d, want 99990", res.TotalPrice)
	}

	// 2. Purchase MaxTransactionQuantity + 1 -> rejected
	_, err = shopService.Purchase(ctx, char.ID, "bulk_potion", shop.MaxTransactionQuantity+1)
	if err == nil {
		t.Fatalf("expected error for quantity > MaxTransactionQuantity, got nil")
	}
}
