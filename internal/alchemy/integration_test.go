package alchemy_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
)

func TestAlchemyIntegrationSynthesis(t *testing.T) {
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
	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	charService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	createdChar, err := charService.Create(ctx, "Alchemy Integrator")
	if err != nil {
		t.Fatal(err)
	}

	itemCatalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	recipeCatalog, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		t.Fatal(err)
	}

	// Give character herbs (item-001) for recipe-001 (2 herbs -> 1 super herb)
	inv, _ := invRepo.FindByCharacterID(ctx, createdChar.ID)
	herbs, _ := item.NewInstance("item-001", 5)
	_ = inv.Add(herbs)
	_ = invRepo.Save(ctx, inv)

	alcService, err := alchemy.NewServiceWithTransaction(charRepo, invRepo, alcRepo, recipeCatalog, itemCatalog)
	if err != nil {
		t.Fatal(err)
	}

	// Synthesize recipe-001 (上薬草)
	res, err := alcService.Synthesize(ctx, createdChar.ID, "recipe-001")
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}

	if res.CreatedItem.DefinitionID != "item-002" || res.CreatedItem.Quantity != 1 {
		t.Fatalf("unexpected synthesized item: %#v", res.CreatedItem)
	}

	// Verify database persistence
	restoredChar, err := charRepo.FindByID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	expectedMoney := 200 - res.GoldCost
	if restoredChar.Money != expectedMoney {
		t.Errorf("restored character money = %d, want %d", restoredChar.Money, expectedMoney)
	}

	restoredInv, err := invRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredInv.Quantity("item-001") != 3 {
		t.Errorf("remaining herbs = %d, want 3", restoredInv.Quantity("item-001"))
	}
	if restoredInv.Quantity("item-002") != 1 {
		t.Errorf("super herb quantity = %d, want 1", restoredInv.Quantity("item-002"))
	}
}

func TestConcurrentAlchemySynthesis(t *testing.T) {
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
	alcRepo, _ := database.NewAlchemyRepository(db)

	char, _ := corecharacter.New("Concurrent Alchemist")
	char.Money = 500
	_ = charRepo.Save(ctx, char)

	itemCatalog, _ := item.InitialCatalog()
	recipeCatalog, _ := alchemy.InitialRecipeCatalog()

	// Give only 3 herbs (recipe-001 needs 2 herbs per synthesis, so only 1 can succeed)
	inv, _ := coreinventory.New(char.ID)
	herbs, _ := item.NewInstance("item-001", 3)
	_ = inv.Add(herbs)
	_ = invRepo.Save(ctx, inv)

	alcService, _ := alchemy.NewServiceWithTransaction(charRepo, invRepo, alcRepo, recipeCatalog, itemCatalog)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := alcService.Synthesize(ctx, char.ID, "recipe-001")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	restoredInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
	if restoredInv.Quantity("item-001") < 0 {
		t.Fatalf("herb quantity became negative: %d", restoredInv.Quantity("item-001"))
	}
	if restoredInv.Quantity("item-002") > 1 {
		t.Fatalf("more than 1 super herb created with only 3 herbs: %d", restoredInv.Quantity("item-002"))
	}
}
