package database_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
)

func TestBlackMarketRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "YamijiTester")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewBlackMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// Points Lifecycle
	pts, err := repo.GetCharacterPoints(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterPoints failed: %v", err)
	}
	if pts.RarePoints != 0 || pts.URarePoints != 0 {
		t.Errorf("expected 0 initial points, got Rare=%d URare=%d", pts.RarePoints, pts.URarePoints)
	}

	pts.RarePoints = 12
	pts.URarePoints = 3
	if err := repo.SaveCharacterPoints(ctx, pts); err != nil {
		t.Fatalf("SaveCharacterPoints failed: %v", err)
	}

	ptsForUpdate, err := repo.GetCharacterPointsForUpdate(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterPointsForUpdate failed: %v", err)
	}
	if ptsForUpdate.RarePoints != 12 || ptsForUpdate.URarePoints != 3 {
		t.Errorf("expected Rare=12 URare=3, got Rare=%d URare=%d", ptsForUpdate.RarePoints, ptsForUpdate.URarePoints)
	}
}
