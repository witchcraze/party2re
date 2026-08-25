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
	rec.DailyAttemptsUsed = 1

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
	if updatedRec.HighestTierCleared != 1 || updatedRec.TotalBossDefeats != 1 || updatedRec.DailyAttemptsUsed != 1 {
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
	leaderboard, err := bossRepo.GetLeaderboard(ctx, 10)
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
