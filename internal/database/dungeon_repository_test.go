package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func TestDungeonRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Dungeon Explorer")
	if err != nil {
		t.Fatal(err)
	}

	// 1. GetRecord (initial empty record)
	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetRecord failed: %v", err)
	}
	if rec.CharacterID != char.ID || rec.HighestDungeonCleared != 0 {
		t.Errorf("unexpected initial record: %#v", rec)
	}

	// 2. SaveActiveExpedition & GetActiveExpedition
	now := time.Now().UTC()
	expID := fmt.Sprintf("exp_%016x", now.UnixNano())
	exp := dungeon.ActiveExpedition{
		ID:               expID,
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		CurrentFloor:     1,
		PosX:             2,
		PosY:             0,
		CurrentHP:        char.Stats.HP,
		TurnsRemaining:   20,
		AccumulatedExp:   150,
		AccumulatedGold:  300,
		AccumulatedItems: []string{"potion", "ether"},
		Status:           dungeon.StatusExploring,
		StartedAt:        now,
		UpdatedAt:        now,
	}

	if err := dungeonRepo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("SaveActiveExpedition failed: %v", err)
	}

	fetchedExp, err := dungeonRepo.GetActiveExpedition(ctx, char.ID)
	if err != nil || fetchedExp == nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if fetchedExp.ID != expID || fetchedExp.AccumulatedGold != 300 || len(fetchedExp.AccumulatedItems) != 2 {
		t.Errorf("unexpected fetched expedition: %#v", fetchedExp)
	}

	// 3. FinalizeExpedition
	rec.HighestDungeonCleared = 1
	rec.TotalExpeditions = 1
	rec.TotalFloorsCleared = 2
	rec.TotalChestsOpened = 1
	rec.TotalMonstersSlain = 3

	histID := fmt.Sprintf("hist_%016x", now.UnixNano())
	history := dungeon.DungeonExpeditionHistory{
		ID:               histID,
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    2,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        450,
		GoldReward:       800,
		ItemsRewardCount: 2,
		CreatedAt:        now,
	}

	char.Money += 800
	rewardItems := []coreitem.Instance{
		{
			ID:               fmt.Sprintf("item_%016x", now.UnixNano()),
			DefinitionID:     "potion",
			Quantity:         1,
			EnhancementLevel: 0,
		},
	}

	err = dungeonRepo.FinalizeExpedition(ctx, history, rec, &char, rewardItems)
	if err != nil {
		t.Fatalf("FinalizeExpedition failed: %v", err)
	}

	// 4. Verify Active Expedition Deleted
	clearedExp, err := dungeonRepo.GetActiveExpedition(ctx, char.ID)
	if err != nil || clearedExp != nil {
		t.Errorf("expected active expedition to be nil after finalize, got %#v", clearedExp)
	}

	// 5. Verify Record Updated
	updatedRec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil || updatedRec.HighestDungeonCleared != 1 || updatedRec.TotalExpeditions != 1 {
		t.Errorf("unexpected updated record: %#v", updatedRec)
	}

	// 6. Verify History
	histories, err := dungeonRepo.GetHistory(ctx, char.ID, 5)
	if err != nil || len(histories) != 1 {
		t.Fatalf("GetHistory failed: %v, len=%d", err, len(histories))
	}
	if histories[0].Outcome != dungeon.StatusCleared || histories[0].GoldReward != 800 {
		t.Errorf("unexpected history entry: %#v", histories[0])
	}
}
