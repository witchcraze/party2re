package database

import (
	"context"
	"os"
	"reflect"
	"testing"

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

	character, err := CreateTestCharacter(context.Background(), db, "Job Test")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := corejob.NewCharacterJob(character.ID, "starter")
	target, _ := corejob.NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	if err := want.ChangeTo(target, 20, "unspecified"); err != nil {
		t.Fatal(err)
	}
	want.Master("vanguard")
	want.Master("paladin")
	want.RecordedJobSP = map[string]int{"vanguard": 120, "unmastered": 17, "zero": 0}
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
		len(got.History) != 1 || got.History[0] != want.History[0] ||
		len(got.MasteredJobs) != 2 || !got.IsMastered("vanguard") || !got.IsMastered("paladin") ||
		!reflect.DeepEqual(got.RecordedJobSP, want.RecordedJobSP) || got.IsMastered("unmastered") {
		t.Fatalf("FindByCharacterID() = %#v, want %#v", got, want)
	}
	// Partial snapshots never erase another recorded job's points.
	want.RecordedJobSP = map[string]int{"unmastered": 3}
	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err = repository.FindByCharacterID(context.Background(), character.ID)
	if err != nil || got.RecordedJobSP["unmastered"] != 3 || got.RecordedJobSP["vanguard"] != 120 {
		t.Fatal(got, err)
	}
	// A failure after parent/history writes rolls the complete save back.
	want.CurrentJobID = "bad-update"
	want.RecordedJobSP = map[string]int{"unmastered": -1}
	if err := repository.Save(context.Background(), want); err == nil {
		t.Fatal("invalid SP saved")
	}
	rolledBack, err := repository.FindByCharacterID(context.Background(), character.ID)
	if err != nil || !reflect.DeepEqual(rolledBack, got) {
		t.Fatal(rolledBack, got, err)
	}
}
