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

	repository, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	want, err := corecharacter.New("Repository Test")
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
	if got != want {
		t.Fatalf("FindByID() = %#v, want %#v", got, want)
	}

	// Update character
	want.Level = 2
	want.Experience = 20
	want.Money = 350
	want.Stats.HP = 25
	want.RebirthCount = 1
	if err := repository.Update(context.Background(), want); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, err := repository.FindByID(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("FindByID() after update error = %v", err)
	}
	if updated != want {
		t.Fatalf("FindByID() after update = %#v, want %#v", updated, want)
	}

	// Nonexistent character errors
	nonexistent, _ := corecharacter.New("Nonexistent")
	if err := repository.Update(context.Background(), nonexistent); err != corecharacter.ErrNotFound {
		t.Fatalf("Update(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
	if _, err := repository.FindByID(context.Background(), "nonexistent_id"); err != corecharacter.ErrNotFound {
		t.Fatalf("FindByID(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
}
