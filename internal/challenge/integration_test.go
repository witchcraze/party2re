package challenge_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
)

func TestChallengeIntegrationFlow(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "Challenge Integrator Hero")
	if err != nil {
		t.Fatal(err)
	}

	// Update character level to 10 and max HP to 300
	_, err = db.ExecContext(ctx, "UPDATE characters SET level = 10, experience = 1000, hp = 300, max_hp = 300, attack = 60, defense = 40 WHERE id = ?", char.ID)
	if err != nil {
		t.Fatal(err)
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	challengeRepo, err := database.NewChallengeRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := challenge.NewService(challengeRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start Challenge Session
	session, err := service.StartSession(ctx, char.ID, "novice")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	if session.ID == "" || session.CurrentRound != 1 || session.Status != challenge.StatusActive {
		t.Errorf("unexpected started session: %#v", session)
	}

	// 2. Execute Consecutive Rounds
	for round := 1; round <= 3; round++ {
		roundRes, err := service.ExecuteRound(ctx, session.ID)
		if err != nil {
			t.Fatalf("ExecuteRound %d failed: %v", round, err)
		}
		if !roundRes.Won {
			t.Fatalf("round %d unexpected defeat", round)
		}
		if roundRes.Round != round {
			t.Errorf("expected round %d, got %d", round, roundRes.Round)
		}
	}

	// 3. Cashout safely at Round 4
	cashout, err := service.Cashout(ctx, session.ID)
	if err != nil {
		t.Fatalf("Cashout failed: %v", err)
	}
	if cashout.RoundsCleared != 3 || cashout.AwardedExp <= 0 || cashout.AwardedGold <= 0 {
		t.Errorf("unexpected cashout result: %#v", cashout)
	}

	// 4. Verify Leaderboard Score
	leaderboard, err := service.GetLeaderboard(ctx, "novice", 100)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	found := false
	for _, entry := range leaderboard {
		if entry.CharacterID == char.ID {
			found = true
			if entry.HighestRound != 3 {
				t.Errorf("expected streak 3, got %d", entry.HighestRound)
			}
			break
		}
	}
	if !found {
		t.Errorf("character not found in challenge leaderboard")
	}
}
