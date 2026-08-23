package blacksmith_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
)

type fixedRandSource struct {
	value float64
}

func (f fixedRandSource) Float64() float64 {
	return f.value
}

func TestBlacksmithIntegrationEnhancement(t *testing.T) {
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
	bsRepo, err := database.NewBlacksmithRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	charService, err := character.NewService(charRepo)
	if err != nil {
		t.Fatal(err)
	}
	createdChar, err := charService.Create(ctx, "Blacksmith Integrator")
	if err != nil {
		t.Fatal(err)
	}

	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}

	// Give character an upgradeable weapon and upgrade materials
	inv, _ := invRepo.FindByCharacterID(ctx, createdChar.ID)
	club, _ := item.NewInstance("weapon-01", 1)
	materials, _ := item.NewInstance(blacksmith.DefaultMaterialDefinitionID, 10)
	_ = inv.Add(club)
	_ = inv.Add(materials)
	_ = invRepo.Save(ctx, inv)

	// Guarantee success with fixed random roll 0.0
	bsService, err := blacksmith.NewServiceWithTransaction(charRepo, invRepo, bsRepo, catalog, fixedRandSource{value: 0.0})
	if err != nil {
		t.Fatal(err)
	}

	// Enhance weapon from +0 to +1
	res, err := bsService.Enhance(ctx, createdChar.ID, club.ID)
	if err != nil {
		t.Fatalf("Enhance() error = %v", err)
	}

	if !res.Success || res.PreviousLevel != 0 || res.NewLevel != 1 {
		t.Fatalf("unexpected enhance result: %#v", res)
	}

	// Verify database state
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
	enhancedWeapon, found := restoredInv.Find(club.ID)
	if !found || enhancedWeapon.EnhancementLevel != 1 {
		t.Fatalf("enhanced weapon level = %d, want 1", enhancedWeapon.EnhancementLevel)
	}
	if restoredInv.Quantity(blacksmith.DefaultMaterialDefinitionID) != 9 {
		t.Errorf("remaining materials = %d, want 9", restoredInv.Quantity(blacksmith.DefaultMaterialDefinitionID))
	}
}

func TestConcurrentEnhancementPreventsOverdraft(t *testing.T) {
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
	bsRepo, _ := database.NewBlacksmithRepository(db)

	char, _ := corecharacter.New("Concurrent Enhancer")
	char.Money = 70 // only enough for 1 enhancement (50G)
	_ = charRepo.Save(ctx, char)

	catalog, _ := item.InitialCatalog()

	inv, _ := coreinventory.New(char.ID)
	sword1, _ := item.NewInstance("weapon-01", 1)
	sword2, _ := item.NewInstance("weapon-02", 1)
	materials, _ := item.NewInstance(blacksmith.DefaultMaterialDefinitionID, 5)
	_ = inv.Add(sword1)
	_ = inv.Add(sword2)
	_ = inv.Add(materials)
	_ = invRepo.Save(ctx, inv)

	bsService, _ := blacksmith.NewServiceWithTransaction(charRepo, invRepo, bsRepo, catalog, fixedRandSource{value: 0.0})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, sw := range []item.Instance{sword1, sword2} {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()
			_, err := bsService.Enhance(ctx, char.ID, targetID)
			errs <- err
		}(sw.ID)
	}
	wg.Wait()
	close(errs)

	restoredChar, _ := charRepo.FindByID(ctx, char.ID)
	if restoredChar.Money < 0 {
		t.Fatalf("character money went negative: %d", restoredChar.Money)
	}
}
