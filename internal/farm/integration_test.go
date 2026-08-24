package farm_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/farm"
)

func TestFarmServiceDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	farmRepo, err := database.NewFarmRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := farm.NewService(farmRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create character
	char, err := database.CreateTestCharacter(ctx, db, "FullFarmer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 0 WHERE id = ?", char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Plant Golden Seed in plot 1
	plot, err := svc.PlantSeed(ctx, char.ID, 1, farm.SeedGolden)
	if err != nil {
		t.Fatalf("PlantSeed failed: %v", err)
	}
	if plot.Status != farm.PlotStatusGrowing || plot.Yield != 1 {
		t.Errorf("plot: status=%v, yield=%d", plot.Status, plot.Yield)
	}

	// 3. Water plot
	plot, err = svc.WaterPlot(ctx, char.ID, 1)
	if err != nil {
		t.Fatalf("WaterPlot failed: %v", err)
	}
	if plot.Yield != 2 || !plot.Watered {
		t.Errorf("after water: yield=%d, watered=%v", plot.Yield, plot.Watered)
	}

	// 4. Fertilize plot
	plot, err = svc.FertilizePlot(ctx, char.ID, 1)
	if err != nil {
		t.Fatalf("FertilizePlot failed: %v", err)
	}
	if !plot.Fertilized {
		t.Error("expected fertilized to be true")
	}

	// 5. Fast forward DB maturity to test harvest
	past := time.Now().UTC().Add(-2 * time.Hour)
	if _, err := db.ExecContext(ctx, "UPDATE farm_plots SET matures_at = ? WHERE character_id = ? AND plot_index = 1", past, char.ID); err != nil {
		t.Fatal(err)
	}

	// 6. Harvest plot
	res, harvestedPlot, err := svc.HarvestPlot(ctx, char.ID, 1)
	if err != nil {
		t.Fatalf("HarvestPlot failed: %v", err)
	}
	if res.Yield != 2 || res.RewardGold != 4000 || harvestedPlot.Status != farm.PlotStatusEmpty {
		t.Errorf("harvest: yield=%d, gold=%d, status=%v", res.Yield, res.RewardGold, harvestedPlot.Status)
	}

	// 7. Verify DB money updated
	var updatedMoney int
	if err := db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", char.ID).Scan(&updatedMoney); err != nil {
		t.Fatal(err)
	}
	if updatedMoney != 4000 {
		t.Errorf("char money = %d, want 4000", updatedMoney)
	}
}
