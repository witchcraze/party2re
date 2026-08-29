package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/farm"
	"github.com/witchcraze/party2re/internal/id"
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

	// Add 1 seed_herb and 1 item_fertilizer to inventory
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 1, 0), (?, ?, ?, 1, 0)
	`, id.New(), char.ID, string(farm.SeedHerb), id.New(), char.ID, farm.FertilizerItemID); err != nil {
		t.Fatal(err)
	}

	// 2. Plant seed
	now := time.Now().UTC()
	seedDef := farm.SeedCatalog[farm.SeedHerb]
	plot, err := repo.PlantSeed(ctx, char.ID, 0, farm.SeedHerb, seedDef, now)
	if err != nil {
		t.Fatalf("PlantSeed failed: %v", err)
	}
	if plot.Status != farm.PlotStatusGrowing || plot.Yield != seedDef.BaseYield {
		t.Errorf("planted plot: status=%v, yield=%d", plot.Status, plot.Yield)
	}

	// Verify seed item was consumed from inventory
	var seedCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = ?", char.ID, string(farm.SeedHerb)).Scan(&seedCount); err != nil {
		t.Fatal(err)
	}
	if seedCount != 0 {
		t.Errorf("seed item was not consumed: count=%d", seedCount)
	}

	// 3. Water plot
	watered, err := repo.WaterPlot(ctx, char.ID, 0, now)
	if err != nil {
		t.Fatalf("WaterPlot failed: %v", err)
	}
	if !watered.Watered || watered.Yield != seedDef.BaseYield+1 {
		t.Errorf("watered plot: watered=%v, yield=%d", watered.Watered, watered.Yield)
	}

	// 4. Fertilize plot
	fertilized, err := repo.FertilizePlot(ctx, char.ID, 0, now)
	if err != nil {
		t.Fatalf("FertilizePlot failed: %v", err)
	}
	if !fertilized.Fertilized {
		t.Errorf("fertilized plot: fertilized=%v", fertilized.Fertilized)
	}

	// Verify fertilizer was consumed
	var fertCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = ?", char.ID, farm.FertilizerItemID).Scan(&fertCount); err != nil {
		t.Fatal(err)
	}
	if fertCount != 0 {
		t.Errorf("fertilizer item was not consumed: count=%d", fertCount)
	}

	// 5. Harvest plot (simulate mature time in past)
	harvestTime := now.Add(2 * time.Hour)
	harvested, err := repo.HarvestPlot(ctx, char.ID, 0, 150, seedDef.RewardItemID, fertilized.Yield, harvestTime)
	if err != nil {
		t.Fatalf("HarvestPlot failed: %v", err)
	}
	if harvested.Status != farm.PlotStatusEmpty {
		t.Errorf("harvested status=%v, want EMPTY", harvested.Status)
	}

	// 6. Verify character money increased (10000 + 150 = 10150)
	var updatedMoney int
	if err := db.QueryRowContext(ctx, "SELECT money FROM characters WHERE id = ?", char.ID).Scan(&updatedMoney); err != nil {
		t.Fatal(err)
	}
	if updatedMoney != 10150 {
		t.Errorf("character money = %d, want 10150", updatedMoney)
	}

	// 7. Verify harvested item added to inventory
	var herbQuantity int
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE character_id = ? AND definition_id = ?", char.ID, seedDef.RewardItemID).Scan(&herbQuantity); err != nil {
		t.Fatalf("harvested item not found in inventory: %v", err)
	}
	if herbQuantity != fertilized.Yield {
		t.Errorf("harvested item quantity = %d, want %d", herbQuantity, fertilized.Yield)
	}
}

func TestFarmRepository_AtomicRollback_OnPreconditionFailure(t *testing.T) {
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

	// 1. Character with 1 seed
	char, err := database.CreateTestCharacter(ctx, db, "FarmerRollback")
	if err != nil {
		t.Fatal(err)
	}
	seedID := id.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 1, 0)
	`, seedID, char.ID, string(farm.SeedMandrake)); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	mandrakeDef := farm.SeedCatalog[farm.SeedMandrake]

	// 2. Plant seed in plot 0 (succeeds)
	_, err = repo.PlantSeed(ctx, char.ID, 0, farm.SeedMandrake, mandrakeDef, now)
	if err != nil {
		t.Fatalf("first PlantSeed failed: %v", err)
	}

	// 3. Give 1 more seed to inventory
	seedID2 := id.New()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
		VALUES (?, ?, ?, 1, 0)
	`, seedID2, char.ID, string(farm.SeedMandrake)); err != nil {
		t.Fatal(err)
	}

	// 4. Try planting in occupied plot 0 -> should fail with ErrPlotNotEmpty
	_, err = repo.PlantSeed(ctx, char.ID, 0, farm.SeedMandrake, mandrakeDef, now)
	if !errors.Is(err, farm.ErrPlotNotEmpty) {
		t.Fatalf("expected ErrPlotNotEmpty, got %v", err)
	}

	// 5. Verify the 2nd seed was NOT consumed (still in inventory)
	var remainingSeeds int
	if err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory_items WHERE id = ?", seedID2).Scan(&remainingSeeds); err != nil {
		t.Fatalf("second seed missing from inventory after rollback: %v", err)
	}
	if remainingSeeds != 1 {
		t.Errorf("second seed quantity = %d, want 1", remainingSeeds)
	}

	// 6. Try fertilizing without fertilizer item -> should fail with ErrInsufficientFertilizer
	_, err = repo.FertilizePlot(ctx, char.ID, 0, now)
	if !errors.Is(err, farm.ErrInsufficientFertilizer) {
		t.Fatalf("expected ErrInsufficientFertilizer, got %v", err)
	}

	// 7. Verify plot was not modified
	p, err := repo.GetPlot(ctx, char.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Fertilized {
		t.Error("plot should not be fertilized after failed FertilizePlot")
	}
}
