package job

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
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

func TestExchangeJobUnmasteredRecordedJobs(t *testing.T) {
	char := corecharacter.Character{
		ID: "character-1", JobID: "job-03", SP: 9, OldJobID: "job-04", OldSP: 11,
	}
	state, err := corejob.NewCharacterJob(char.ID, char.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RecordJobSP("job-01", 17); err != nil {
		t.Fatal(err)
	}
	if err := state.RecordJobSP("job-02", 25); err != nil {
		t.Fatal(err)
	}
	if state.IsMastered("job-01") || state.IsMastered("job-02") {
		t.Fatal("expected jobs to be unmastered")
	}

	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	inventory, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := item.NewInstance("item-168", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.Add(token); err != nil {
		t.Fatal(err)
	}
	invRepo := &inventoryRepoStub{inventory: inventory}
	svc, err := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))
	if err != nil {
		t.Fatal(err)
	}

	updated, updatedState, err := svc.ExchangeJob(context.Background(), char.ID, "job-01", "job-02", "item-168")
	if err != nil {
		t.Fatal(err)
	}
	if updated.JobID != "job-01" || updated.SP != 17 || updated.OldJobID != "job-02" || updated.OldSP != 25 {
		t.Fatalf("unexpected updated character: %#v", updated)
	}
	if updated.JobMemory == nil || updated.JobMemory.JobID != "job-03" || updated.JobMemory.SP != 9 ||
		updated.JobMemory.OldJobID != "job-04" || updated.JobMemory.OldSP != 11 ||
		updated.JobMemory.Kind != corecharacter.JobMemoryKindPersistent {
		t.Fatalf("unexpected job memory: %#v", updated.JobMemory)
	}
	if updatedState.CurrentJobID != "job-01" || repo.value.CurrentJobID != "job-01" {
		t.Fatalf("expected CurrentJobID job-01, got %s / %s", updatedState.CurrentJobID, repo.value.CurrentJobID)
	}
	if updatedState.IsMastered("job-01") || updatedState.IsMastered("job-02") {
		t.Fatal("expected jobs to remain unmastered")
	}
	if invRepo.inventory.Quantity("item-168") != 0 {
		t.Fatalf("expected item-168 consumed, got %d", invRepo.inventory.Quantity("item-168"))
	}

	restored, restoredState, err := svc.ExchangeJob(context.Background(), char.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if restored.JobID != "job-03" || restored.SP != 9 || restored.OldJobID != "job-04" || restored.OldSP != 11 || restored.JobMemory != nil {
		t.Fatalf("unexpected restored character: %#v", restored)
	}
	if restoredState.CurrentJobID != "job-03" || repo.value.CurrentJobID != "job-03" {
		t.Fatalf("expected restored CurrentJobID job-03, got %s / %s", restoredState.CurrentJobID, repo.value.CurrentJobID)
	}
}

func TestExchangeJobRejectsUnrecordedAndUnknownSP(t *testing.T) {
	char := corecharacter.Character{
		ID: "character-1", JobID: "job-03", SP: 9, OldJobID: "job-04", OldSP: 11, Gender: "m",
	}
	state, err := corejob.NewCharacterJob(char.ID, char.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.RecordJobSP("job-01", 17); err != nil {
		t.Fatal(err)
	}
	if state.RecordedJobSP == nil {
		state.RecordedJobSP = make(map[string]int)
	}
	state.RecordedJobSP["job-05"] = 30
	state.History = append(state.History, corejob.Change{FromJobID: "job-06", ToJobID: "job-03"})

	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	inventory, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := item.NewInstance("item-168", 10)
	_ = inventory.Add(token)
	invRepo := &inventoryRepoStub{inventory: inventory}
	svc, err := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	cases := []struct {
		name      string
		target    string
		targetOld string
	}{
		{"unrecorded target", "job-99", "job-01"},
		{"unrecorded targetOld", "job-01", "job-99"},
		{"migration-only unrecorded target", "job-05", "job-01"},
		{"history only unknown SP target", "job-06", "job-01"},
		{"history only unknown SP targetOld", "job-01", "job-06"},
		{"same target and targetOld", "job-01", "job-01"},
		{"empty target", "", "job-01"},
		{"empty targetOld", "job-01", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.ExchangeJob(ctx, char.ID, tc.target, tc.targetOld, "item-168")
			if !errors.Is(err, corejob.ErrJobUnavailable) {
				t.Fatalf("expected ErrJobUnavailable, got %v", err)
			}
		})
	}
}
