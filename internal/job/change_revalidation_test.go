package job

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

func TestChangeJob_StalePreflight_RevalidatesCharacterAndItemsUnderLock(t *testing.T) {
	ctx := context.Background()

	t.Run("job08_to_job46_loses_casino_wins_returns_ErrJobUnavailable", func(t *testing.T) {
		preflight := corecharacter.Character{
			ID: "char-stale-cas", JobID: "job-08", OldJobID: "job-01",
			Level: 50, Gender: "male", CasinoWins: 10, SP: 30, OldSP: 15,
		}
		locked := preflight
		locked.CasinoWins = 9

		charRepo := &stalePreflightCharRepoStub{preflightChar: preflight, lockedChar: locked}
		inv, _ := coreinventory.New(preflight.ID)
		dice, _ := item.NewInstance("item-039", 1)
		_ = inv.Add(dice)
		invRepo := &errInventoryRepoStub{inv: inv}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: preflight.ID, CurrentJobID: preflight.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		var memories int
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
			WithMemoryRecorder(func(_ context.Context, _ string, _ corecharacter.MemoryKind, _ string) error {
				memories++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, preflight.ID, "job-46")
		if !errors.Is(err, corejob.ErrJobUnavailable) {
			t.Fatalf("expected ErrJobUnavailable, got %v", err)
		}
		if charRepo.updateCount != 0 || jobRepo.saveCount != 0 || memories != 0 {
			t.Fatalf("expected zero mutations on rejection, got updates=%d saves=%d memories=%d",
				charRepo.updateCount, jobRepo.saveCount, memories)
		}
		if invRepo.inv.Quantity("item-039") != 1 {
			t.Fatalf("expected item-039 preserved, got %d", invRepo.inv.Quantity("item-039"))
		}
	})

	t.Run("job08_to_job46_changed_to_job01_consumes_item039", func(t *testing.T) {
		preflight := corecharacter.Character{
			ID: "char-stale-to01", JobID: "job-08", OldJobID: "job-01",
			Level: 50, Gender: "male", CasinoWins: 10, SP: 30, OldSP: 15,
		}
		locked := preflight
		locked.JobID = "job-01"
		locked.OldJobID = "job-02"
		locked.SP = 45
		locked.OldSP = 25

		charRepo := &stalePreflightCharRepoStub{preflightChar: preflight, lockedChar: locked}
		inv, _ := coreinventory.New(preflight.ID)
		dice, _ := item.NewInstance("item-039", 1)
		_ = inv.Add(dice)
		invRepo := &errInventoryRepoStub{inv: inv}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: preflight.ID, CurrentJobID: locked.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		updated, _, err := svc.ChangeJob(ctx, preflight.ID, "job-46")
		if err != nil {
			t.Fatalf("expected success when item-039 is present, got %v", err)
		}
		if updated.JobID != "job-46" || updated.OldJobID != "job-01" || updated.OldSP != 45 {
			t.Fatalf("unexpected updated character: %+v", updated)
		}
		if invRepo.inv.Quantity("item-039") != 0 {
			t.Fatalf("expected item-039 consumed when locked job is job-01, got %d", invRepo.inv.Quantity("item-039"))
		}
	})

	t.Run("job01_to_job46_changed_to_job08_preserves_item039", func(t *testing.T) {
		preflight := corecharacter.Character{
			ID: "char-stale-to08", JobID: "job-01", OldJobID: "job-02",
			Level: 50, Gender: "male", CasinoWins: 10, SP: 45, OldSP: 25,
		}
		locked := preflight
		locked.JobID = "job-08"
		locked.OldJobID = "job-01"

		charRepo := &stalePreflightCharRepoStub{preflightChar: preflight, lockedChar: locked}
		inv, _ := coreinventory.New(preflight.ID)
		dice, _ := item.NewInstance("item-039", 1)
		_ = inv.Add(dice)
		invRepo := &errInventoryRepoStub{inv: inv}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: preflight.ID, CurrentJobID: locked.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		updated, _, err := svc.ChangeJob(ctx, preflight.ID, "job-46")
		if err != nil {
			t.Fatalf("expected success when locked job is job-08 with item-039, got %v", err)
		}
		if updated.JobID != "job-46" || updated.OldJobID != "job-08" {
			t.Fatalf("unexpected updated character: %+v", updated)
		}
		if invRepo.inv.Quantity("item-039") != 1 {
			t.Fatalf("expected item-039 preserved when locked job is job-08, got %d", invRepo.inv.Quantity("item-039"))
		}
	})

	t.Run("job01_to_job46_changed_to_old_job46_succeeds_without_item", func(t *testing.T) {
		preflight := corecharacter.Character{
			ID: "char-stale-old46", JobID: "job-01", OldJobID: "job-02",
			Level: 50, Gender: "male", CasinoWins: 0, SP: 45, OldSP: 25,
		}
		locked := preflight
		locked.OldJobID = "job-46"
		locked.OldSP = 77

		charRepo := &stalePreflightCharRepoStub{preflightChar: preflight, lockedChar: locked}
		inv, _ := coreinventory.New(preflight.ID)
		invRepo := &errInventoryRepoStub{inv: inv}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: preflight.ID, CurrentJobID: locked.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		updated, _, err := svc.ChangeJob(ctx, preflight.ID, "job-46")
		if err != nil {
			t.Fatalf("expected bypass when locked OldJobID is job-46, got %v", err)
		}
		if updated.JobID != "job-46" || updated.SP != 77 {
			t.Fatalf("expected job-46 with restored SP 77, got job=%s SP=%d", updated.JobID, updated.SP)
		}
	})

	t.Run("job01_to_job84_loses_prerequisite_tree_does_not_consume_armor", func(t *testing.T) {
		preflight := corecharacter.Character{
			ID: "char-stale-ff", JobID: "job-01", OldJobID: "job-04",
			Level: 50, Gender: "male", SP: 30, OldSP: 10,
		}
		locked := preflight
		locked.JobID = "job-02"
		locked.OldJobID = "job-03"

		charRepo := &stalePreflightCharRepoStub{preflightChar: preflight, lockedChar: locked}
		inv, _ := coreinventory.New(preflight.ID)
		armor, _ := item.NewInstance("armor-29", 1)
		_ = inv.Add(armor)
		invRepo := &errInventoryRepoStub{inv: inv}
		equip, _ := coreequipment.New(preflight.ID)
		equip.Slots[item.SlotBody] = armor.ID
		equipRepo := &equipmentRepoStub{equipment: equip}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: preflight.ID, CurrentJobID: locked.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEquipmentRepository(equipRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, preflight.ID, "job-84")
		if !errors.Is(err, corejob.ErrJobUnavailable) {
			t.Fatalf("expected ErrJobUnavailable, got %v", err)
		}
		if invRepo.inv.Quantity("armor-29") != 1 || equipRepo.equipment.Slots[item.SlotBody] != armor.ID {
			t.Fatalf("expected armor-29 untouched on rejection")
		}
		if charRepo.updateCount != 0 || jobRepo.saveCount != 0 {
			t.Fatalf("expected zero saves on rejection, got updates=%d saves=%d", charRepo.updateCount, jobRepo.saveCount)
		}
	})

	t.Run("job73_requires_all_jobs_mastered_in_locked_state", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-stale-73", JobID: "job-01", OldJobID: "job-02",
			Level: 50, Gender: "male",
		}
		charRepo := &stalePreflightCharRepoStub{preflightChar: char, lockedChar: char}
		inv, _ := coreinventory.New(char.ID)
		invRepo := &errInventoryRepoStub{inv: inv}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, char.ID, "job-73")
		if !errors.Is(err, corejob.ErrJobUnavailable) {
			t.Fatalf("expected ErrJobUnavailable for job-73 without mastery, got %v", err)
		}
		if charRepo.updateCount != 0 || jobRepo.saveCount != 0 {
			t.Fatalf("expected zero saves, got updates=%d saves=%d", charRepo.updateCount, jobRepo.saveCount)
		}
	})
}

