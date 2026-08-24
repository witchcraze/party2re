package adventure_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/scheduling"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func TestConcurrentAdventureClaimsApplyRewardOnce(t *testing.T) {
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
	value, err := characterService.Create(ctx, player.ID, "Concurrent Adventure")
	if err != nil {
		t.Fatal(err)
	}

	clock := &fixedClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	adventures, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	starter, err := adventure.NewServiceWithClock(adventures, characters, corebattle.Engine{}, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := starter.Start(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = scheduled.AvailableAt

	firstRepository, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	secondRepository, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	first, err := adventure.NewServiceWithClock(firstRepository, characters, corebattle.Engine{}, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adventure.NewServiceWithClock(secondRepository, characters, corebattle.Engine{}, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, service := range []*adventure.Service{first, second} {
		wg.Add(1)
		go func(service *adventure.Service) {
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
		case errors.Is(claimErr, adventure.ErrAlreadyClaimed):
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
	if restoredCharacter.Experience != scheduled.ExperienceReward {
		t.Fatalf("character experience = %d, want %d", restoredCharacter.Experience, scheduled.ExperienceReward)
	}
	restoredAdventure, err := adventures.FindByID(ctx, scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredAdventure.Claimed || !restoredAdventure.Resolved {
		t.Fatalf("adventure state = %#v", restoredAdventure)
	}
}

func TestAdventureScheduledActionIntegration(t *testing.T) {
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
	value, err := characterService.Create(ctx, player.ID, "Worker Adventure")
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now().UTC()
	clock := &fixedClock{now: start}
	adventures, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	schedRepo := scheduling.NewValkeyRepository(valkeyClient)
	schedService := scheduling.NewService(schedRepo)

	service, err := adventure.NewServiceWithClock(adventures, characters, corebattle.Engine{}, schedService, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	scheduled, err := service.Start(ctx, value.ID)
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
		if a.Params["adventure_id"] == scheduled.ID {
			foundAction = &a
			break
		}
	}
	if foundAction == nil {
		t.Fatal("ScheduledAction was not enqueued or fetched")
	}

	handler := adventure.NewAdventureCompletionHandler(service)
	if err := handler.Handle(ctx, *foundAction); err != nil {
		t.Fatal(err)
	}

	restoredCharacter, err := characters.FindByID(ctx, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredCharacter.Experience != scheduled.ExperienceReward {
		t.Fatalf("restored character experience = %d, want %d", restoredCharacter.Experience, scheduled.ExperienceReward)
	}

	restoredAdventure, err := adventures.FindByID(ctx, scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredAdventure.Claimed || !restoredAdventure.Resolved {
		t.Fatalf("restored adventure state = %#v", restoredAdventure)
	}
	if restoredAdventure.BattleResult.WinnerID != value.ID {
		t.Fatalf("restored battle result = %#v, want WinnerID = %s", restoredAdventure.BattleResult, value.ID)
	}

	valkeyClient.Do(ctx, valkeyClient.B().Del().Key("party2:scheduled:action:"+foundAction.ID).Build())
}
