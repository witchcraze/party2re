package job

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

func TestChangeJobRecordsUnmasteredAndLatestMasteredPoints(t *testing.T) {
	for _, sp := range []int{17, 120, 20} {
		char := corecharacter.Character{ID: "character-1", JobID: "job-01", Gender: "m", Level: 20, SP: sp, Stats: corecharacter.Stats{MaxHP: 40, MaxMP: 20, HP: 40, MP: 20, Attack: 20, Defense: 20, Agility: 20}}
		state, err := corejob.NewCharacterJob(char.ID, char.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if sp != 17 {
			state.RecordMastery(char.JobID, 80, 80)
		}
		repo := &repositoryStub{value: state}
		chars := &charRepoStub{char: char}
		catalog, err := corejob.NewCatalog([]corejob.Definition{{ID: "job-01", Name: "First", CMPTier: 1, MasterySP: 80}, {ID: "job-02", Name: "Next", CMPTier: 1}})
		if err != nil {
			t.Fatal(err)
		}
		inv, err := coreinventory.New(char.ID)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(repo, WithCatalog(catalog), WithCharacterRepository(chars), WithInventoryRepository(&inventoryRepoStub{inventory: inv}))
		if err != nil {
			t.Fatal(err)
		}
		_, got, err := svc.ChangeJob(context.Background(), char.ID, "job-02")
		if err != nil || got.RecordedJobSP["job-01"] != sp || got.IsMastered("job-01") != (sp != 17) {
			t.Fatal(got, err)
		}
		if sp != 17 && got.MasteredJobSP["job-01"] != sp {
			t.Fatal(got)
		}
	}
}

func TestFutureMemoryDoesNotCreateUnknownRecords(t *testing.T) {
	state, err := corejob.NewCharacterJob("character", "job-02")
	if err != nil {
		t.Fatal(err)
	}
	state.History = []corejob.Change{{FromJobID: "job-01", ToJobID: "job-02"}}
	state.RecordedJobSP = map[string]int{"job-02": 9}
	char := corecharacter.Character{JobID: "job-02", SP: 50, OldJobID: "job-01", OldSP: 17}
	if err := refreshFutureMemoryPoints(&state, char); err != nil {
		t.Fatal(err)
	}
	if state.RecordedJobSP["job-01"] != 17 {
		t.Fatal(state)
	}
	if state.RecordedJobSP["job-02"] != 9 || state.HasRecordedJob("job-02") {
		t.Fatal("migration-only SP was updated or became a departure record", state)
	}
	sp, oldSP := futureMemoryPoints(state, char, corecharacter.FutureMemory{JobID: "unknown", OldJobID: "job-01"})
	if sp != 50 || oldSP != 17 {
		t.Fatal(sp, oldSP)
	}
	sp, oldSP = futureMemoryPoints(state, char, corecharacter.FutureMemory{JobID: "job-02", OldJobID: "job-01"})
	if sp != 50 || oldSP != 17 {
		t.Fatal("recall used preservation-only SP", sp, oldSP)
	}
}
