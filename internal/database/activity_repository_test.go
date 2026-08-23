package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

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
}
