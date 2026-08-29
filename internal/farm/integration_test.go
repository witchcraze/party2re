package farm_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/farm"
	"github.com/witchcraze/party2re/internal/id"
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

	// 1. Create character with 0 money and seed/fertilizer in inventory
	char, err := database.CreateTestCharacter(ctx, db, "FullFarmer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = 0 WHERE id = ?", char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 1, 0), (?, ?, ?, 1, 0)
	`, id.New(), char.ID, string(farm.SeedGolden), id.New(), char.ID, farm.FertilizerItemID); err != nil {
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

	// Verify golden seed consumed
	var seedCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = ?", char.ID, string(farm.SeedGolden)).Scan(&seedCount); err != nil {
		t.Fatal(err)
	}
	if seedCount != 0 {
		t.Errorf("expected seed to be consumed, count=%d", seedCount)
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

	// Verify fertilizer consumed
	var fertCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = ?", char.ID, farm.FertilizerItemID).Scan(&fertCount); err != nil {
		t.Fatal(err)
	}
	if fertCount != 0 {
		t.Errorf("expected fertilizer to be consumed, count=%d", fertCount)
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

	// 8. Verify harvested item added to inventory
	var harvestItemCount int
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE character_id = ? AND definition_id = ?", char.ID, "item_golden_fruit").Scan(&harvestItemCount); err != nil {
		t.Fatalf("harvested item not found in inventory: %v", err)
	}
	if harvestItemCount != 2 {
		t.Errorf("harvested item count = %d, want 2", harvestItemCount)
	}
}

func TestFarmService_AtomicRollback_PlantOccupied_ItemPreserved(t *testing.T) {
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
	char, err := database.CreateTestCharacter(ctx, db, "RollbackTester")
	if err != nil {
		t.Fatal(err)
	}

	// Add 2 Herb Seeds to inventory
	seedID := id.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 2, 0)
	`, seedID, char.ID, string(farm.SeedHerb)); err != nil {
		t.Fatal(err)
	}

	// 1. Plant Herb Seed in plot 2
	_, err = svc.PlantSeed(ctx, char.ID, 2, farm.SeedHerb)
	if err != nil {
		t.Fatalf("first PlantSeed failed: %v", err)
	}

	// Verify 1 Herb Seed remains
	var remaining int
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE id = ?", seedID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 remaining seed, got %d", remaining)
	}

	// 2. Attempt to plant on same occupied plot 2
	_, err = svc.PlantSeed(ctx, char.ID, 2, farm.SeedHerb)
	if !errors.Is(err, farm.ErrPlotNotEmpty) {
		t.Fatalf("expected ErrPlotNotEmpty, got %v", err)
	}

	// 3. Verify seed was NOT consumed and remained 1
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE id = ?", seedID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Errorf("seed was consumed on error! remaining = %d, want 1", remaining)
	}
}

func TestFarmService_AtomicRollback_FertilizeEmpty_FertilizerPreserved(t *testing.T) {
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
	char, err := database.CreateTestCharacter(ctx, db, "FertilizerRollbackTester")
	if err != nil {
		t.Fatal(err)
	}

	// Add 1 fertilizer
	fertID := id.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 1, 0)
	`, fertID, char.ID, farm.FertilizerItemID); err != nil {
		t.Fatal(err)
	}

	// Attempt to fertilize empty plot 3 -> fails with ErrPlotNotFound
	_, err = svc.FertilizePlot(ctx, char.ID, 3)
	if !errors.Is(err, farm.ErrPlotNotFound) {
		t.Fatalf("expected ErrPlotNotFound, got %v", err)
	}

	// Verify fertilizer is still in inventory
	var fertQty int
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE id = ?", fertID).Scan(&fertQty); err != nil {
		t.Fatal(err)
	}
	if fertQty != 1 {
		t.Errorf("fertilizer consumed on failed fertilize: qty=%d, want 1", fertQty)
	}
}

func TestFarmService_ConcurrentPlantExploitPrevention(t *testing.T) {
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
	char, err := database.CreateTestCharacter(ctx, db, "ConcurrentFarmer")
	if err != nil {
		t.Fatal(err)
	}

	// Add 10 seeds to character inventory
	seedID := id.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 10, 0)
	`, seedID, char.ID, string(farm.SeedMoonlight)); err != nil {
		t.Fatal(err)
	}

	// Concurrently attempt to plant in plot 0 from 10 goroutines
	var (
		wg           sync.WaitGroup
		successCount int32
		errorCount   int32
	)

	concurrency := 10
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.PlantSeed(ctx, char.ID, 0, farm.SeedMoonlight)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&errorCount, 1)
			}
		}()
	}

	wg.Wait()

	if successCount != 1 {
		t.Fatalf("concurrent PlantSeed: expected exactly 1 success, got %d (errors=%d)", successCount, errorCount)
	}
	if errorCount != int32(concurrency-1) {
		t.Fatalf("concurrent PlantSeed: expected %d errors, got %d", concurrency-1, errorCount)
	}

	// Verify exactly 1 seed was consumed (10 - 1 = 9 remain)
	var remaining int
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE id = ?", seedID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 9 {
		t.Errorf("remaining seeds = %d, want 9", remaining)
	}
}
