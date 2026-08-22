package database

import (
	"context"
	"os"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

func TestCharacterJobRepositoryPersistsAndLoadsHistory(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	character, err := corecharacter.New("Job Test")
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
	want, _ := corejob.NewCharacterJob(character.ID, "starter")
	target, _ := corejob.NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	if err := want.ChangeTo(target, 1, "unspecified"); err != nil {
		t.Fatal(err)
	}
	repository, err := NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := repository.FindByCharacterID(context.Background(), character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CharacterID != want.CharacterID || got.CurrentJobID != want.CurrentJobID ||
		len(got.History) != 1 || got.History[0] != want.History[0] {
		t.Fatalf("FindByCharacterID() = %#v, want %#v", got, want)
	}
}
