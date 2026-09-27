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

	// 2. Execute Consecutive Rounds until defeat or completion
	roundRes, err := service.ExecuteRound(ctx, session.ID)
	if err != nil {
		t.Fatalf("ExecuteRound failed: %v", err)
	}
	if roundRes.Round != 1 {
		t.Errorf("expected round 1, got %d", roundRes.Round)
	}

	// 3. Verify Leaderboard Query
	leaderboard, err := service.GetLeaderboard(ctx, "0", 100)
	if err != nil {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	_ = leaderboard
}
