package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestNewActivityRepositoryNilDB(t *testing.T) {
	repo, err := NewActivityRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewActivityRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestActivityRepositoryRestoresActivity(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := corecharacter.New("Activity Test")
	if err != nil {
		t.Fatal(err)
	}
	characters, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := characters.Save(context.Background(), character); err != nil {
		t.Fatal(err)
	}

	want := activity.Activity{
		ID:               character.ID,
		CharacterID:      character.ID,
		Type:             activity.TrainingType,
		StartedAt:        time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		AvailableAt:      time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC),
		ExperienceReward: activity.TrainingReward,
	}
	repository, err := NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.FindByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !got.StartedAt.Equal(want.StartedAt) || !got.AvailableAt.Equal(want.AvailableAt) ||
		got.ID != want.ID || got.CharacterID != want.CharacterID || got.Type != want.Type ||
		got.ExperienceReward != want.ExperienceReward || got.Claimed != want.Claimed {
		t.Fatalf("FindByID() = %#v, want %#v", got, want)
	}

	// FindByID not found
	if _, err := repository.FindByID(context.Background(), "nonexistent_activity"); !errors.Is(err, activity.ErrNotFound) {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, activity.ErrNotFound)
	}
}

func TestActivityRepositoryClaimAndApply(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := corecharacter.New("Claim Activity Test")
	if err != nil {
		t.Fatal(err)
	}
	characterRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := characterRepo.Save(context.Background(), character); err != nil {
		t.Fatal(err)
	}

	activityRepo, err := NewActivityRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	act := activity.Activity{
		ID:               character.ID,
		CharacterID:      character.ID,
		Type:             activity.TrainingType,
		StartedAt:        time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		AvailableAt:      time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC),
		ExperienceReward: activity.TrainingReward,
		Claimed:          false,
	}
	if err := activityRepo.Save(context.Background(), act); err != nil {
		t.Fatal(err)
	}

	character.Experience = 10
	if err := activityRepo.ClaimAndApply(context.Background(), act.ID, character); err != nil {
		t.Fatalf("ClaimAndApply() error = %v", err)
	}

	// Verify activity claimed
	claimedAct, err := activityRepo.FindByID(context.Background(), act.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !claimedAct.Claimed {
		t.Fatal("activity Claimed = false, want true")
	}

	// Verify character updated
	updatedChar, err := characterRepo.FindByID(context.Background(), character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedChar.Experience != 10 {
		t.Fatalf("character Experience = %d, want 10", updatedChar.Experience)
	}

	// Double claim should return ErrAlreadyClaimed
	if err := activityRepo.ClaimAndApply(context.Background(), act.ID, character); !errors.Is(err, activity.ErrAlreadyClaimed) {
		t.Fatalf("ClaimAndApply(already claimed) error = %v, want %v", err, activity.ErrAlreadyClaimed)
	}

	// Claiming nonexistent activity should return ErrNotFound
	if err := activityRepo.ClaimAndApply(context.Background(), "nonexistent_activity", character); !errors.Is(err, activity.ErrNotFound) {
		t.Fatalf("ClaimAndApply(nonexistent) error = %v, want %v", err, activity.ErrNotFound)
	}
}
