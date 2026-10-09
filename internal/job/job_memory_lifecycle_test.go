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

type memoryTestCharRepo struct {
	chars map[string]corecharacter.Character
}

func newMemoryTestCharRepo() *memoryTestCharRepo {
	return &memoryTestCharRepo{chars: make(map[string]corecharacter.Character)}
}

func (r *memoryTestCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *memoryTestCharRepo) FindByIDForUpdate(_ context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(context.Background(), id)
}

func (r *memoryTestCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.chars[c.ID] = c
	return nil
}

type memoryTestJobRepo struct {
	jobs map[string]corejob.CharacterJob
}

func newMemoryTestJobRepo() *memoryTestJobRepo {
	return &memoryTestJobRepo{jobs: make(map[string]corejob.CharacterJob)}
}

func (r *memoryTestJobRepo) FindByCharacterID(_ context.Context, id string) (corejob.CharacterJob, error) {
	j, ok := r.jobs[id]
	if !ok {
		return corejob.CharacterJob{}, errors.New("job not found")
	}
	return j, nil
}

func (r *memoryTestJobRepo) Save(_ context.Context, j corejob.CharacterJob) error {
	r.jobs[j.CharacterID] = j
	return nil
}

type memoryTestInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func newMemoryTestInvRepo() *memoryTestInvRepo {
	return &memoryTestInvRepo{invs: make(map[string]coreinventory.Inventory)}
}

func (r *memoryTestInvRepo) FindByCharacterID(_ context.Context, id string) (coreinventory.Inventory, error) {
	inv, ok := r.invs[id]
	if !ok {
		inv, _ = coreinventory.New(id)
		r.invs[id] = inv
	}
	return inv, nil
}

func (r *memoryTestInvRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, id)
}

func (r *memoryTestInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.invs[inv.CharacterID] = inv
	return nil
}

func setupJobMemoryService(t *testing.T) (*job.Service, *memoryTestCharRepo, *memoryTestJobRepo, *memoryTestInvRepo) {
	t.Helper()
	charRepo := newMemoryTestCharRepo()
	jobRepo := newMemoryTestJobRepo()
	invRepo := newMemoryTestInvRepo()

	ecoSvc, err := economy.NewService(charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create economy service: %v", err)
	}

	jobSvc, err := job.NewService(
		jobRepo,
		job.WithCharacterRepository(charRepo),
		job.WithInventoryRepository(invRepo),
		job.WithEconomy(ecoSvc),
	)
	if err != nil {
		t.Fatalf("failed to create job service: %v", err)
	}

	return jobSvc, charRepo, jobRepo, invRepo
}

