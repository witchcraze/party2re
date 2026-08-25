package pvp_test

import (
	"context"
	"os"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestPvPIntegrationChallengeFlow(t *testing.T) {
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

	service, err := pvp.NewService(pvpRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	// Create two characters
	charAttacker, err := database.CreateTestCharacter(ctx, db, "PvP Attacker")
	if err != nil {
		t.Fatal(err)
	}
	charDefender, err := database.CreateTestCharacter(ctx, db, "PvP Defender")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial ratings
	attRating, err := service.GetRating(ctx, charAttacker.ID)
	if err != nil || attRating.Rating != 1000 {
		t.Fatalf("unexpected attacker initial rating: %#v, err=%v", attRating, err)
	}

	defRating, err := service.GetRating(ctx, charDefender.ID)
	if err != nil || defRating.Rating != 1000 {
		t.Fatalf("unexpected defender initial rating: %#v, err=%v", defRating, err)
	}

	// 2. Find opponents
	opponents, err := service.FindOpponents(ctx, charAttacker.ID, 10)
	if err != nil {
		t.Fatalf("FindOpponents error = %v", err)
	}
	if len(opponents) == 0 {
		t.Fatalf("expected at least 1 opponent")
	}
	for _, opp := range opponents {
		if opp.CharacterID == charAttacker.ID {
			t.Errorf("FindOpponents returned self: %s", charAttacker.ID)
		}
		if opp.Name == "" || opp.Level <= 0 || opp.Rating < 0 {
			t.Errorf("FindOpponents returned invalid candidate: %#v", opp)
		}
	}

	// 3. Challenge
	res, err := service.Challenge(ctx, charAttacker.ID, charDefender.ID)
	if err != nil {
		t.Fatalf("Challenge error = %v", err)
	}

	if res.Match.Turns < 1 {
		t.Errorf("match turns = %d, want >= 1", res.Match.Turns)
	}

	// Verify ratings updated in DB
	newAttRating, err := service.GetRating(ctx, charAttacker.ID)
	if err != nil {
		t.Fatalf("GetRating attacker error = %v", err)
	}
	newDefRating, err := service.GetRating(ctx, charDefender.ID)
	if err != nil {
		t.Fatalf("GetRating defender error = %v", err)
	}

	if res.Match.Outcome == pvp.OutcomeWin {
		if newAttRating.Rating <= 1000 || newDefRating.Rating >= 1000 {
			t.Errorf("expected ratings to update after win: att=%d, def=%d", newAttRating.Rating, newDefRating.Rating)
		}
		if newAttRating.Wins != 1 || newDefRating.Losses != 1 {
			t.Errorf("expected win/loss count to increment: attWins=%d, defLosses=%d", newAttRating.Wins, newDefRating.Losses)
		}
	}

	// 4. Match history
	hist, err := service.GetMatchHistory(ctx, charAttacker.ID, 10)
	if err != nil || len(hist) == 0 {
		t.Fatalf("GetMatchHistory len = %d, err = %v", len(hist), err)
	}
	if hist[0].ID != res.Match.ID {
		t.Errorf("history match id = %s, want %s", hist[0].ID, res.Match.ID)
	}

	// 5. Defense logs for defender
	defLogs, err := service.GetDefenseLogs(ctx, charDefender.ID, 10)
	if err != nil || len(defLogs) == 0 {
		t.Fatalf("GetDefenseLogs len = %d, err = %v", len(defLogs), err)
	}
	if defLogs[0].ID != res.Match.ID || defLogs[0].DefenderID != charDefender.ID {
		t.Errorf("defense log = %#v", defLogs[0])
	}

	// 6. Leaderboard
	leaderboard, err := service.GetLeaderboard(ctx, 10)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard len = %d, err = %v", len(leaderboard), err)
	}
}
