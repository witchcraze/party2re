package database

import (
	"context"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestNewCharacterRepositoryNilDB(t *testing.T) {
	repo, err := NewCharacterRepository(nil)
	if err == nil || repo != nil {
		t.Fatalf("NewCharacterRepository(nil) = (%v, %v), want error", repo, err)
	}
}

func TestCharacterRepositoryPersistsAndLoadsCharacter(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	repository, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	name := "RepoTest_" + player.ID[:8]
	want, err := corecharacter.New(name)
	if err != nil {
		t.Fatal(err)
	}
	want.PlayerID = player.ID

	if err := repository.Save(ctx, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := repository.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got != want {
		t.Fatalf("FindByID() = %#v, want %#v", got, want)
	}

	// FindByPlayerID
	byPlayer, err := repository.FindByPlayerID(ctx, player.ID)
	if err != nil {
		t.Fatalf("FindByPlayerID() error = %v", err)
	}
	if len(byPlayer) != 1 || byPlayer[0] != want {
		t.Fatalf("FindByPlayerID() = %#v, want [%#v]", byPlayer, want)
	}

	// Update character
	want.Level = 2
	want.Experience = 20
	want.Money = 350
	want.Stats.HP = 25
	want.RebirthCount = 1
	if err := repository.Update(ctx, want); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, err := repository.FindByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("FindByID() after update error = %v", err)
	}
	if updated != want {
		t.Fatalf("FindByID() after update = %#v, want %#v", updated, want)
	}

	// FindByName & FindByNameForUpdate
	byName, err := repository.FindByName(ctx, want.Name)
	if err != nil || byName.ID != want.ID {
		t.Fatalf("FindByName() error = %v, got %+v", err, byName)
	}

	byNameUpdate, err := repository.FindByNameForUpdate(ctx, want.Name)
	if err != nil || byNameUpdate.ID != want.ID {
		t.Fatalf("FindByNameForUpdate() error = %v, got %+v", err, byNameUpdate)
	}

	// Profile Operations
	defaultProf, err := repository.GetProfile(ctx, want.ID)
	if err != nil || defaultProf.CharacterID != want.ID {
		t.Fatalf("GetProfile(default) error = %v, got %+v", err, defaultProf)
	}

	profToSave := defaultProf
	profToSave.Comment = "Testing bio"
	profToSave.AvatarURL = "https://example.com/avatar.png"
	profToSave.BioData = map[string]string{
		"hobby": "Coding",
	}
	if err := repository.SaveProfile(ctx, profToSave); err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}

	loadedProf, err := repository.GetProfile(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetProfile() after save error = %v", err)
	}
	if loadedProf.Comment != "Testing bio" || loadedProf.AvatarURL != "https://example.com/avatar.png" || loadedProf.BioData["hobby"] != "Coding" {
		t.Fatalf("unexpected loaded profile: %+v", loadedProf)
	}

	// Nonexistent character errors
	nonexistent, _ := corecharacter.New("Nonexistent")
	if err := repository.Update(ctx, nonexistent); err != corecharacter.ErrNotFound {
		t.Fatalf("Update(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
	if _, err := repository.FindByID(ctx, "nonexistent_id"); err != corecharacter.ErrNotFound {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
}