func TestChangeJob_ReadErrorsPropagateWithoutPartialSaves(t *testing.T) {
	ctx := context.Background()
	readErr := errors.New("simulated storage read failure")

	t.Run("inventory_read_error_propagates_on_consuming_change", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-read-inv1", JobID: "job-01", OldJobID: "job-02",
			Level: 50, Gender: "male", CasinoWins: 10,
		}
		charRepo := &stalePreflightCharRepoStub{preflightChar: char, lockedChar: char}
		invRepo := &errInventoryRepoStub{findErr: readErr}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		var memories int
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
			WithMemoryRecorder(func(_ context.Context, _ string, _ corecharacter.MemoryKind, _ string) error {
				memories++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
		if !errors.Is(err, readErr) {
			t.Fatalf("expected propagated readErr, got %v", err)
		}
		if errors.Is(err, ErrRequiredItem) {
			t.Fatal("read error must not be converted to ErrRequiredItem")
		}
		if charRepo.updateCount != 0 || jobRepo.saveCount != 0 || memories != 0 {
			t.Fatalf("expected zero saves, got updates=%d saves=%d memories=%d", charRepo.updateCount, jobRepo.saveCount, memories)
		}
	})

	t.Run("inventory_read_error_propagates_on_exempt_possession_check", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-read-inv2", JobID: "job-08", OldJobID: "job-01",
			Level: 50, Gender: "male", CasinoWins: 10,
		}
		charRepo := &stalePreflightCharRepoStub{preflightChar: char, lockedChar: char}
		invRepo := &errInventoryRepoStub{findErr: readErr}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
		if !errors.Is(err, readErr) {
			t.Fatalf("expected propagated readErr, got %v", err)
		}
		if errors.Is(err, ErrRequiredItem) {
			t.Fatal("read error must not be converted to ErrRequiredItem")
		}
	})

	t.Run("equipment_read_error_propagates_without_mutating_inventory_or_job", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-read-eq", JobID: "job-01", OldJobID: "job-04",
			Level: 50, Gender: "male",
		}
		charRepo := &stalePreflightCharRepoStub{preflightChar: char, lockedChar: char}
		inv, _ := coreinventory.New(char.ID)
		armor, _ := item.NewInstance("armor-29", 1)
		_ = inv.Add(armor)
		invRepo := &errInventoryRepoStub{inv: inv}
		equipRepo := &equipmentRepoStub{findErr: readErr}
		jobRepo := &trackingJobRepoStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID}}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		var memories int
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEquipmentRepository(equipRepo),
			WithEconomy(ecoSvc),
			WithMemoryRecorder(func(_ context.Context, _ string, _ corecharacter.MemoryKind, _ string) error {
				memories++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, char.ID, "job-84")
		if !errors.Is(err, readErr) {
			t.Fatalf("expected propagated readErr, got %v", err)
		}
		if errors.Is(err, ErrRequiredArmor) {
			t.Fatal("read error must not be converted to ErrRequiredArmor")
		}
		if invRepo.inv.Quantity("armor-29") != 1 || charRepo.updateCount != 0 || jobRepo.saveCount != 0 || memories != 0 {
			t.Fatalf("expected zero mutations on equipment read failure")
		}
	})

	t.Run("job_read_error_propagates_without_consuming_armor", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-read-job", JobID: "job-01", OldJobID: "job-04",
			Level: 50, Gender: "male",
		}
		charRepo := &stalePreflightCharRepoStub{preflightChar: char, lockedChar: char}
		inv, _ := coreinventory.New(char.ID)
		armor, _ := item.NewInstance("armor-29", 1)
		_ = inv.Add(armor)
		invRepo := &errInventoryRepoStub{inv: inv}
		equip, _ := coreequipment.New(char.ID)
		equip.Slots[item.SlotBody] = armor.ID
		equipRepo := &equipmentRepoStub{equipment: equip}
		jobRepo := &errRepoStub{findErr: readErr}
		ecoSvc, err := economy.NewService(charRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}
		svc, err := NewService(
			jobRepo,
			WithCharacterRepository(charRepo),
			WithInventoryRepository(invRepo),
			WithEquipmentRepository(equipRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, char.ID, "job-84")
		if !errors.Is(err, readErr) {
			t.Fatalf("expected propagated readErr, got %v", err)
		}
		if invRepo.inv.Quantity("armor-29") != 1 || equipRepo.equipment.Slots[item.SlotBody] != armor.ID {
			t.Fatalf("expected armor-29 unconsumed and still equipped on job read failure")
		}
		if charRepo.updateCount != 0 {
			t.Fatalf("expected zero character updates, got %d", charRepo.updateCount)
		}
	})
}
