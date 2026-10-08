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

	// 3. Reset fullness on existing status: preserves meals, gold spent, and last eaten timestamp
	if err := repo.ResetFullness(ctx, char.ID); err != nil {
		t.Fatalf("ResetFullness failed: %v", err)
	}
	afterResetStatus, err := repo.GetCharacterStatus(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterStatus after reset failed: %v", err)
	}
	if afterResetStatus.IsFull {
		t.Errorf("expected is_full to be false after ResetFullness")
	}
	if afterResetStatus.TotalMealsEaten != 1 || afterResetStatus.TotalGoldSpent != 400 {
		t.Errorf("expected history preserved (1 meal, 400 gold), got: %+v", afterResetStatus)
	}
	if afterResetStatus.LastEatenAt == nil || !afterResetStatus.LastEatenAt.Equal(now) {
		t.Errorf("expected last_eaten_at preserved (%v), got: %v", now, afterResetStatus.LastEatenAt)
	}

	// 4. Reset fullness on missing status: creates record with is_full=false and zero counters
	char2, err := database.CreateTestCharacter(ctx, db, "TavernTester2")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ResetFullness(ctx, char2.ID); err != nil {
		t.Fatalf("ResetFullness on missing status failed: %v", err)
	}
	char2Status, err := repo.GetCharacterStatus(ctx, char2.ID)
	if err != nil {
		t.Fatalf("GetCharacterStatus for char2 failed: %v", err)
	}
	if char2Status.IsFull || char2Status.TotalMealsEaten != 0 || char2Status.TotalGoldSpent != 0 || char2Status.LastEatenAt != nil {
		t.Errorf("unexpected initialized status for char2: %+v", char2Status)
	}

	// 5. Delivery reservation operations
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
