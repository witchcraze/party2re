package database

import (
	"context"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

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
}
