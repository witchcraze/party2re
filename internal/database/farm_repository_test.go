package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/farm"
)

func TestFarmRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewFarmRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character with 10,000 gold
	char, err := database.CreateTestCharacter(ctx, db, "FarmerTester")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Save new plot
	now := time.Now().UTC()
	matures := now.Add(5 * time.Minute)
	wither := now.Add(24 * time.Hour)

	plot := farm.FarmPlot{
		CharacterID: char.ID,
		PlotIndex:   0,
		SeedType:    farm.SeedHerb,
		Status:      farm.PlotStatusGrowing,
		PlantedAt:   &now,
		MaturesAt:   &matures,
		WitherAt:    &wither,
		Watered:     true,
		Fertilized:  false,
		Yield:       3,
	}

	if err := repo.SavePlot(ctx, plot); err != nil {
		t.Fatalf("SavePlot failed: %v", err)
	}

	// 3. Get plot
	fetched, err := repo.GetPlot(ctx, char.ID, 0)
	if err != nil {
		t.Fatalf("GetPlot failed: %v", err)
	}
	if fetched.Yield != 3 || !fetched.Watered || fetched.SeedType != farm.SeedHerb {
		t.Errorf("fetched plot: yield=%d, watered=%v, seed=%s", fetched.Yield, fetched.Watered, fetched.SeedType)
	}

	// 4. Harvest plot (adds 150 gold)
	harvested, err := repo.HarvestPlot(ctx, char.ID, 0, 150)
	if err != nil {
		t.Fatalf("HarvestPlot failed: %v", err)
	}
	if harvested.Status != farm.PlotStatusEmpty {
		t.Errorf("harvested status=%v, want EMPTY", harvested.Status)
	}

	// 5. Verify character money increased
	var updatedMoney int
	if err := db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", char.ID).Scan(&updatedMoney); err != nil {
		t.Fatal(err)
	}
	if updatedMoney != 10150 {
		t.Errorf("character money = %d, want 10150", updatedMoney)
	}
}
