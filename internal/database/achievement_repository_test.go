package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/medal"
)

func TestAchievementRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := database.NewAchievementRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character
	char, err := database.CreateTestCharacter(ctx, db, "AchievementTester")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Test RecordProgress
	testAchs := []medal.Achievement{
		{
			ID:        "int_adv_1",
			Metric:    medal.MetricAdventureVictories,
			Threshold: 5,
		},
	}

	// Progress = 2 (under threshold)
	if err := repo.RecordProgress(ctx, char.ID, medal.MetricAdventureVictories, 2, testAchs); err != nil {
		t.Fatalf("RecordProgress failed: %v", err)
	}

	records, err := repo.GetCharacterAchievements(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterAchievements failed: %v", err)
	}
	if len(records) != 1 || records[0].CurrentProgress != 2 || records[0].IsCompleted {
		t.Fatalf("unexpected record after progress 2: %+v", records)
	}

	// Progress + 4 = 6 (over threshold 5)
	if err := repo.RecordProgress(ctx, char.ID, medal.MetricAdventureVictories, 4, testAchs); err != nil {
		t.Fatalf("RecordProgress 2 failed: %v", err)
	}

	rec, err := repo.GetAchievementForUpdate(ctx, char.ID, "int_adv_1")
	if err != nil {
		t.Fatalf("GetAchievementForUpdate failed: %v", err)
	}
	if rec.CurrentProgress != 6 || !rec.IsCompleted || rec.CompletedAt == nil {
		t.Fatalf("unexpected record after progress 6: %+v", rec)
	}
	if rec.IsClaimed {
		t.Fatalf("expected not claimed initially, got claimed")
	}

	// 3. Test MarkAchievementClaimed
	claimTime := time.Now().UTC()
	if err := repo.MarkAchievementClaimed(ctx, char.ID, "int_adv_1", claimTime); err != nil {
		t.Fatalf("MarkAchievementClaimed failed: %v", err)
	}

	recClaimed, err := repo.GetAchievementForUpdate(ctx, char.ID, "int_adv_1")
	if err != nil {
		t.Fatalf("GetAchievementForUpdate after claim failed: %v", err)
	}
	if !recClaimed.IsClaimed || recClaimed.ClaimedAt == nil {
		t.Fatalf("expected is_claimed true with claimed_at set, got %+v", recClaimed)
	}

	// 4. Test SaveMedal & GetCharacterMedals
	medalRecord := medal.CharacterMedal{
		CharacterID: char.ID,
		MedalID:     "medal_adv_test",
		MedalName:   "テスト冒険勲章",
		Category:    "adventure",
		Description: "テスト統合メダル",
		AwardedAt:   claimTime,
	}
	if err := repo.SaveMedal(ctx, medalRecord); err != nil {
		t.Fatalf("SaveMedal failed: %v", err)
	}

	medals, err := repo.GetCharacterMedals(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetCharacterMedals failed: %v", err)
	}
	if len(medals) != 1 || medals[0].MedalID != "medal_adv_test" {
		t.Fatalf("unexpected medals: %+v", medals)
	}
}
