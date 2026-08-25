package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/database"
)

func TestChallengeRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "Challenge Repo Hero")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewChallengeRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	sessionID := fmt.Sprintf("chal_sess_%016x", now.UnixNano())
	s := challenge.ChallengeSession{
		ID:                 sessionID,
		CharacterID:        char.ID,
		TierID:             "novice",
		CurrentRound:       1,
		CharacterCurrentHP: 150,
		AccumulatedExp:     50,
		AccumulatedGold:    30,
		AccumulatedItems:   []string{"potion_minor"},
		Status:             challenge.StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// 1. SaveSession
	if err := repo.SaveSession(ctx, s); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// 2. FindSessionByID
	fetched, err := repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("FindSessionByID failed: %v", err)
	}
	if fetched.ID != sessionID || fetched.CurrentRound != 1 || len(fetched.AccumulatedItems) != 1 {
		t.Errorf("unexpected fetched session: %#v", fetched)
	}

	// 3. FindActiveSessionByCharacter
	active, err := repo.FindActiveSessionByCharacter(ctx, char.ID)
	if err != nil || active == nil {
		t.Fatalf("FindActiveSessionByCharacter failed: %v", err)
	}
	if active.ID != sessionID {
		t.Errorf("expected active session ID %s, got %s", sessionID, active.ID)
	}

	// 4. UpdateSession
	s.CurrentRound = 3
	s.AccumulatedExp = 120
	if err := repo.UpdateSession(ctx, s); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	// 5. FinalizeSession
	s.Status = challenge.StatusClaimed
	if err := repo.FinalizeSession(ctx, s, 120, 80, []string{"potion_minor"}, 2); err != nil {
		t.Fatalf("FinalizeSession failed: %v", err)
	}

	// 6. FindRecord
	rec, err := repo.FindRecord(ctx, char.ID, "novice")
	if err != nil || rec == nil {
		t.Fatalf("FindRecord failed: %v", err)
	}
	if rec.HighestRound != 2 || rec.TotalVictories != 2 || rec.TotalAttempts != 1 {
		t.Errorf("unexpected record: %#v", rec)
	}

	// 7. GetLeaderboard
	leaderboard, err := repo.GetLeaderboard(ctx, "novice", 100)
	if err != nil || len(leaderboard) == 0 {
		t.Fatalf("GetLeaderboard failed: %v", err)
	}
	found := false
	for _, entry := range leaderboard {
		if entry.CharacterID == char.ID {
			found = true
			if entry.HighestRound != 2 {
				t.Errorf("unexpected leaderboard streak: %d", entry.HighestRound)
			}
			break
		}
	}
	if !found {
		t.Errorf("character not found in challenge leaderboard")
	}
}
