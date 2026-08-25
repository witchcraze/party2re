package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestPvPRepository(t *testing.T) {
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
	pvpRepo, err := database.NewPvPRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	charA, err := database.CreateTestCharacter(ctx, db, "Arena Fighter A")
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacter(ctx, db, "Arena Fighter B")
	if err != nil {
		t.Fatal(err)
	}

	// 1. GetOrCreateRating
	ratingA, err := pvpRepo.GetOrCreateRating(ctx, charA.ID)
	if err != nil {
		t.Fatalf("GetOrCreateRating charA error = %v", err)
	}
	if ratingA.Rating != 1000 || ratingA.Wins != 0 {
		t.Errorf("initial ratingA = %#v, want rating 1000", ratingA)
	}

	ratingB, err := pvpRepo.GetOrCreateRating(ctx, charB.ID)
	if err != nil {
		t.Fatalf("GetOrCreateRating charB error = %v", err)
	}
	if ratingB.Rating != 1000 {
		t.Errorf("initial ratingB = %#v, want rating 1000", ratingB)
	}

	// 2. RecordMatchAndUpdateRatings
	now := time.Now().UTC()
	matchID := fmt.Sprintf("match_%d", time.Now().UnixNano())
	match := pvp.MatchRecord{
		ID:                   matchID,
		AttackerID:           charA.ID,
		DefenderID:           charB.ID,
		WinnerID:             charA.ID,
		LoserID:              charB.ID,
		Outcome:              pvp.OutcomeWin,
		Turns:                2,
		AttackerRatingBefore: 1000,
		AttackerRatingAfter:  1016,
		DefenderRatingBefore: 1000,
		DefenderRatingAfter:  984,
		RewardGold:           200,
		RewardExp:            100,
		CreatedAt:            now,
	}

	ratingA.Rating = 1016
	ratingA.Wins = 1
	ratingB.Rating = 984
	ratingB.Losses = 1

	charA.Money += 200
	charA.Experience += 100

	err = pvpRepo.RecordMatchAndUpdateRatings(ctx, match, ratingA, ratingB, charA)
	if err != nil {
		t.Fatalf("RecordMatchAndUpdateRatings error = %v", err)
	}

	// Verify ratings updated
	updatedA, err := pvpRepo.GetOrCreateRating(ctx, charA.ID)
	if err != nil || updatedA.Rating != 1016 || updatedA.Wins != 1 {
		t.Errorf("updatedA = %#v, want rating 1016, wins 1", updatedA)
	}

	updatedB, err := pvpRepo.GetOrCreateRating(ctx, charB.ID)
	if err != nil || updatedB.Rating != 984 || updatedB.Losses != 1 {
		t.Errorf("updatedB = %#v, want rating 984, losses 1", updatedB)
	}

	// Verify character update
	updatedCharA, err := charRepo.FindByID(ctx, charA.ID)
	if err != nil || updatedCharA.Money != charA.Money {
		t.Errorf("updatedCharA money = %d, want %d", updatedCharA.Money, charA.Money)
	}

	// 3. FindOpponents
	opponents, err := pvpRepo.FindOpponents(ctx, charA.ID, 10)
	if err != nil {
		t.Fatalf("FindOpponents error = %v", err)
	}
	if len(opponents) == 0 {
		t.Fatalf("FindOpponents returned 0 opponents")
	}
	for _, opp := range opponents {
		if opp.CharacterID == charA.ID {
			t.Errorf("FindOpponents returned self: %s", charA.ID)
		}
		if opp.Name == "" || opp.Level <= 0 || opp.Rating < 0 {
			t.Errorf("FindOpponents returned invalid candidate: %#v", opp)
		}
	}

	// 4. GetMatchHistory
	hist, err := pvpRepo.GetMatchHistory(ctx, charA.ID, 5)
	if err != nil || len(hist) == 0 {
		t.Fatalf("GetMatchHistory error = %v, len = %d", err, len(hist))
	}
	if hist[0].ID != matchID || hist[0].Outcome != pvp.OutcomeWin {
		t.Errorf("match history[0] = %#v", hist[0])
	}

	// 5. GetDefenseLogs
	defenseLogs, err := pvpRepo.GetDefenseLogs(ctx, charB.ID, 5)
	if err != nil || len(defenseLogs) == 0 {
		t.Fatalf("GetDefenseLogs error = %v, len = %d", err, len(defenseLogs))
	}
	if defenseLogs[0].ID != matchID || defenseLogs[0].DefenderID != charB.ID {
		t.Errorf("defenseLogs[0] = %#v", defenseLogs[0])
	}

	// 6. GetLeaderboard
	leaderboard, err := pvpRepo.GetLeaderboard(ctx, 10)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard error = %v, len = %d", err, len(leaderboard))
	}
	if leaderboard[0].Rating < 1000 {
		t.Errorf("leaderboard top rating = %d, expected >= 1000", leaderboard[0].Rating)
	}
}
