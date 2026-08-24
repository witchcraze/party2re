package activity_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/character"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/scheduling"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

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
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	value, err := characterService.Create(ctx, player.ID, "Training Integration")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	activities, err := database.NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := activity.NewServiceWithClock(activities, characters, nil, nil, clock)
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
	restarted, err := activity.NewServiceWithClock(restartedActivities, restartedCharacters, nil, nil, clock)
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

func TestConcurrentTrainingClaimsApplyRewardOnce(t *testing.T) {
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
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	value, err := characterService.Create(ctx, player.ID, "Concurrent Training")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: start}
	activities, err := database.NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	starter, err := activity.NewServiceWithClock(activities, characters, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := starter.StartTraining(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = scheduled.AvailableAt

	firstRepository, err := database.NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	secondRepository, err := database.NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	first, err := activity.NewServiceWithClock(firstRepository, characters, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := activity.NewServiceWithClock(secondRepository, characters, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, service := range []*activity.Service{first, second} {
		wg.Add(1)
		go func(service *activity.Service) {
			defer wg.Done()
			_, claimErr := service.Claim(ctx, scheduled.ID)
			errs <- claimErr
		}(service)
	}
	wg.Wait()
	close(errs)

	var successes, alreadyClaimed int
	for claimErr := range errs {
		switch {
		case claimErr == nil:
			successes++
		case errors.Is(claimErr, activity.ErrAlreadyClaimed):
			alreadyClaimed++
		default:
			t.Fatalf("Claim() error = %v", claimErr)
		}
	}
	if successes != 1 || alreadyClaimed != 1 {
		t.Fatalf("concurrent claims: successes = %d, already claimed = %d", successes, alreadyClaimed)
	}

	restoredCharacter, err := characters.FindByID(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredCharacter.Experience != activity.TrainingReward {
		t.Fatalf("character experience = %d, want %d", restoredCharacter.Experience, activity.TrainingReward)
	}
	restoredActivity, err := activities.FindByID(ctx, scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredActivity.Claimed {
		t.Fatal("activity was not claimed")
	}
}

func TestTrainingScheduledActionIntegration(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	valkeyClient, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer valkeyClient.Close()

	characters, _ := database.NewCharacterRepository(db)
	characterService, _ := character.NewService(characters)
	player, _ := database.CreateTestPlayer(ctx, db)
	value, _ := characterService.Create(ctx, player.ID, "Worker Training")

	start := time.Now().UTC()
	clock := &fixedClock{now: start}
	activities, _ := database.NewActivityRepository(db)

	schedRepo := scheduling.NewValkeyRepository(valkeyClient)
	schedService := scheduling.NewService(schedRepo)

	service, _ := activity.NewServiceWithClock(activities, characters, schedService, nil, clock)

	scheduled, err := service.StartTraining(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}

	clock.now = scheduled.AvailableAt

	due, err := schedRepo.FetchDue(ctx, clock.now, 10)
	if err != nil {
		t.Fatal(err)
	}

	var foundAction *core_scheduling.ScheduledAction
	for _, a := range due {
		if a.Params["activity_id"] == scheduled.ID {
			foundAction = &a
			break
		}
	}
	if foundAction == nil {
		t.Fatal("ScheduledAction was not enqueued or fetched")
	}

	handler := activity.NewTrainingHandler(service)
	if err := handler.Handle(ctx, *foundAction); err != nil {
		t.Fatal(err)
	}

	restoredCharacter, _ := characters.FindByID(ctx, value.ID)
	if restoredCharacter.Experience != activity.TrainingReward {
		t.Fatalf("restored character experience = %d, want %d", restoredCharacter.Experience, activity.TrainingReward)
	}

	valkeyClient.Do(ctx, valkeyClient.B().Del().Key("party2:scheduled:action:"+foundAction.ID).Build())
}
