package job

import (
	"context"
	"errors"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

func templeService(t *testing.T, char corecharacter.Character, items ...string) (*Service, *inventoryRepoStub) {
	t.Helper()
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	state.RecordedJobSP = map[string]int{"job-01": 30, "job-05": 12}
	state.History = []corejob.Change{{FromJobID: "job-05", ToJobID: "job-01"}, {FromJobID: "job-01", ToJobID: "job-05"}}
	inv, _ := coreinventory.New(char.ID)
	for _, id := range items {
		instance, _ := item.NewInstance(id, 1)
		_ = inv.Add(instance)
	}
	invRepo := &inventoryRepoStub{inventory: inv}
	svc, err := NewService(&repositoryStub{value: state}, WithCharacterRepository(&charRepoStub{char: char}), WithInventoryRepository(invRepo),
		WithFutureMemoryRepository(&futureMemoryRepoStub{memories: []corecharacter.FutureMemory{{ID: "fm1", CharacterID: char.ID}}}))
	if err != nil {
		t.Fatal(err)
	}
	return svc, invRepo
}

func ids(defs []corejob.Definition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.ID)
	}
	return out
}

func TestTempleStatusListsAcceptedChoices(t *testing.T) {
	char := corecharacter.Character{ID: "c1", JobID: "job-05", OldJobID: "job-01", Level: 25, Gender: "male"}
	svc, _ := templeService(t, char, "item-168")
	status, err := svc.TempleStatus(context.Background(), char.ID)
	if err != nil {
		t.Fatal(err)
	}
	changeable := ids(status.Changeable)
	// job-33 needs item-027 and job-84 worn armor-29; neither is held.
	if !slices.Contains(changeable, "job-02") || slices.Contains(changeable, "job-33") || slices.Contains(changeable, "job-84") || slices.Contains(changeable, "starter") {
		t.Fatalf("changeable %v", changeable)
	}
	if got := ids(status.Recallable); !slices.Equal(got, []string{"job-01", "job-05"}) {
		t.Fatalf("recallable %v", got)
	}
	if status.RecallItem != "item-168" || len(status.FutureMemories) != 1 {
		t.Fatalf("item %q memories %v", status.RecallItem, status.FutureMemories)
	}

	svc, _ = templeService(t, char, "item-027")
	if status, _ = svc.TempleStatus(context.Background(), char.ID); !slices.Contains(ids(status.Changeable), "job-33") || status.RecallItem != "" {
		t.Fatalf("held item-027: %v %q", ids(status.Changeable), status.RecallItem)
	}
}

func TestTempleStatusOffersNoChangeOrRecallDuringMemoryOrBelowLevel(t *testing.T) {
	for _, char := range []corecharacter.Character{
		{ID: "c1", JobID: "job-05", OldJobID: "job-01", Level: 25, JobMemory: &corecharacter.JobMemory{JobID: "job-02"}},
		{ID: "c1", JobID: "job-05", OldJobID: "job-01", Level: 25, OverLevel: true},
	} {
		svc, _ := templeService(t, char, "item-168")
		status, err := svc.TempleStatus(context.Background(), char.ID)
		if err != nil || len(status.Recallable) != 0 || (char.JobMemory != nil && len(status.Changeable) != 0) {
			t.Fatalf("%+v: %v %v %v", char, err, ids(status.Changeable), ids(status.Recallable))
		}
	}
	svc, _ := templeService(t, corecharacter.Character{ID: "c1", JobID: "job-05", Level: 19})
	if status, err := svc.TempleStatus(context.Background(), "c1"); err != nil || len(status.Changeable) != 0 {
		t.Fatalf("below level 20: %v %v", err, ids(status.Changeable))
	}
}

func TestTempleStatusFailsOnRequiredRead(t *testing.T) {
	sentinel := errors.New("inventory down")
	svc, _ := templeService(t, corecharacter.Character{ID: "c1", JobID: "job-05", Level: 25})
	svc.inventories = &errInventoryRepoStub{findErr: sentinel}
	if _, err := svc.TempleStatus(context.Background(), "c1"); !errors.Is(err, sentinel) {
		t.Fatalf("err %v", err)
	}
}

func TestExchangeJobRequiresAHeldRecallItem(t *testing.T) {
	char := corecharacter.Character{ID: "c1", JobID: "job-05", OldJobID: "job-01", Level: 25}
	svc, invRepo := templeService(t, char)
	runner, err := economy.NewService(&charRepoStub{char: char}, invRepo)
	if err != nil {
		t.Fatal(err)
	}
	svc.economy = runner
	if _, _, err := svc.ExchangeJob(context.Background(), char.ID, "job-01", "job-05"); !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("empty hand: %v", err)
	}
	sentinel := errors.New("inventory down")
	svc.inventories = &errInventoryRepoStub{findErr: sentinel}
	if _, _, err := svc.ExchangeJob(context.Background(), char.ID, "job-01", "job-05"); !errors.Is(err, sentinel) {
		t.Fatalf("read error: %v", err)
	}
}
