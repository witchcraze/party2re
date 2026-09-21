package job_test

import (
	"context"
	"errors"
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

func TestFutureMemory_ValidationAndErrors(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{
		ID:         "char-val-1",
		Name:       "ValHero",
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
	futureRepo := &testFutureMemoryRepo{}

	// 1. Service without character repository
	svcNoChar, _ := job.NewService(jobRepo, job.WithFutureMemoryRepository(futureRepo))
	if _, err := svcNoChar.SaveFutureMemory(ctx, char.ID); err == nil || err.Error() != "character repository is nil" {
		t.Fatalf("expected character repository is nil error, got %v", err)
	}
	if _, _, err := svcNoChar.RecallFutureMemory(ctx, char.ID, "mem-1"); err == nil || err.Error() != "character repository is nil" {
		t.Fatalf("expected character repository is nil error in recall, got %v", err)
	}

	// 2. Service without future memory repository
	charRepo := &testCharRepo{char: char}
	svcNoFuture, _ := job.NewService(jobRepo, job.WithCharacterRepository(charRepo))
	if _, err := svcNoFuture.SaveFutureMemory(ctx, char.ID); err == nil || err.Error() != "future memory repository is nil" {
		t.Fatalf("expected future memory repository is nil error, got %v", err)
	}
	if _, _, err := svcNoFuture.RecallFutureMemory(ctx, char.ID, "mem-1"); err == nil || err.Error() != "future memory repository is nil" {
		t.Fatalf("expected future memory repository is nil error in recall, got %v", err)
	}
	// ListFutureMemories returns nil, nil when repository is nil
	if mems, err := svcNoFuture.ListFutureMemories(ctx, char.ID); err != nil || mems != nil {
		t.Fatalf("expected nil, nil when future repo is nil: mems=%v, err=%v", mems, err)
	}

	// 3. SaveFutureMemory & RecallFutureMemory when character has JobMemory
	charWithMemory := char
	charWithMemory.JobMemory = &corecharacter.JobMemory{JobID: "job-02", SP: 10}
	charRepoMem := &testCharRepo{char: charWithMemory}
	svcWithMem, _ := job.NewService(jobRepo, job.WithCharacterRepository(charRepoMem), job.WithFutureMemoryRepository(futureRepo))
	if _, err := svcWithMem.SaveFutureMemory(ctx, char.ID); !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when JobMemory is present, got %v", err)
	}
	if _, _, err := svcWithMem.RecallFutureMemory(ctx, char.ID, "mem-1"); !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable in recall when JobMemory is present, got %v", err)
	}

	// 4. Non-economy mode: inventory repo is nil -> ErrRequiredItem
	svcNoInv, _ := job.NewService(jobRepo, job.WithCharacterRepository(charRepo), job.WithFutureMemoryRepository(futureRepo))
	if _, err := svcNoInv.SaveFutureMemory(ctx, char.ID); !errors.Is(err, job.ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when inventory is nil, got %v", err)
	}

	// 5. Non-economy mode: inventory has 0 item-207 -> ErrRequiredItem
	emptyInv, _ := coreinventory.New(char.ID)
	invRepo := &testInventoryRepo{inventory: emptyInv}
	svcWithInv, _ := job.NewService(jobRepo, job.WithCharacterRepository(charRepo), job.WithInventoryRepository(invRepo), job.WithFutureMemoryRepository(futureRepo))
	if _, err := svcWithInv.SaveFutureMemory(ctx, char.ID); !errors.Is(err, job.ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when fragment is missing, got %v", err)
	}

	// 6. Non-economy mode: inventory has item-207 -> successfully saved and consumed
	frag, _ := item.NewInstance("item-207", 1)
	_ = invRepo.inventory.Add(frag)
	snapshot, err := svcWithInv.SaveFutureMemory(ctx, char.ID)
	if err != nil {
		t.Fatalf("SaveFutureMemory failed in non-economy mode: %v", err)
	}
	if invRepo.inventory.Quantity("item-207") != 0 {
		t.Fatalf("expected item-207 consumed, got %d", invRepo.inventory.Quantity("item-207"))
	}

	// 7. OverFuture capacity reached: OverFuture=0 allows 1 memory (len=1 is already at limit)
	_ = invRepo.inventory.Add(frag)
	if _, err := svcWithInv.SaveFutureMemory(ctx, char.ID); err == nil || err.Error() != "future memory slot limit reached" {
		t.Fatalf("expected future memory slot limit reached error, got %v", err)
	}

	// 8. RecallFutureMemory when memory ID does not exist -> error "future memory not found"
	if _, _, err := svcWithInv.RecallFutureMemory(ctx, char.ID, "non-existent-id"); err == nil || err.Error() != "future memory not found" {
		t.Fatalf("expected future memory not found error, got %v", err)
	}

	// 9. RecallFutureMemory in non-economy mode -> succeeds
	charRepo.char.JobID = "job-01"
	recalledChar, _, err := svcWithInv.RecallFutureMemory(ctx, char.ID, snapshot.ID)
	if err != nil {
		t.Fatalf("RecallFutureMemory in non-economy mode failed: %v", err)
	}
	if recalledChar.JobID != "job-05" {
		t.Fatalf("expected job-05 restored, got %s", recalledChar.JobID)
	}

	// 10. RecallFutureMemory with economy mode when memory not found
	ecoCharRepo := &testCharRepo{char: char}
	ecoInvRepo := &testInventoryRepo{inventory: emptyInv}
	ecoSvc, _ := economy.NewService(ecoCharRepo, ecoInvRepo)
	svcEco, _ := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
		job.WithInventoryRepository(ecoInvRepo),
		job.WithEconomy(ecoSvc),
		job.WithFutureMemoryRepository(futureRepo),
	)
	if _, _, err := svcEco.RecallFutureMemory(ctx, char.ID, "non-existent-id"); err == nil || err.Error() != "future memory not found" {
		t.Fatalf("expected future memory not found error in economy mode, got %v", err)
	}

	// 11. RecallFutureMemory with economy mode when character has JobMemory inside tx
	charEcoWithMem := char
	charEcoWithMem.JobMemory = &corecharacter.JobMemory{JobID: "job-02", SP: 10}
	ecoCharRepoWithMem := &testCharRepo{char: charEcoWithMem}
	ecoSvcWithMem, _ := economy.NewService(ecoCharRepoWithMem, ecoInvRepo)
	svcEcoMem, _ := job.NewService(
		jobRepo,
		job.WithCharacterRepository(ecoCharRepoWithMem),
		job.WithInventoryRepository(ecoInvRepo),
		job.WithEconomy(ecoSvcWithMem),
		job.WithFutureMemoryRepository(futureRepo),
	)
	if _, _, err := svcEcoMem.RecallFutureMemory(ctx, char.ID, "any-id"); !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable in economy recall, got %v", err)
	}
}
