package home_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestHomeServiceIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char1, err := database.CreateTestCharacter(ctx, db, "HomeIntegrationHero1")
	if err != nil {
		t.Fatal(err)
	}
	char2, err := database.CreateTestCharacter(ctx, db, "HomeIntegrationHero2")
	if err != nil {
		t.Fatal(err)
	}

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := home.NewService(homeRepo, charRepo, home.WithCharacterUpdater(charRepo))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Visit & View
	view, err := svc.GetHomeView(ctx, char1.ID, char2.ID)
	if err != nil {
		t.Fatalf("GetHomeView failed: %v", err)
	}
	if view.Owner.ID != char1.ID {
		t.Errorf("expected owner ID %s, got %s", char1.ID, view.Owner.ID)
	}

	// 2. Custom companion settings & character color
	updatedHome, err := svc.UpdateHome(ctx, char1.ID, "ドラキー")
	if err != nil {
		t.Fatalf("UpdateHome failed: %v", err)
	}
	if updatedHome.CompanionName != "ドラキー" {
		t.Errorf("expected companion ドラキー, got %s", updatedHome.CompanionName)
	}

	if err := svc.SetCharacterColor(ctx, char1.ID, "#123456"); err != nil {
		t.Fatalf("SetCharacterColor failed: %v", err)
	}

	// 3. Letters
	letter, err := svc.SendLetter(ctx, char2.ID, char1.ID, "Nice house!", "#00ff00")
	if err != nil {
		t.Fatalf("SendLetter failed: %v", err)
	}

	unread, err := svc.GetUnreadLetterCount(ctx, char1.ID)
	if err != nil || unread != 1 {
		t.Errorf("expected 1 unread, got %d, err=%v", unread, err)
	}

	err = svc.ReadLetter(ctx, letter.ID, char1.ID)
	if err != nil {
		t.Fatalf("ReadLetter failed: %v", err)
	}

	// 4. Companion phrases
	phrase, err := svc.TeachCompanionPhrase(ctx, char1.ID, "いらっしゃい！")
	if err != nil {
		t.Fatalf("TeachCompanionPhrase failed: %v", err)
	}

	talk, err := svc.TalkToCompanion(ctx, char1.ID)
	if err != nil || talk != "いらっしゃい！" {
		t.Errorf("expected 'いらっしゃい！', got %s, err=%v", talk, err)
	}

	err = svc.ForgetCompanionPhrase(ctx, phrase.ID, char1.ID)
	if err != nil {
		t.Fatalf("ForgetCompanionPhrase failed: %v", err)
	}

	// 5. Town House Estate (メケメケ村: 500G)
	_, _ = db.ExecContext(ctx, "DELETE FROM character_homes WHERE town_id = 'town1'")
	char1.Money = 1000
	if err := charRepo.Update(ctx, char1); err != nil {
		t.Fatal(err)
	}

	buildRes, err := svc.BuildHouse(ctx, char1.ID, "town1", "001")
	if err != nil {
		t.Fatalf("BuildHouse failed: %v", err)
	}
	if buildRes.TownID != "town1" || buildRes.HouseStyle != "001" {
		t.Errorf("unexpected build result: %+v", buildRes)
	}

	// Check house
	checkRes, err := svc.CheckHouse(ctx, char1.ID)
	if err != nil {
		t.Fatalf("CheckHouse failed: %v", err)
	}
	if checkRes.OwnerName != char1.Name {
		t.Errorf("expected owner %s, got %s", char1.Name, checkRes.OwnerName)
	}
}

type testDepotManagerAdapter struct {
	repo *database.DepotRepository
}

func (a *testDepotManagerAdapter) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	return a.repo.FindByCharacterID(ctx, characterID)
}

func (a *testDepotManagerAdapter) ConsumeOne(ctx context.Context, characterID, itemInstanceID string) error {
	dp, err := a.repo.FindByCharacterIDForUpdate(ctx, characterID)
	if err != nil {
		return err
	}
	if _, err := dp.ConsumeOne(itemInstanceID); err != nil {
		return err
	}
	return a.repo.Save(ctx, dp)
}

