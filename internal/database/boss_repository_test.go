package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
)

func TestBossRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "Boss Challenger")
	if err != nil {
		t.Fatal(err)
	}

	// 1. GetOrCreateRecord
	rec, err := bossRepo.GetOrCreateRecord(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetOrCreateRecord failed: %v", err)
	}
	if rec.CharacterID != char.ID || rec.HighestTierCleared != 0 || rec.TotalBossDefeats != 0 {
		t.Errorf("unexpected initial record: %#v", rec)
	}

	// 2. RecordChallenge
	now := time.Now().UTC()
	historyID := fmt.Sprintf("hist_%016x", now.UnixNano())
	history := boss.BossChallengeHistory{
		ID:           historyID,
		CharacterID:  char.ID,
		BossID:       "king-01",
		Tier:         1,
		Outcome:      corebattle.OutcomeWin,
		Turns:        5,
		RewardExp:    800,
		RewardGold:   1500,
		RewardItemID: "potion",
		IsFirstClear: true,
		CreatedAt:    now,
	}

	rec.HighestTierCleared = 1
	rec.TotalBossDefeats = 1
	rec.FirstClearedAt = &now
	rec.LastChallengedAt = &now

	char.Money += 1500

	rewardItem := &coreitem.Instance{
		ID:               fmt.Sprintf("item_%016x", now.UnixNano()),
		DefinitionID:     "potion",
		Quantity:         1,
		EnhancementLevel: 0,
	}

	err = bossRepo.RecordChallenge(ctx, history, rec, char, rewardItem)
	if err != nil {
		t.Fatalf("RecordChallenge failed: %v", err)
	}

	// 3. Verify Updated Record
	updatedRec, err := bossRepo.GetOrCreateRecord(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetOrCreateRecord updated failed: %v", err)
	}
	if updatedRec.HighestTierCleared != 1 || updatedRec.TotalBossDefeats != 1 {
		t.Errorf("unexpected updated record: %#v", updatedRec)
	}

	// 4. Verify History
	histories, err := bossRepo.GetHistory(ctx, char.ID, 10)
	if err != nil || len(histories) != 1 {
		t.Fatalf("GetHistory failed: %v, len = %d", err, len(histories))
	}
	if histories[0].BossID != "king-01" || histories[0].RewardGold != 1500 {
		t.Errorf("unexpected history entry: %#v", histories[0])
	}

	// 5. Verify Leaderboard
	leaderboard, err := bossRepo.GetLeaderboard(ctx, 10000)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	found := false
	for _, entry := range leaderboard {
		if entry.CharacterID == char.ID {
			found = true
			if entry.HighestTierCleared != 1 || entry.TotalBossDefeats != 1 {
				t.Errorf("unexpected leaderboard entry: %#v", entry)
			}
			break
		}
	}
	if !found {
		t.Errorf("character not found in boss leaderboard")
	}
}

func TestBossRepository_ItemDeliveryAndDepotFallback(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	bossRepo, err := database.NewBossRepository(db)
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

	char, err := database.CreateTestCharacter(ctx, db, "Boss Overflow Hero")
	if err != nil {
		t.Fatal(err)
	}

	rec, err := bossRepo.GetOrCreateRecord(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Deliver first item when inventory is empty -> should land in inventory
	now := time.Now().UTC()
	hist1 := boss.BossChallengeHistory{
		ID:           fmt.Sprintf("hist_%016x", now.UnixNano()),
		CharacterID:  char.ID,
		BossID:       "king-01",
		Tier:         1,
		Outcome:      corebattle.OutcomeWin,
		RewardItemID: "potion",
		CreatedAt:    now,
	}
	item1 := &coreitem.Instance{DefinitionID: "potion", Quantity: 1}
	if err := bossRepo.RecordChallenge(ctx, hist1, rec, char, item1); err != nil {
		t.Fatalf("RecordChallenge item1 failed: %v", err)
	}

	inv, err := invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].DefinitionID != "potion" {
		t.Fatalf("expected 1 item in inventory, got: %#v", inv.Items)
	}

	// 2. Deliver second item when inventory is full -> should overflow to depot
	hist2 := boss.BossChallengeHistory{
		ID:           fmt.Sprintf("hist_%016x", now.UnixNano()+1),
		CharacterID:  char.ID,
		BossID:       "king-01",
		Tier:         1,
		Outcome:      corebattle.OutcomeWin,
		RewardItemID: "ether",
		CreatedAt:    now,
	}
	item2 := &coreitem.Instance{DefinitionID: "ether", Quantity: 1}
	if err := bossRepo.RecordChallenge(ctx, hist2, rec, char, item2); err != nil {
		t.Fatalf("RecordChallenge item2 overflow failed: %v", err)
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

	// 3. Fill depot to capacity, then deliver third item -> should be treated as lost drop without error
	for i := len(dep.Items); i < dep.Capacity; i++ {
		fillItem, _ := coreitem.NewInstance(fmt.Sprintf("filler_%d", i), 1)
		_ = dep.AddItem(fillItem, false)
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		t.Fatalf("failed to fill depot: %v", err)
	}

	hist3 := boss.BossChallengeHistory{
		ID:           fmt.Sprintf("hist_%016x", now.UnixNano()+2),
		CharacterID:  char.ID,
		BossID:       "king-01",
		Tier:         1,
		Outcome:      corebattle.OutcomeWin,
		RewardItemID: "elixir",
		CreatedAt:    now,
	}
	item3 := &coreitem.Instance{DefinitionID: "elixir", Quantity: 1}
	if err := bossRepo.RecordChallenge(ctx, hist3, rec, char, item3); err != nil {
		t.Fatalf("RecordChallenge with full inventory and depot should succeed as lost drop: %v", err)
	}

	depAfter3, err := depotRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID depot after 3 failed: %v", err)
	}
	if len(depAfter3.Items) != dep.Capacity {
		t.Fatalf("depot item count changed, expected %d, got %d", dep.Capacity, len(depAfter3.Items))
	}
}
