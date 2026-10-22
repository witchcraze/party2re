package shop_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/collection"
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
	createdChar, err := charService.Create(ctx, player.ID, "ShopInt")
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	shopService, err := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider), shop.WithDepotRepository(depotRepo))
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
	expectedMoney := 200 - (testItem.Price * 2)
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
	char.Money = 250
	_ = charRepo.Update(ctx, char)

	sword, _ := item.NewEquipmentDefinition("con_sword", "Con Sword", 100, item.SlotMainHand)
	catalog, _ := item.NewCatalog([]item.Definition{sword})

	depotRepo, _ := database.NewDepotRepository(db)
	shopService, _ := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider), shop.WithDepotRepository(depotRepo))

	// Attempt two concurrent purchases of a 200G (2x retail) item with only 250G total
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
	char.Money = 999_999
	_ = charRepo.Update(ctx, char)

	potion, _ := item.NewDefinition("bulk_potion", "Bulk Potion", 10)
	catalog, _ := item.NewCatalog([]item.Definition{potion})
	depotRepo, _ := database.NewDepotRepository(db)
	shopService, _ := shop.NewService(charRepo, invRepo, catalog, shop.WithTransactionProvider(txProvider), shop.WithDepotRepository(depotRepo))

	// 1. Purchase MaxTransactionQuantity (9999)
	res, err := shopService.Purchase(ctx, char.ID, "bulk_potion", shop.MaxTransactionQuantity)
	if err != nil {
		t.Fatalf("Purchase(9999) failed: %v", err)
	}
	expectedTotal := 10 * 2 * shop.MaxTransactionQuantity
	if res.TotalPrice != expectedTotal {
		t.Errorf("TotalPrice = %d, want %d", res.TotalPrice, expectedTotal)
	}

	// 2. Purchase MaxTransactionQuantity + 1 -> rejected
	_, err = shopService.Purchase(ctx, char.ID, "bulk_potion", shop.MaxTransactionQuantity+1)
	if err == nil {
		t.Fatalf("expected error for quantity > MaxTransactionQuantity, got nil")
	}
}

type shopLegendInductor struct {
	err   error
	calls []string
}

func (l *shopLegendInductor) RecordLegend(_ context.Context, category, characterID string) error {
	if l.err != nil {
		return l.err
	}
	l.calls = append(l.calls, category+":"+characterID)
	return nil
}

func TestShopIntegration_MandatoryInductionRollback(t *testing.T) {
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
	colRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	inductor := &shopLegendInductor{err: errors.New("mandatory shop legend failure")}
	colSvc, err := collection.NewService(colRepo, 180, 141,
		collection.WithTotalWeapons(1),
		collection.WithLegendInductor(inductor),
	)
	if err != nil {
		t.Fatal(err)
	}

	shopService, err := shop.NewService(charRepo, invRepo, catalog,
		shop.WithTransactionProvider(txProvider),
		shop.WithDepotRepository(depotRepo),
		shop.WithCollectionRecorder(colSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "ShopRollbackTester", 50000)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial purchase attempt fails because legend induction fails
	_, err = shopService.Purchase(ctx, char.ID, "weapon-01", 1)
	if err == nil {
		t.Fatal("expected Purchase to fail when collection legend induction fails")
	}

	// 2. Assert full database rollback: money, inventory, collection, milestone
	restoredChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredChar.Money != 50000 {
		t.Errorf("character money = %d; want 50000 (unspent)", restoredChar.Money)
	}
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(inv.Items) != 0 {
		t.Errorf("inventory item count = %d; want 0", len(inv.Items))
	}
	colEntries, err := colRepo.GetItemCollection(ctx, char.ID, "weapon")
	if err != nil || len(colEntries) != 0 {
		t.Errorf("collection weapon count = %d; want 0", len(colEntries))
	}
	isCompleted, err := colRepo.IsCompleted(ctx, char.ID, "weapon")
	if err != nil || isCompleted {
		t.Errorf("weapon milestone completed = %v; want false", isCompleted)
	}

	// 3. Resolve failure and retry purchase
	inductor.err = nil
	res, err := shopService.Purchase(ctx, char.ID, "weapon-01", 1)
	if err != nil {
		t.Fatalf("expected Purchase to succeed after resolving inductor failure: %v", err)
	}
	if res.Character.Money >= 50000 {
		t.Errorf("expected money deducted, got %d", res.Character.Money)
	}

	// 4. Assert DB state after successful retry
	restoredChar, err = charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredChar.Money != res.Character.Money {
		t.Errorf("persisted char money = %d; want %d", restoredChar.Money, res.Character.Money)
	}
	inv, err = invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil || len(inv.Items) != 1 {
		t.Errorf("persisted inventory items = %d; want 1", len(inv.Items))
	}
	colEntries, err = colRepo.GetItemCollection(ctx, char.ID, "weapon")
	if err != nil || len(colEntries) != 1 {
		t.Errorf("persisted collection items = %d; want 1", len(colEntries))
	}
	isCompleted, err = colRepo.IsCompleted(ctx, char.ID, "weapon")
	if err != nil || !isCompleted {
		t.Errorf("weapon milestone completed = %v; want true", isCompleted)
	}
	if len(inductor.calls) != 1 {
		t.Errorf("inductor calls = %d; want 1", len(inductor.calls))
	}

	// 5. Subsequent duplicate purchase succeeds without redundant induction
	_, err = shopService.Purchase(ctx, char.ID, "weapon-01", 1)
	if err != nil {
		t.Fatalf("subsequent purchase failed: %v", err)
	}
	if len(inductor.calls) != 1 {
		t.Errorf("inductor calls after repeat purchase = %d; want 1", len(inductor.calls))
	}
}
