package boss_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
)

func TestBossIntegrationFlow(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	battleEngine := corebattle.Engine{}
	service, err := boss.NewService(bossRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create a high-level test character capable of challenging boss
	char, err := database.CreateTestCharacter(ctx, db, "Legendary Boss Slayer")
	if err != nil {
		t.Fatal(err)
	}

	// Buff stats so the test character can defeat Tier 1 Boss
	updateStatsQuery := `
		UPDATE characters
		SET level = 50, hp = 1500, max_hp = 1500, attack = 350, defense = 200, agility = 150
		WHERE id = ?
	`
	if _, err := db.ExecContext(ctx, updateStatsQuery, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. List Bosses
	statuses, err := service.ListBosses(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListBosses failed: %v", err)
	}
	if len(statuses) != 11 {
		t.Errorf("expected 11 bosses, got %d", len(statuses))
	}
	if !statuses[0].IsUnlocked {
		t.Errorf("expected Tier 1 to be unlocked for level 50 character")
	}

	// 3. Challenge Tier 1 Boss (Victory)
	res, err := service.ChallengeBoss(ctx, char.ID, "king-01")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}
	if res.BattleResult.WinnerID != char.ID {
		t.Fatalf("expected character to win against Tier 1 boss")
	}
	if !res.IsFirstClear {
		t.Errorf("expected isFirstClear to be true")
	}
	if res.ExperienceReward != 800 || res.GoldReward != 1500 {
		t.Errorf("unexpected rewards: EXP=%d, Gold=%d", res.ExperienceReward, res.GoldReward)
	}
	if res.UpdatedRecord.HighestTierCleared != 1 || res.UpdatedRecord.TotalBossDefeats != 1 {
		t.Errorf("unexpected updated record: %#v", res.UpdatedRecord)
	}

	// 4. Verify Tier 2 is now unlocked
	updatedStatuses, err := service.ListBosses(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updatedStatuses[1].IsUnlocked {
		t.Errorf("expected Tier 2 to be unlocked after clearing Tier 1")
	}

	// 5. Verify History & Leaderboard
	history, err := service.GetHistory(ctx, char.ID, 5)
	if err != nil || len(history) != 1 {
		t.Fatalf("GetHistory failed: %v, len=%d", err, len(history))
	}

	leaderboard, err := service.GetLeaderboard(ctx, 5)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
}
