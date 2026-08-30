package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/tavern"
)

func TestTavernRepository_Database(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not set")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char, err := database.CreateTestCharacter(ctx, db, "TavernTester")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewTavernRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial status check
	status, err := repo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterStatus failed: %v", err)
	}
	if status.IsFull {
		t.Errorf("expected is_full to be false initially")
	}

	// 2. Upsert status
	now := time.Now().UTC().Truncate(time.Second)
	status.IsFull = true
	status.LastEatenAt = &now
	status.TotalMealsEaten = 1
	status.TotalGoldSpent = 400

	if err := repo.UpsertCharacterStatus(ctx, status); err != nil {
		t.Fatalf("UpsertCharacterStatus failed: %v", err)
	}

	fetchedStatus, err := repo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterStatus failed: %v", err)
	}
	if !fetchedStatus.IsFull || fetchedStatus.TotalMealsEaten != 1 || fetchedStatus.TotalGoldSpent != 400 {
		t.Errorf("unexpected fetched status: %+v", fetchedStatus)
	}

	// 3. Delivery reservation operations
	deliv := tavern.DeliveryReservation{
		CharacterID: char.ID,
		ItemID:      "tavern_omelet_rice",
		ItemName:    "ふわとろオムライス",
		Price:       750,
		HPHeal:      500,
		MPHeal:      100,
		Tickets:     7,
		CreatedAt:   now,
	}

	if err := repo.SaveDelivery(ctx, deliv); err != nil {
		t.Fatalf("SaveDelivery failed: %v", err)
	}

	fetchedDeliv, err := repo.GetDelivery(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetDelivery failed: %v", err)
	}
	if fetchedDeliv.ItemID != "tavern_omelet_rice" || fetchedDeliv.Price != 750 {
		t.Errorf("unexpected fetched delivery: %+v", fetchedDeliv)
	}

	// 4. Delete delivery
	if err := repo.DeleteDelivery(ctx, char.ID); err != nil {
		t.Fatalf("DeleteDelivery failed: %v", err)
	}

	_, err = repo.GetDelivery(ctx, char.ID)
	if !errors.Is(err, tavern.ErrNoActiveDelivery) {
		t.Errorf("expected ErrNoActiveDelivery, got %v", err)
	}
}
