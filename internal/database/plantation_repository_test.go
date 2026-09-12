package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/plantation"
)

func TestPlantationRepositoryNilDB(t *testing.T) {
	if _, err := NewPlantationRepository(nil); err == nil {
		t.Fatal("NewPlantationRepository(nil) expected error, got nil")
	}
}

func TestPlantationRepositoryLifecycle(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := NewPlantationRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "Plantation Repo Test")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initially no plot
	_, err = repo.GetPlot(ctx, char.ID)
	if !errors.Is(err, plantation.ErrPlotNotFound) {
		t.Fatalf("expected ErrPlotNotFound, got %v", err)
	}

	// 2. Save plot without fertilizer
	now := time.Now().UTC().Truncate(time.Second)
	matures := now.Add(14 * time.Hour)
	plot := plantation.Plot{
		CharacterID: char.ID,
		SeedID:      "red",
		SownAt:      now,
		MaturesAt:   matures,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := repo.SavePlot(ctx, plot); err != nil {
		t.Fatalf("SavePlot failed: %v", err)
	}

	retrieved, err := repo.GetPlot(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetPlot failed: %v", err)
	}
	if retrieved.SeedID != "red" || retrieved.FertilizerID != nil {
		t.Fatalf("retrieved plot mismatch: %+v", retrieved)
	}

	// 3. Update plot with fertilizer
	fertID := "chemical"
	plot.FertilizerID = &fertID
	plot.UpdatedAt = now.Add(1 * time.Minute)
	if err := repo.SavePlot(ctx, plot); err != nil {
		t.Fatalf("SavePlot update failed: %v", err)
	}

	retrieved2, err := repo.GetPlotForUpdate(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetPlotForUpdate failed: %v", err)
	}
	if retrieved2.FertilizerID == nil || *retrieved2.FertilizerID != "chemical" {
		t.Fatalf("expected fertilizer chemical, got %+v", retrieved2.FertilizerID)
	}

	// 4. Delete plot
	if err := repo.DeletePlot(ctx, char.ID); err != nil {
		t.Fatalf("DeletePlot failed: %v", err)
	}

	_, err = repo.GetPlot(ctx, char.ID)
	if !errors.Is(err, plantation.ErrPlotNotFound) {
		t.Fatalf("expected ErrPlotNotFound after delete, got %v", err)
	}
}
