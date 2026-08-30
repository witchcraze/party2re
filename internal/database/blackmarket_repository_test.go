package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
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

	today := time.Now().UTC()

	// 1. Initial daily purchases check
	purchases, err := repo.GetDailyPurchases(ctx, char.ID, today)
	if err != nil {
		t.Fatalf("GetDailyPurchases failed: %v", err)
	}
	if len(purchases) != 0 {
		t.Errorf("expected empty purchases initially, got %d", len(purchases))
	}

	// 2. Record purchase
	if err := repo.RecordPurchase(ctx, char.ID, "bm_poison_needle", today, 2); err != nil {
		t.Fatalf("RecordPurchase failed: %v", err)
	}

	purchases, err = repo.GetDailyPurchases(ctx, char.ID, today)
	if err != nil {
		t.Fatalf("GetDailyPurchases failed: %v", err)
	}
	if purchases["bm_poison_needle"] != 2 {
		t.Errorf("expected 2 purchases for bm_poison_needle, got %d", purchases["bm_poison_needle"])
	}

	// 3. Accumulate purchase on same day
	if err := repo.RecordPurchase(ctx, char.ID, "bm_poison_needle", today, 1); err != nil {
		t.Fatalf("RecordPurchase (accumulate) failed: %v", err)
	}
	purchases, err = repo.GetDailyPurchases(ctx, char.ID, today)
	if err != nil {
		t.Fatalf("GetDailyPurchases failed: %v", err)
	}
	if purchases["bm_poison_needle"] != 3 {
		t.Errorf("expected 3 purchases after accumulation, got %d", purchases["bm_poison_needle"])
	}

	// 4. Market State
	state, err := repo.GetMarketState(ctx)
	if err != nil {
		t.Fatalf("GetMarketState failed: %v", err)
	}
	if state.Condition == "" {
		t.Errorf("expected valid condition name")
	}

	newState := blackmarket.MarketState{
		Condition:       blackmarket.ConditionHotDemand,
		PriceMultiplier: 1.35,
		SellMultiplier:  1.25,
		RiskLevel:       "Medium",
	}
	if err := repo.SaveMarketState(ctx, newState); err != nil {
		t.Fatalf("SaveMarketState failed: %v", err)
	}

	fetchedState, err := repo.GetMarketState(ctx)
	if err != nil {
		t.Fatalf("GetMarketState after save failed: %v", err)
	}
	if fetchedState.Condition != blackmarket.ConditionHotDemand {
		t.Errorf("expected HotDemand, got %s", fetchedState.Condition)
	}
	if fetchedState.PriceMultiplier != 1.35 {
		t.Errorf("expected 1.35, got %f", fetchedState.PriceMultiplier)
	}
}
