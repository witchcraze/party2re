package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNewAdventureRepositoryNilDB(t *testing.T) {
	repo, err := NewAdventureRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewAdventureRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestAdventureRepositoryPersistsAndLoadsResult(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := CreateTestCharacter(context.Background(), db, "Adventure Test")
	if err != nil {
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

	// FindByID not found
	if _, err := repository.FindByID(context.Background(), "nonexistent_adventure"); !errors.Is(err, adventure.ErrNotFound) {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, adventure.ErrNotFound)
	}
}

func TestAdventureRepositoryClaimAndApply(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	characterRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	character, err := CreateTestCharacter(context.Background(), db, "Claim Adventure Test")
	if err != nil {
		t.Fatal(err)
	}

	adventureRepo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	adv := adventure.Adventure{
		ID:               character.ID,
		CharacterID:      character.ID,
		Type:             adventure.StarterAdventure,
		StartedAt:        time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		AvailableAt:      time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC),
		ExperienceReward: adventure.AdventureReward,
		Resolved:         false,
		Claimed:          false,
	}
	if err := adventureRepo.Save(context.Background(), adv); err != nil {
		t.Fatal(err)
	}

	// Resolve and claim
	adv.Resolved = true
	adv.Claimed = true
	adv.BattleResult = corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: character.ID,
		LoserID:  adventure.AdventureEnemyID,
		Turns:    2,
		Reward: corebattle.Reward{
			Experience: 20,
			Currency:   50,
		},
	}
	character.Experience = 20
	character.Money += 50

	if err := adventureRepo.ClaimAndApply(context.Background(), adv, character); err != nil {
		t.Fatalf("ClaimAndApply() error = %v", err)
	}

	// Verify adventure claimed and result stored
	claimedAdv, err := adventureRepo.FindByID(context.Background(), adv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !claimedAdv.Claimed || !claimedAdv.Resolved || claimedAdv.BattleResult != adv.BattleResult {
		t.Fatalf("claimed adventure = %#v, want %#v", claimedAdv, adv)
	}

	// Verify character updated
	updatedChar, err := characterRepo.FindByID(context.Background(), character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedChar.Experience != 20 || updatedChar.Money != character.Money {
		t.Fatalf("character = %#v, want Experience 20 and Money %d", updatedChar, character.Money)
	}

	// Double claim returns ErrAlreadyClaimed
	if err := adventureRepo.ClaimAndApply(context.Background(), adv, character); !errors.Is(err, adventure.ErrAlreadyClaimed) {
		t.Fatalf("ClaimAndApply(already claimed) error = %v, want %v", err, adventure.ErrAlreadyClaimed)
	}

	// Claim nonexistent adventure returns ErrNotFound
	adv.ID = "nonexistent_adv"
	if err := adventureRepo.ClaimAndApply(context.Background(), adv, character); !errors.Is(err, adventure.ErrNotFound) {
		t.Fatalf("ClaimAndApply(nonexistent) error = %v, want %v", err, adventure.ErrNotFound)
	}
}
