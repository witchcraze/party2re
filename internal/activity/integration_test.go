package activity_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/database"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestTrainingPersistsResultAndCharacterAcrossServiceRestart(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	characterService, err := character.NewService(characters)
	if err != nil {
		t.Fatal(err)
	}
	value, err := characterService.Create(ctx, "Training Integration")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	clock := fixedClock{now: start}
	activities, err := database.NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := activity.NewServiceWithClock(activities, characters, &clock)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := service.StartTraining(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}

	clock.now = scheduled.AvailableAt
	if _, err := service.Claim(ctx, scheduled.ID); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	restartedActivities, err := database.NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	restartedCharacters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := activity.NewServiceWithClock(restartedActivities, restartedCharacters, &clock)
	if err != nil {
		t.Fatal(err)
	}
	restoredActivity, err := restartedActivities.FindByID(ctx, scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredActivity.Claimed {
		t.Fatal("activity was not persisted as claimed")
	}
	restoredCharacter, err := restartedCharacters.FindByID(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredCharacter.Experience != activity.TrainingReward {
		t.Fatalf("restored character experience = %d, want %d", restoredCharacter.Experience, activity.TrainingReward)
	}
	if _, err := restarted.Claim(ctx, scheduled.ID); err == nil {
		t.Fatal("restarted service claimed activity twice")
	}
}
