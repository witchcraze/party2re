package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestAdventureRepositoryPersistsAndLoadsResult(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := corecharacter.New("Adventure Test")
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

	want := adventure.Adventure{
		ID:               character.ID,
		CharacterID:      character.ID,
		Type:             adventure.StarterAdventure,
		StartedAt:        time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		AvailableAt:      time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC),
		ExperienceReward: adventure.AdventureReward,
		BattleResult: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: character.ID,
			LoserID:  adventure.AdventureEnemyID,
			Turns:    3,
		},
		Resolved: true,
		Claimed:  true,
	}
	repository, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := repository.FindByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.ID != want.ID || got.CharacterID != want.CharacterID || got.BattleResult != want.BattleResult ||
		!got.StartedAt.Equal(want.StartedAt) || !got.AvailableAt.Equal(want.AvailableAt) ||
		!got.Resolved || !got.Claimed {
		t.Fatalf("FindByID() = %#v, want %#v", got, want)
	}
}
