package alchemy_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestAlchemyIntegrationOvernightDepotSynthesis(t *testing.T) {
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
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	charService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	createdChar, err := charService.Create(ctx, player.ID, "Alchemy Integrator")
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

	simTime := time.Date(2026, 9, 12, 15, 0, 0, 0, timer.JST)
	alcService, err := alchemy.NewService(
		charRepo,
		depotRepo,
		alcRepo,
		recipeCatalog,
		itemCatalog,
		alchemy.WithTransactionProvider(txProvider),
		alchemy.WithNowFunc(func() time.Time { return simTime }),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Unlock recipe-001 (2 herbs item-001 -> 1 super herb item-002)
	if err := alcService.UnlockRecipe(ctx, createdChar.ID, "recipe-001"); err != nil {
		t.Fatalf("UnlockRecipe failed: %v", err)
	}

	// 2. Put 5 herbs into Depot
	dep, err := depot.NewDepot(createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	dep.Capacity = 10
	herbInst, err := item.NewInstance("item-001", 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := dep.AddItem(herbInst); err != nil {
		t.Fatal(err)
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}

	// 3. Synthesize recipe-001: consumes 2 herbs from Depot
	synthRes, err := alcService.Synthesize(ctx, createdChar.ID, "recipe-001")
	if err != nil {
		t.Fatalf("Synthesize failed: %v", err)
	}
	if synthRes.State != alchemy.StateOngoing {
		t.Errorf("expected StateOngoing, got %v", synthRes.State)
	}

	// Verify depot materials decreased from 5 to 3
	savedDepot, err := depotRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedDepot.Quantity("item-001") != 3 {
		t.Errorf("depot item-001 quantity = %d, want 3", savedDepot.Quantity("item-001"))
	}

	// 4. Before morning / before home rest, claiming must fail
	if _, err := alcService.Claim(ctx, createdChar.ID); err == nil {
		t.Fatal("expected Claim to fail before maturity/rest, got nil")
	}

	// 5. Complete via Home Sleep Hook
	if err := alcService.CompleteOngoingSynthesis(ctx, createdChar.ID); err != nil {
		t.Fatalf("CompleteOngoingSynthesis failed: %v", err)
	}

	status, err := alcService.GetStatus(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != alchemy.StateCompleted {
		t.Errorf("expected status StateCompleted, got %v", status.State)
	}

	// 6. Claim delivers item-002 directly to Depot
	claimRes, err := alcService.Claim(ctx, createdChar.ID)
	if err != nil {
		t.Fatalf("Claim failed: %v", err)
	}
	if claimRes.CreatedItem.DefinitionID != "item-002" {
		t.Errorf("expected created item item-002, got %s", claimRes.CreatedItem.DefinitionID)
	}

	finalDepot, err := depotRepo.FindByCharacterID(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalDepot.Quantity("item-002") != 1 {
		t.Errorf("depot item-002 quantity = %d, want 1", finalDepot.Quantity("item-002"))
	}

	// 7. Compendium reflects crafted state
	comp, err := alcService.GetCompendium(ctx, createdChar.ID)
	if err != nil {
		t.Fatal(err)
	}
	if comp.CraftedCount != 1 {
		t.Errorf("expected crafted count 1, got %d", comp.CraftedCount)
	}
}

func TestAlchemyIntegrationConcurrentSynthesize(t *testing.T) {
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
	depotRepo, _ := database.NewDepotRepository(db)
	alcRepo, _ := database.NewAlchemyRepository(db)
	txProvider := database.NewTransactionProvider(db)

	charService, _ := character.NewService(charRepo)
	player, _ := database.CreateTestPlayer(ctx, db)
	char, _ := charService.Create(ctx, player.ID, "Concurrent Alchemist")

	itemCatalog, _ := item.InitialCatalog()
	recipeCatalog, _ := alchemy.InitialRecipeCatalog()

	alcService, _ := alchemy.NewService(
		charRepo,
		depotRepo,
		alcRepo,
		recipeCatalog,
		itemCatalog,
		alchemy.WithTransactionProvider(txProvider),
	)

	_ = alcService.UnlockRecipe(ctx, char.ID, "recipe-001")

	dep, _ := depot.NewDepot(char.ID)
	dep.Capacity = 10
	herbInst, _ := item.NewInstance("item-001", 10)
	_ = dep.AddItem(herbInst)
	_ = depotRepo.Save(ctx, dep)

	const goroutines = 5
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := alcService.Synthesize(ctx, char.ID, "recipe-001")
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 concurrent synthesize to succeed, got %d", successCount)
	}
}
