package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
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

func TestDungeonRepository_ItemDeliveryAndDepotFallback(t *testing.T) {
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
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Dungeon Overflow Hero")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := dungeonRepo.GetRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()

	// 1. Finalize with first item -> goes into inventory
	hist1 := dungeon.DungeonExpeditionHistory{
		ID:               fmt.Sprintf("hist_%016x", now.UnixNano()),
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    1,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        100,
		GoldReward:       200,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	item1 := []coreitem.Instance{
		{
			ID:           fmt.Sprintf("item_%016x", now.UnixNano()),
			DefinitionID: "potion",
			Quantity:     1,
		},
	}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist1, rec, &char, item1); err != nil {
		t.Fatalf("FinalizeExpedition item1 failed: %v", err)
	}

	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "potion" {
		t.Fatalf("expected 1 item in inventory, got: %#v", inv.Items)
	}

	// 2. Finalize with second item -> inventory full, overflows to depot
	hist2 := dungeon.DungeonExpeditionHistory{
		ID:               fmt.Sprintf("hist_%016x", now.UnixNano()+1),
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    2,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        100,
		GoldReward:       200,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	item2 := []coreitem.Instance{
		{
			ID:           fmt.Sprintf("item_%016x", now.UnixNano()+1),
			DefinitionID: "ether",
			Quantity:     1,
		},
	}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist2, rec, &char, item2); err != nil {
		t.Fatalf("FinalizeExpedition item2 overflow failed: %v", err)
	}

	invAfter2, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID after 2 failed: %v", err)
	}
	if len(invAfter2.Items) != 1 {
		t.Fatalf("inventory must remain at capacity 1, got %d items", len(invAfter2.Items))
	}

	var dep depot.Depot
	dep, err = depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot failed: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "ether" {
		t.Fatalf("expected 1 overflow item in depot, got: %#v", dep.Items)
	}

	// 3. Fill depot to capacity, then deliver third item -> treated as lost drop
	for i := len(dep.Items); i < dep.Capacity; i++ {
		fillItem, _ := coreitem.NewInstance(fmt.Sprintf("filler_%d", i), 1)
		_ = dep.AddItem(fillItem, false)
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("failed to fill depot: %v", err)
	}

	hist3 := dungeon.DungeonExpeditionHistory{
		ID:               fmt.Sprintf("hist_%016x", now.UnixNano()+2),
		CharacterID:      char.ID,
		DungeonID:        "dungeon-01",
		FloorsReached:    3,
		Outcome:          dungeon.StatusCleared,
		ExpReward:        100,
		GoldReward:       200,
		ItemsRewardCount: 1,
		CreatedAt:        now,
	}
	item3 := []coreitem.Instance{
		{
			ID:           fmt.Sprintf("item_%016x", now.UnixNano()+2),
			DefinitionID: "elixir",
			Quantity:     1,
		},
	}
	if err := dungeonRepo.FinalizeExpedition(ctx, hist3, rec, &char, item3); err != nil {
		t.Fatalf("FinalizeExpedition with full depot should succeed as lost drop: %v", err)
	}

	depAfter3, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot after 3 failed: %v", err)
	}
	if len(depAfter3.Items) != dep.Capacity {
		t.Fatalf("depot item count changed, expected %d, got %d", dep.Capacity, len(depAfter3.Items))
	}
}
