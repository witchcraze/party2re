package challenge_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	vk "github.com/witchcraze/party2re/internal/valkey"
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

	var opts []challenge.Option
	if os.Getenv("PARTY2_VALKEY_ADDR") != "" {
		vkClient, err := vk.NewClient()
		if err != nil {
			t.Fatal(err)
		}
		defer vkClient.Close()
		vStore, err := challenge.NewValkeySessionRepository(vkClient)
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, challenge.WithActiveSessionStore(vStore))
	}

	service, err := challenge.NewService(challengeRepo, charRepo, corebattle.Engine{}, opts...)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start Challenge Session on Stage 0
	session, err := service.StartSession(ctx, char.ID, "0")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	if session.ID == "" || session.CurrentRound != 1 || session.Status != challenge.StatusActive {
		t.Errorf("unexpected started session: %#v", session)
	}

	// 2. Execute Consecutive Rounds until defeat so FinalizeSession is triggered
	var lastRes *challenge.RoundResult
	for r := 1; r <= 100; r++ {
		roundRes, err := service.ExecuteRound(ctx, session.ID)
		if err != nil {
			t.Fatalf("ExecuteRound %d failed: %v", r, err)
		}
		lastRes = roundRes
		if roundRes.SessionEnded {
			break
		}
	}

	if lastRes == nil || lastRes.Won || !lastRes.SessionEnded {
		t.Fatalf("expected session to end in defeat, got %#v", lastRes)
	}

	// 3. Verify Leaderboard Score
	clearedRounds := lastRes.Round - 1
	leaderboard, err := service.GetLeaderboard(ctx, "0", 100)
	if err != nil {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}

	found := false
	for _, entry := range leaderboard {
		if entry.CharacterID == char.ID {
			found = true
			if entry.HighestRound != clearedRounds {
				t.Errorf("expected highest round %d, got %d", clearedRounds, entry.HighestRound)
			}
			break
		}
	}
	if !found && clearedRounds > 0 {
		t.Errorf("character %s not found in challenge leaderboard for cleared rounds %d", char.ID, clearedRounds)
	}
}
