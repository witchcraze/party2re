package rescue_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/scheduling"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func TestRescueServiceIntegration_WithScheduling(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	rescueRepo, err := database.NewRescueRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := database.CreateTestCharacter(ctx, db, "RescueHero")
	if err != nil {
		t.Fatal(err)
	}

	var schedService *scheduling.Service
	var schedRepo *scheduling.ValkeyRepository
	if os.Getenv("PARTY2_VALKEY_ADDR") != "" {
		valkeyClient, err := vk.NewClient()
		if err == nil {
			defer valkeyClient.Close()
			schedRepo = scheduling.NewValkeyRepository(valkeyClient)
			schedService = scheduling.NewService(schedRepo)

			// Enqueue a scheduled action for this character
			_, err = schedService.Schedule(ctx, "adventure:complete", char.ID, map[string]string{"key": "val"}, time.Now().Add(1*time.Hour))
			if err != nil {
				t.Fatalf("failed to schedule action: %v", err)
			}
		}
	}

	svc := rescue.NewService(rescueRepo, charRepo, schedService)

	now := time.Now().UTC()
	rec, err := svc.EmergencyRescue(ctx, char.ID, "Stuck during integration test", now)
	if err != nil {
		t.Fatalf("EmergencyRescue failed: %v", err)
	}
	if rec.CharacterID != char.ID {
		t.Errorf("expected character ID %s, got %s", char.ID, rec.CharacterID)
	}

	// Verify penalty
	underPenalty, _, err := svc.IsUnderPenalty(ctx, char.ID, now.Add(10*time.Second))
	if err != nil || !underPenalty {
		t.Errorf("expected under penalty, got %v (err: %v)", underPenalty, err)
	}

	// If valkey was tested, verify actions for this character are canceled/cleared
	if schedRepo != nil {
		due, err := schedRepo.FetchDue(ctx, time.Now().Add(2*time.Hour), 10)
		if err != nil {
			t.Fatalf("FetchDue failed: %v", err)
		}
		for _, action := range due {
			if action.ActorID == char.ID {
				t.Errorf("scheduled action for %s was not canceled", char.ID)
			}
		}
	}
}
