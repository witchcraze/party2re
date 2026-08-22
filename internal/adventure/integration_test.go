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
	"github.com/witchcraze/party2re/internal/database"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

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
	value, err := characterService.Create(ctx, "Concurrent Adventure")
	if err != nil {
		t.Fatal(err)
	}

	clock := &fixedClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	adventures, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	starter, err := adventure.NewServiceWithClock(adventures, characters, corebattle.Engine{}, clock)
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
	first, err := adventure.NewServiceWithClock(firstRepository, characters, corebattle.Engine{}, clock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := adventure.NewServiceWithClock(secondRepository, characters, corebattle.Engine{}, clock)
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
	if restoredCharacter.Experience != adventure.AdventureReward {
		t.Fatalf("character experience = %d, want %d", restoredCharacter.Experience, adventure.AdventureReward)
	}
	restoredAdventure, err := adventures.FindByID(ctx, scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !restoredAdventure.Claimed || !restoredAdventure.Resolved {
		t.Fatalf("adventure state = %#v", restoredAdventure)
	}
}
