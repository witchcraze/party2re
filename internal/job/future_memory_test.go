package job_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/job"
)

type testFutureMemoryRepo struct {
	memories []corecharacter.FutureMemory
}

func (f *testFutureMemoryRepo) Save(_ context.Context, memory corecharacter.FutureMemory) error {
	f.memories = append(f.memories, memory)
	return nil
}

func (f *testFutureMemoryRepo) FindByCharacterID(_ context.Context, characterID string) ([]corecharacter.FutureMemory, error) {
	var results []corecharacter.FutureMemory
	for _, m := range f.memories {
		if m.CharacterID == characterID {
			results = append(results, m)
		}
	}
	return results, nil
}

func (f *testFutureMemoryRepo) Delete(_ context.Context, characterID, memoryID string) error {
	filtered := make([]corecharacter.FutureMemory, 0, len(f.memories))
	for _, m := range f.memories {
		if m.CharacterID == characterID && m.ID == memoryID {
			continue
		}
		filtered = append(filtered, m)
	}
	f.memories = filtered
	return nil
}

type testJobRepo struct {
	value corejob.CharacterJob
}

func (r *testJobRepo) Save(_ context.Context, value corejob.CharacterJob) error {
	r.value = value
	return nil
}

func (r *testJobRepo) FindByCharacterID(_ context.Context, _ string) (corejob.CharacterJob, error) {
	return r.value, nil
}

type testCharRepo struct {
	char corecharacter.Character
}

func (c *testCharRepo) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	return c.char, nil
}

func (c *testCharRepo) FindByIDForUpdate(_ context.Context, _ string) (corecharacter.Character, error) {
	return c.char, nil
}

func (c *testCharRepo) Update(_ context.Context, char corecharacter.Character) error {
	c.char = char
	return nil
}

type testInventoryRepo struct {
	inventory coreinventory.Inventory
}

func (r *testInventoryRepo) FindByCharacterID(_ context.Context, _ string) (coreinventory.Inventory, error) {
	return r.inventory, nil
}

func (r *testInventoryRepo) FindByCharacterIDForUpdate(_ context.Context, _ string) (coreinventory.Inventory, error) {
	return r.inventory, nil
}

func (r *testInventoryRepo) Save(_ context.Context, value coreinventory.Inventory) error {
	r.inventory = value
	return nil
}

func TestFutureMemory_TransactionalLifecycle(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:         "char-tx-1",
		Name:       "TxHero",
		JobID:      "job-05",
		OldJobID:   "job-01",
		Level:      50,
		Experience: 12000,
		SP:         80,
		OldSP:      40,
		Gender:     "female",
		OverLevel:  true,
		OverFuture: 0,
		Stats: corecharacter.Stats{
			MaxHP:   600,
			MaxMP:   300,
			HP:      200,
			MP:      100,
			Attack:  150,
			Defense: 110,
			Agility: 90,
		},
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	jobRepo := &testJobRepo{value: state}
	charRepo := &testCharRepo{char: char}
	futureRepo := &testFutureMemoryRepo{}

	inventory, _ := coreinventory.New(char.ID)
	frag, _ := item.NewInstance("item-207", 2)
	_ = inventory.Add(frag)
	invRepo := &testInventoryRepo{inventory: inventory}

	economySvc, err := economy.NewService(charRepo, invRepo)
	if err != nil {
		t.Fatalf("economy.NewService failed: %v", err)
	}

	svc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
		job.WithInventoryRepository(invRepo),
		job.WithEconomy(economySvc),
		job.WithFutureMemoryRepository(futureRepo),
	)
	if err != nil {
		t.Fatalf("job.NewService failed: %v", err)
	}

	// 1. Save future memory through economy transaction runner
	snapshot, err := svc.SaveFutureMemory(ctx, char.ID)
	if err != nil {
		t.Fatalf("SaveFutureMemory failed: %v", err)
	}
	if snapshot.JobID != "job-05" || snapshot.Level != 50 || snapshot.MaxHP != 600 {
		t.Fatalf("unexpected saved snapshot: %#v", snapshot)
	}
	// Item consumed
	if invRepo.inventory.Quantity("item-207") != 1 {
		t.Fatalf("expected 1 fragment remaining, got %d", invRepo.inventory.Quantity("item-207"))
	}
	// SP saved to state
	if jobRepo.value.MasteredJobSP["job-05"] != 80 || jobRepo.value.MasteredJobSP["job-01"] != 40 {
		t.Fatalf("expected SP recorded in state: %#v", jobRepo.value.MasteredJobSP)
	}

	// 2. Mutate character state (e.g. changed to a new job at level 1)
	charRepo.char.JobID = "job-02"
	charRepo.char.OldJobID = "job-05"
	charRepo.char.Level = 1
	charRepo.char.Experience = 0
	charRepo.char.Stats = corecharacter.Stats{MaxHP: 30, MaxMP: 10, HP: 30, MP: 10, Attack: 10, Defense: 10, Agility: 10}
	charRepo.char.SP = 0
	charRepo.char.OldSP = 80
	charRepo.char.OverLevel = false

	// 3. Recall future memory through economy transaction runner
	restoredChar, restoredState, err := svc.RecallFutureMemory(ctx, char.ID, snapshot.ID)
	if err != nil {
		t.Fatalf("RecallFutureMemory failed: %v", err)
	}

	if restoredChar.JobID != "job-05" || restoredChar.OldJobID != "job-01" || restoredChar.Level != 50 ||
		restoredChar.Experience != 12000 || !restoredChar.OverLevel {
		t.Fatalf("unexpected restored char: %#v", restoredChar)
	}
	if restoredChar.Stats.MaxHP != 600 || restoredChar.Stats.HP != 600 || restoredChar.Stats.MaxMP != 300 || restoredChar.Stats.MP != 300 {
		t.Fatalf("unexpected restored stats: %#v", restoredChar.Stats)
	}
	if restoredChar.SP != 80 || restoredChar.OldSP != 40 {
		t.Fatalf("unexpected restored SP: SP=%d, OldSP=%d", restoredChar.SP, restoredChar.OldSP)
	}
	if restoredState.CurrentJobID != "job-05" {
		t.Fatalf("expected CurrentJobID to be restored to job-05, got %s", restoredState.CurrentJobID)
	}

	// Memory slot should be consumed / removed
	memories, err := svc.ListFutureMemories(ctx, char.ID)
	if err != nil || len(memories) != 0 {
		t.Fatalf("expected memories to be empty, got %v (err: %v)", memories, err)
	}
}