func TestJobMemory_PersistentLifecycle_Item168(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, jobRepo, invRepo := setupJobMemoryService(t)

	charID := "char-168"
	char := corecharacter.Character{
		ID:       charID,
		Name:     "Tester168",
		JobID:    "job-01",
		OldJobID: "job-02",
		SP:       50,
		OldSP:    40,
		Gender:   "m",
	}
	charRepo.chars[charID] = char

	jobState, _ := corejob.NewCharacterJob(charID, "job-01")
	jobState.RecordMastery("job-03", 80, 80)
	jobState.RecordMastery("job-04", 100, 100)
	jobRepo.jobs[charID] = jobState

	inv, _ := coreinventory.New(charID)
	item168, _ := item.NewInstance("item-168", 1)
	_ = inv.Add(item168)
	invRepo.invs[charID] = inv

	// 1. Start persistent memory using item-168
	updatedChar, updatedJob, err := svc.ExchangeJob(ctx, charID, "job-03", "job-04", "item-168")
	if err != nil {
		t.Fatalf("ExchangeJob with item-168 failed: %v", err)
	}

	// Verify item-168 was consumed once
	curInv168 := invRepo.invs[charID]
	if curInv168.Quantity("item-168") != 0 {
		t.Fatalf("expected item-168 quantity to be 0, got %d", curInv168.Quantity("item-168"))
	}

	// Verify updated character state and synchronized current job
	if updatedChar.JobID != "job-03" || updatedChar.SP != 80 || updatedChar.OldJobID != "job-04" || updatedChar.OldSP != 100 {
		t.Fatalf("unexpected updated character: %+v", updatedChar)
	}
	if updatedJob.CurrentJobID != "job-03" {
		t.Fatalf("expected character_jobs.current_job_id = job-03, got %s", updatedJob.CurrentJobID)
	}

	// Verify persistent JobMemory was saved
	if updatedChar.JobMemory == nil {
		t.Fatal("expected JobMemory to be set")
	}
	if updatedChar.JobMemory.Kind != corecharacter.JobMemoryKindPersistent || !updatedChar.JobMemory.IsPersistent() || updatedChar.JobMemory.IsTemporary() {
		t.Fatalf("expected JobMemory.Kind = persistent, got %+v", updatedChar.JobMemory)
	}
	if updatedChar.JobMemory.JobID != "job-01" || updatedChar.JobMemory.SP != 50 ||
		updatedChar.JobMemory.OldJobID != "job-02" || updatedChar.JobMemory.OldSP != 40 {
		t.Fatalf("unexpected previous state in JobMemory: %+v", updatedChar.JobMemory)
	}

	// 2. Normal job change is rejected while persistent JobMemory is active (job_change.cgi:151)
	_, _, err = svc.ChangeJob(ctx, charID, "job-04")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ChangeJob to fail with ErrJobUnavailable while in persistent memory, got %v", err)
	}

	// 3. Overlapping start with item-243 is rejected
	curInv := invRepo.invs[charID]
	item243, _ := item.NewInstance("item-243", 1)
	_ = curInv.Add(item243)
	invRepo.invs[charID] = curInv
	_, _, err = svc.ExchangeJob(ctx, charID, "job-03", "job-04", "item-243")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected overlapping ExchangeJob with item-243 to be rejected, got %v", err)
	}

	// 4. Manual revert succeeds for persistent memory (job_change.cgi:369 job_reverse)
	restoredChar, restoredJob, err := svc.ExchangeJob(ctx, charID, "ignored", "ignored")
	if err != nil {
		t.Fatalf("manual revert of persistent JobMemory failed: %v", err)
	}
	if restoredChar.JobMemory != nil {
		t.Fatalf("expected JobMemory to be cleared after manual revert, got %+v", restoredChar.JobMemory)
	}
	if restoredChar.JobID != "job-01" || restoredChar.SP != 50 || restoredChar.OldJobID != "job-02" || restoredChar.OldSP != 40 {
		t.Fatalf("unexpected restored character state: %+v", restoredChar)
	}
	if restoredJob.CurrentJobID != "job-01" {
		t.Fatalf("expected character_jobs.current_job_id restored to job-01, got %s", restoredJob.CurrentJobID)
	}
}

