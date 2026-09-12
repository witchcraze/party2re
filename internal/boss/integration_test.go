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

	// 1. Create a high-level test character capable of challenging boss.
	char, err := database.CreateTestCharacter(ctx, db, "Legendary Boss Slayer")
	if err != nil {
		t.Fatal(err)
	}

	// Buff stats so the test character can defeat King 1 boss army.
	updateStatsQuery := `
		UPDATE characters
		SET level = 99, hp = 500000, max_hp = 500000, attack = 999999, defense = 99999, agility = 9999
		WHERE id = ?
	`
	if _, err := db.ExecContext(ctx, updateStatsQuery, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. List Bosses — expect 11 stages (king1-10 + king99).
	statuses, err := service.ListBosses(ctx, char.ID)
	if err != nil {
		t.Fatalf("ListBosses failed: %v", err)
	}
	if len(statuses) != 11 {
		t.Errorf("expected 11 bosses, got %d", len(statuses))
	}
	if !statuses[0].IsUnlocked {
		t.Errorf("expected king1 to be unlocked for level 50 character")
	}

	// 3. Challenge king1 (expect Victory for a buffed character).
	res, err := service.ChallengeBoss(ctx, char.ID, "king1")
	if err != nil {
		t.Fatalf("ChallengeBoss failed: %v", err)
	}
	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected victory against king-01, got outcome %v", res.Outcome)
	}
	if res.HeroCountGained != 1 {
		t.Errorf("expected HeroCountGained=1 on first victory")
	}

	// 4. Verify king-02 is now accessible (HighestTierCleared >= 1).
	updatedStatuses, err := service.ListBosses(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updatedStatuses[1].IsUnlocked {
		t.Errorf("expected king2 to be unlocked after clearing king1")
	}

	// 5. Verify History & Leaderboard.
	history, err := service.GetHistory(ctx, char.ID, 5)
	if err != nil || len(history) != 1 {
		t.Fatalf("GetHistory failed: %v, len=%d", err, len(history))
	}

	leaderboard, err := service.GetLeaderboard(ctx, 5)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
}