func TestHomeItemUsage_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "HomeItemHero")
	if err != nil {
		t.Fatal(err)
	}

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
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
	txProvider := database.NewTransactionProvider(db)
	eco, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	invService, err := inventory.NewService(invRepo)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	depotAdapter := &testDepotManagerAdapter{repo: depotRepo}

	svc, err := home.NewService(
		homeRepo,
		charRepo,
		home.WithEconomy(eco),
		home.WithInventoryManager(invService),
		home.WithDepotManager(depotAdapter),
		home.WithItemCatalog(cat),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Inventory seed consumption (item-018: 力の種 -> Attack increase)
	initialAttack := char.Stats.Attack
	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		inv, _ = coreinventory.New(char.ID)
	}
	if err := inv.Add(coreitem.Instance{ID: "inv-seed-1", DefinitionID: "item-018", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	resInv, err := svc.UseHomeItem(ctx, char.ID, "inv-seed-1", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem inventory failed: %v", err)
	}
	if !resInv.Consumed || resInv.Action != "consumed" {
		t.Fatalf("expected consumed action, got %+v", resInv)
	}

	// Verify persistence directly from repositories
	reloadedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedChar.Stats.Attack <= initialAttack {
		t.Errorf("expected attack to increase from %d, got %d", initialAttack, reloadedChar.Stats.Attack)
	}

	reloadedInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := reloadedInv.Find("inv-seed-1"); found {
		t.Errorf("expected inv-seed-1 to be deleted from inventory")
	}

	// 2. Depot seed consumption (item-019: 守りの種 -> Defense increase)
	initialDefense := reloadedChar.Stats.Defense
	dp, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		dp, _ = depot.NewDepot(char.ID)
	}
	dp.Items = append(dp.Items, coreitem.Instance{ID: "depot-seed-1", DefinitionID: "item-019", Quantity: 1})
	if err := depotRepo.Save(ctx, dp); err != nil {
		t.Fatal(err)
	}

	resDepot, err := svc.UseHomeItem(ctx, char.ID, "depot-seed-1", "depot")
	if err != nil {
		t.Fatalf("UseHomeItem depot failed: %v", err)
	}
	if !resDepot.Consumed || resDepot.Action != "consumed" {
		t.Fatalf("expected consumed action, got %+v", resDepot)
	}

	// Verify persistence in depot
	reloadedCharAfterDepot, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedCharAfterDepot.Stats.Defense <= initialDefense {
		t.Errorf("expected defense to increase from %d, got %d", initialDefense, reloadedCharAfterDepot.Stats.Defense)
	}

	reloadedDepot, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range reloadedDepot.Items {
		if it.ID == "depot-seed-1" {
			t.Errorf("expected depot-seed-1 to be removed from depot")
		}
	}
}

func TestHomeItemUsage_ConcurrentStress(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "HomeStressHero")
	if err != nil {
		t.Fatal(err)
	}

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
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
	txProvider := database.NewTransactionProvider(db)
	eco, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	invService, err := inventory.NewService(invRepo)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	depotAdapter := &testDepotManagerAdapter{repo: depotRepo}

	svc, err := home.NewService(
		homeRepo,
		charRepo,
		home.WithEconomy(eco),
		home.WithInventoryManager(invService),
		home.WithDepotManager(depotAdapter),
		home.WithItemCatalog(cat),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare 15 inventory seed items (item-018: 力の種)
	inv, _ := coreinventory.New(char.ID)
	for i := 0; i < 15; i++ {
		_ = inv.Add(coreitem.Instance{
			ID:           fmt.Sprintf("stress-inv-seed-%d", i),
			DefinitionID: "item-018",
			Quantity:     1,
		})
	}
	if err := invRepo.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}

	// Prepare 15 depot seed items (item-019: 守りの種)
	dp, _ := depot.NewDepot(char.ID)
	for i := 0; i < 15; i++ {
		dp.Items = append(dp.Items, coreitem.Instance{
			ID:           fmt.Sprintf("stress-depot-seed-%d", i),
			DefinitionID: "item-019",
			Quantity:     1,
		})
	}
	if err := depotRepo.Save(ctx, dp); err != nil {
		t.Fatal(err)
	}

	// Concurrency test: 30 workers attempt concurrent item usages (mixed inventory and depot)
	cfg := testutil.ConcurrencyStressConfig{
		Workers:      10,
		OpsPerWorker: 3,
	}

	var opCounter int64
	var inventorySuccesses int64
	var depotSuccesses int64

	stressRes := testutil.RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		curOp := atomic.AddInt64(&opCounter, 1) - 1
		var itemID string
		var source string
		if curOp < 15 {
			itemID = fmt.Sprintf("stress-inv-seed-%d", curOp)
			source = "inventory"
		} else {
			itemID = fmt.Sprintf("stress-depot-seed-%d", curOp-15)
			source = "depot"
		}

		res, err := svc.UseHomeItem(ctx, char.ID, itemID, source)
		if err != nil {
			if errors.Is(err, home.ErrItemNotFound) {
				return nil
			}
			return err
		}
		if res.Consumed {
			if source == "inventory" {
				atomic.AddInt64(&inventorySuccesses, 1)
			} else {
				atomic.AddInt64(&depotSuccesses, 1)
			}
		}
		return nil
	})

	if stressRes.Deadlocks > 0 {
		t.Fatalf("encountered %d deadlocks during concurrent home item usage", stressRes.Deadlocks)
	}

	// Verify database integrity
	finalChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalChar.Stats.Attack <= char.Stats.Attack {
		t.Errorf("expected attack to have increased after %d inventory seed consumptions", inventorySuccesses)
	}
	if finalChar.Stats.Defense <= char.Stats.Defense {
		t.Errorf("expected defense to have increased after %d depot seed consumptions", depotSuccesses)
	}

	finalInv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedInvRemaining := 15 - int(inventorySuccesses)
	if len(finalInv.Items) != expectedInvRemaining {
		t.Errorf("expected %d inventory items remaining, got %d", expectedInvRemaining, len(finalInv.Items))
	}

	finalDepot, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedDepotRemaining := 15 - int(depotSuccesses)
	if len(finalDepot.Items) != expectedDepotRemaining {
		t.Errorf("expected %d depot items remaining, got %d", expectedDepotRemaining, len(finalDepot.Items))
	}
}