func TestJobMemory_TemporaryLifecycle_Item243(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, jobRepo, invRepo := setupJobMemoryService(t)

	charID := "char-243"
	char := corecharacter.Character{
		ID:       charID,
		Name:     "Tester243",
		JobID:    "job-01",
		OldJobID: "job-02",
		SP:       50,
		OldSP:    40,
		Gender:   "m",
	}
	charRepo.chars[charID] = char

	jobState, _ := corejob.NewCharacterJob(charID, "job-01")
	jobState.RecordMastery("job-03", 80, 80)
	jobState.RecordMastery("job-04", 100, 100)
	jobRepo.jobs[charID] = jobState

	inv, _ := coreinventory.New(charID)
	item243, _ := item.NewInstance("item-243", 1)
	_ = inv.Add(item243)
	invRepo.invs[charID] = inv

	// 1. Start temporary memory using item-243
	updatedChar, updatedJob, err := svc.ExchangeJob(ctx, charID, "job-03", "job-04", "item-243")
	if err != nil {
		t.Fatalf("ExchangeJob with item-243 failed: %v", err)
	}

	// Verify item-243 was consumed once
	curInv243 := invRepo.invs[charID]
	if curInv243.Quantity("item-243") != 0 {
		t.Fatalf("expected item-243 quantity to be 0, got %d", curInv243.Quantity("item-243"))
	}

	// Verify updated character state and synchronized current job
	if updatedChar.JobID != "job-03" || updatedChar.SP != 80 || updatedChar.OldJobID != "job-04" || updatedChar.OldSP != 100 {
		t.Fatalf("unexpected updated character: %+v", updatedChar)
	}
	if updatedJob.CurrentJobID != "job-03" {
		t.Fatalf("expected character_jobs.current_job_id = job-03, got %s", updatedJob.CurrentJobID)
	}

	// Verify temporary JobMemory was saved
	if updatedChar.JobMemory == nil {
		t.Fatal("expected JobMemory to be set")
	}
	if updatedChar.JobMemory.Kind != corecharacter.JobMemoryKindTemporary || !updatedChar.JobMemory.IsTemporary() || updatedChar.JobMemory.IsPersistent() {
		t.Fatalf("expected JobMemory.Kind = temporary, got %+v", updatedChar.JobMemory)
	}
	if updatedChar.JobMemory.JobID != "job-01" || updatedChar.JobMemory.SP != 50 ||
		updatedChar.JobMemory.OldJobID != "job-02" || updatedChar.JobMemory.OldSP != 40 {
		t.Fatalf("unexpected previous state in JobMemory: %+v", updatedChar.JobMemory)
	}

	// 2. Normal job change is rejected while temporary JobMemory is active (job_change.cgi:151)
	_, _, err = svc.ChangeJob(ctx, charID, "job-04")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ChangeJob to fail with ErrJobUnavailable while in temporary memory, got %v", err)
	}

	// 3. Manual revert via ExchangeJob is rejected for temporary memory (job_change.cgi:121, 131)
	_, _, err = svc.ExchangeJob(ctx, charID, "ignored", "ignored")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected manual revert of temporary JobMemory to be rejected, got %v", err)
	}

	// 4. Overlapping start with item-168 is rejected
	curInv := invRepo.invs[charID]
	item168, _ := item.NewInstance("item-168", 1)
	_ = curInv.Add(item168)
	invRepo.invs[charID] = curInv
	_, _, err = svc.ExchangeJob(ctx, charID, "job-03", "job-04", "item-168")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected overlapping ExchangeJob with item-168 to be rejected, got %v", err)
	}

	// 5. Restore temporary memory via RestoreActiveJob & RevertJobMemory (simulating sleep reversal)
	curChar := charRepo.chars[charID]
	reverted := curChar.RevertJobMemory()
	if !reverted {
		t.Fatal("expected RevertJobMemory to return true")
	}
	err = svc.RestoreActiveJob(ctx, charID, curChar.JobID)
	if err != nil {
		t.Fatalf("RestoreActiveJob failed: %v", err)
	}
	_ = charRepo.Update(ctx, curChar)

	finalChar := charRepo.chars[charID]
	if finalChar.JobMemory != nil {
		t.Fatalf("expected JobMemory cleared after sleep revert, got %+v", finalChar.JobMemory)
	}
	if finalChar.JobID != "job-01" || finalChar.SP != 50 || finalChar.OldJobID != "job-02" || finalChar.OldSP != 40 {
		t.Fatalf("unexpected final character after sleep revert: %+v", finalChar)
	}
	finalJob := jobRepo.jobs[charID]
	if finalJob.CurrentJobID != "job-01" {
		t.Fatalf("expected character_jobs.current_job_id restored to job-01, got %s", finalJob.CurrentJobID)
	}
}

func TestJobMemory_FailureAtomicity(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, jobRepo, invRepo := setupJobMemoryService(t)

	charID := "char-atomic"
	char := corecharacter.Character{
		ID:       charID,
		Name:     "TesterAtomic",
		JobID:    "job-01",
		OldJobID: "job-02",
		SP:       50,
		OldSP:    40,
		Gender:   "m",
	}
	charRepo.chars[charID] = char

	jobState, _ := corejob.NewCharacterJob(charID, "job-01")
	jobState.RecordMastery("job-03", 80, 80)
	jobState.RecordMastery("job-04", 100, 100)
	jobRepo.jobs[charID] = jobState

	inv, _ := coreinventory.New(charID)
	invRepo.invs[charID] = inv

	// 1. Missing item-243 -> ErrRequiredItem
	_, _, err := svc.ExchangeJob(ctx, charID, "job-03", "job-04", "item-243")
	if !errors.Is(err, job.ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem for missing item-243, got %v", err)
	}

	// 2. Unmastered jobs -> ErrJobUnavailable even with item
	curInv := invRepo.invs[charID]
	item243, _ := item.NewInstance("item-243", 1)
	_ = curInv.Add(item243)
	invRepo.invs[charID] = curInv
	_, _, err = svc.ExchangeJob(ctx, charID, "job-05", "job-06", "item-243")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable for unmastered jobs, got %v", err)
	}
	curInvAfter := invRepo.invs[charID]
	if curInvAfter.Quantity("item-243") != 1 {
		t.Fatalf("item-243 should NOT be consumed on failure, quantity = %d", curInvAfter.Quantity("item-243"))
	}

	// 3. OverLevel -> ErrJobUnavailable
	char.OverLevel = true
	charRepo.chars[charID] = char
	_, _, err = svc.ExchangeJob(ctx, charID, "job-03", "job-04", "item-243")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable for OverLevel character, got %v", err)
	}
}
