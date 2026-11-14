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
	"github.com/witchcraze/party2re/internal/id"
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
	value, err := characterService.Create(ctx, player.ID, id.New()[:8])
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
	value, err := characterService.Create(ctx, player.ID, id.New()[:8])
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
	t.Cleanup(func() { _ = db.Close() })

	valkeyClient, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(valkeyClient.Close)

	characters, _ := database.NewCharacterRepository(db)
	characterService, _ := character.NewService(characters)
	player, _ := database.CreateTestPlayer(ctx, db)
	value, _ := characterService.Create(ctx, player.ID, id.New()[:8])

	actorKey := "party2:scheduled:actor:" + value.ID
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		members, err := valkeyClient.Do(cleanupCtx, valkeyClient.B().Smembers().Key(actorKey).Build()).AsStrSlice()
		if err != nil {
			t.Errorf("list scheduled actions for %s: %v", value.ID, err)
		}
		for _, actionID := range members {
			if err := valkeyClient.Do(cleanupCtx, valkeyClient.B().Del().Key("party2:scheduled:action:"+actionID, "party2:scheduled:lock:"+actionID).Build()).Error(); err != nil {
				t.Errorf("delete scheduled action %s: %v", actionID, err)
			}
			if err := valkeyClient.Do(cleanupCtx, valkeyClient.B().Zrem().Key("party2:scheduled:pending").Member(actionID).Build()).Error(); err != nil {
				t.Errorf("remove scheduled action %s from pending queue: %v", actionID, err)
			}
		}
		if err := valkeyClient.Do(cleanupCtx, valkeyClient.B().Del().Key(actorKey).Build()).Error(); err != nil {
			t.Errorf("delete scheduled actor index %s: %v", actorKey, err)
		}
	})

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

	pending, err := schedRepo.FindPendingByActorID(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}

	var foundAction *core_scheduling.ScheduledAction
	for i := range pending {
		if pending[i].Params["activity_id"] == scheduled.ID {
			foundAction = &pending[i]
			break
		}
	}
	if foundAction == nil {
		t.Fatal("ScheduledAction was not enqueued for actor")
	}
	score, err := valkeyClient.Do(ctx, valkeyClient.B().Zscore().Key("party2:scheduled:pending").Member(foundAction.ID).Build()).AsFloat64()
	if err != nil {
		t.Fatalf("ScheduledAction %s missing from pending queue: %v", foundAction.ID, err)
	}
	if score > float64(clock.now.Unix()) {
		t.Fatalf("ScheduledAction %s score = %v, want <= %d", foundAction.ID, score, clock.now.Unix())
	}

	handler := activity.NewTrainingHandler(service)
	if err := handler.Handle(ctx, *foundAction); err != nil {
		t.Fatal(err)
	}

	restoredCharacter, _ := characters.FindByID(ctx, value.ID)
	if restoredCharacter.Experience != activity.TrainingReward {
		t.Fatalf("restored character experience = %d, want %d", restoredCharacter.Experience, activity.TrainingReward)
	}
}
