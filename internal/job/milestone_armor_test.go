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

// equipmentRepoStub implements EquipmentRepository for tests.
type equipmentRepoStub struct {
	saveErr   error
	equipment coreequipment.Equipment
}

func (e *equipmentRepoStub) FindByCharacterID(_ context.Context, _ string) (coreequipment.Equipment, error) {
	return e.equipment, nil
}

func (e *equipmentRepoStub) Save(_ context.Context, value coreequipment.Equipment) error {
	if e.saveErr != nil {
		return e.saveErr
	}
	e.equipment = value
	return nil
}

// TestChangeJob_MilestoneGating verifies that monster kill / PvP win / casino win
// counters gate the special job prerequisites per legacy _data.cgi conditions.
func TestChangeJob_MilestoneGating(t *testing.T) {
	ctx := context.Background()

	makeChar := func(jobID string, monsterKills, pvpWins int) corecharacter.Character {
		return corecharacter.Character{
			ID:           "char-ms",
			JobID:        jobID,
			OldJobID:     "job-04",
			Level:        50,
			Gender:       "unspecified",
			MonsterKills: monsterKills,
			PvPWins:      pvpWins,
			OverLevel:    true,
		}
	}
	makeSvc := func(char corecharacter.Character, state corejob.CharacterJob) *Service {
		repo := &repositoryStub{value: state}
		charRepo := &charRepoStub{char: char}
		svc, _ := NewService(repo, WithCharacterRepository(charRepo))
		return svc
	}

	// job-21 (バーサーカー): requires MonsterKills > 200 && isNeedJob(1,4,12,21)
	// char.OldJobID = "job-04" satisfies isNeedJob(4)
	t.Run("Berserker_below_kill_m", func(t *testing.T) {
		char := makeChar("job-01", 200, 0) // exactly 200, needs > 200
		state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
		svc := makeSvc(char, state)
		_, _, err := svc.ChangeJob(ctx, char.ID, "job-21")
		if !errors.Is(err, corejob.ErrJobUnavailable) {
			t.Fatalf("expected ErrJobUnavailable with MonsterKills=200, got %v", err)
		}
	})
	t.Run("Berserker_meets_kill_m", func(t *testing.T) {
		char := makeChar("job-01", 201, 0)
		state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
		svc := makeSvc(char, state)
		_, _, err := svc.ChangeJob(ctx, char.ID, "job-21")
		if err != nil {
			t.Fatalf("expected success with MonsterKills=201, got %v", err)
		}
	})

	// job-22 (暗黒騎士): requires PvPWins > 50 && isNeedJob(2,3,17,20,22,52)
	// char needs current/old in [2,3,17,20,22,52]. Use "job-02" as OldJobID.
	t.Run("DarkKnight_below_kill_p", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-ms", JobID: "job-01", OldJobID: "job-02",
			Level: 50, Gender: "unspecified", PvPWins: 50, OverLevel: true,
		}
		state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
		svc := makeSvc(char, state)
		_, _, err := svc.ChangeJob(ctx, char.ID, "job-22")
		if !errors.Is(err, corejob.ErrJobUnavailable) {
			t.Fatalf("expected ErrJobUnavailable with PvPWins=50, got %v", err)
		}
	})
	t.Run("DarkKnight_meets_kill_p", func(t *testing.T) {
		char := corecharacter.Character{
			ID: "char-ms", JobID: "job-01", OldJobID: "job-02",
			Level: 50, Gender: "unspecified", PvPWins: 51, OverLevel: true,
		}
		state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
		svc := makeSvc(char, state)
		_, _, err := svc.ChangeJob(ctx, char.ID, "job-22")
		if err != nil {
			t.Fatalf("expected success with PvPWins=51, got %v", err)
		}
	})
}

// TestChangeJob_FireFighterArmorConsumption verifies that job-84 (炎闘士) requires
// armor-29 (炎の鎧) to be equipped in the body slot and consumes it on change.
func TestChangeJob_FireFighterArmorConsumption(t *testing.T) {
	ctx := context.Background()

	// char.JobID must be in [job-01, job-04, job-25, job-30] for the fresh-change path.
	char := corecharacter.Character{
		ID:        "char-ff",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "unspecified",
		OverLevel: true,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}

	// Prepare inventory with armor-29 instance.
	inv, _ := coreinventory.New(char.ID)
	armorInstance, _ := item.NewInstance("armor-29", 1)
	_ = inv.Add(armorInstance)
	invRepo := &inventoryRepoStub{inventory: inv}

	// Prepare equipment with armor-29 equipped in body slot.
	equip, _ := coreequipment.New(char.ID)
	_ = equip.Slots // initialize
	equip.Slots[item.SlotBody] = armorInstance.ID
	equipRepo := &equipmentRepoStub{equipment: equip}

	svc, err := NewService(
		repo,
		WithCharacterRepository(charRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Change to job-84 with armor-29 equipped -> success, armor consumed
	updatedChar, _, err := svc.ChangeJob(ctx, char.ID, "job-84")
	if err != nil {
		t.Fatalf("expected job-84 change to succeed with armor-29 equipped, got %v", err)
	}
	if updatedChar.JobID != "job-84" {
		t.Fatalf("expected job-84, got %s", updatedChar.JobID)
	}
	if invRepo.inventory.Quantity("armor-29") != 0 {
		t.Fatalf("expected armor-29 consumed, remaining %d", invRepo.inventory.Quantity("armor-29"))
	}
	if _, ok := equipRepo.equipment.Equipped(item.SlotBody); ok {
		t.Fatal("expected body slot to be empty after armor consumption")
	}
}

// TestChangeJob_FireFighterWithoutArmor verifies ErrRequiredArmor is returned
// when trying to change to job-84 (炎闘士) without armor-29 equipped.
func TestChangeJob_FireFighterWithoutArmor(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-ff",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "unspecified",
		OverLevel: true,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}

	// Inventory has no armor-29.
	inv, _ := coreinventory.New(char.ID)
	invRepo := &inventoryRepoStub{inventory: inv}

	// Equipment has nothing equipped.
	equip, _ := coreequipment.New(char.ID)
	equipRepo := &equipmentRepoStub{equipment: equip}

	svc, _ := NewService(
		repo,
		WithCharacterRepository(charRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
	)

	_, _, err := svc.ChangeJob(ctx, char.ID, "job-84")
	if !errors.Is(err, ErrRequiredArmor) {
		t.Fatalf("expected ErrRequiredArmor without armor-29 equipped, got %v", err)
	}
}

// TestChangeJob_FireFighter_MissingArmor_DoesNotMutateJob verifies character job
// remains unchanged in DB when armor-29 is missing in economy transaction mode.
func TestChangeJob_FireFighter_MissingArmor_DoesNotMutateJob(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-ff-missing",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "unspecified",
		OverLevel: true,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	ecoCharRepo := &errCharRepoStub{char: char}
	inv, _ := coreinventory.New(char.ID)
	invRepo := &errInventoryRepoStub{inv: inv}
	ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	equip, _ := coreequipment.New(char.ID)
	equipRepo := &equipmentRepoStub{equipment: equip}

	svc, err := NewService(
		repo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
		WithEconomy(ecoSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.ChangeJob(ctx, char.ID, "job-84")
	if !errors.Is(err, ErrRequiredArmor) {
		t.Fatalf("expected ErrRequiredArmor, got %v", err)
	}

	// Verify character in repository is completely untouched
	savedChar, err := ecoCharRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedChar.JobID != "job-01" {
		t.Fatalf("character JobID was mutated to %s, expected job-01", savedChar.JobID)
	}
	if repo.value.CurrentJobID != "job-01" {
		t.Fatalf("job state was mutated to %s, expected job-01", repo.value.CurrentJobID)
	}
}

// TestChangeJob_FireFighter_WrongArmor_DoesNotMutateJob verifies character job
// remains unchanged in DB when wearing armor-01 instead of armor-29.
func TestChangeJob_FireFighter_WrongArmor_DoesNotMutateJob(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-ff-wrong",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "unspecified",
		OverLevel: true,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	ecoCharRepo := &errCharRepoStub{char: char}

	inv, _ := coreinventory.New(char.ID)
	wrongArmor, _ := item.NewInstance("armor-01", 1)
	_ = inv.Add(wrongArmor)
	invRepo := &errInventoryRepoStub{inv: inv}

	equip, _ := coreequipment.New(char.ID)
	equip.Slots[item.SlotBody] = wrongArmor.ID
	equipRepo := &equipmentRepoStub{equipment: equip}

	ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(
		repo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
		WithEconomy(ecoSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.ChangeJob(ctx, char.ID, "job-84")
	if !errors.Is(err, ErrRequiredArmor) {
		t.Fatalf("expected ErrRequiredArmor when wearing armor-01, got %v", err)
	}

	// Verify character in repository is completely unchanged
	savedChar, err := ecoCharRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedChar.JobID != "job-01" {
		t.Fatalf("character JobID was mutated to %s, expected job-01", savedChar.JobID)
	}
	if repo.value.CurrentJobID != "job-01" {
		t.Fatalf("job state was mutated to %s, expected job-01", repo.value.CurrentJobID)
	}

	// Verify armor-01 remains equipped and unconsumed
	if equipRepo.equipment.Slots[item.SlotBody] != wrongArmor.ID {
		t.Fatal("expected armor-01 to remain equipped in body slot")
	}
	if invRepo.inv.Quantity("armor-01") != 1 {
		t.Fatal("expected armor-01 to remain in inventory")
	}
}

// TestChangeJob_FireFighter_ArmorSaveFailure_RollsBack verifies atomic rollback
// if armor consumption fails during the transaction.
func TestChangeJob_FireFighter_ArmorSaveFailure_RollsBack(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-ff-rollback",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "unspecified",
		OverLevel: true,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	ecoCharRepo := &errCharRepoStub{char: char}

	inv, _ := coreinventory.New(char.ID)
	fireArmor, _ := item.NewInstance("armor-29", 1)
	_ = inv.Add(fireArmor)
	invRepo := &errInventoryRepoStub{inv: inv}

	equip, _ := coreequipment.New(char.ID)
	equip.Slots[item.SlotBody] = fireArmor.ID
	equipRepo := &equipmentRepoStub{
		equipment: equip,
		saveErr:   errors.New("simulated equipment save error"),
	}

	ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(
		repo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
		WithEconomy(ecoSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.ChangeJob(ctx, char.ID, "job-84")
	if err == nil {
		t.Fatal("expected error due to simulated save failure")
	}

	// Character must NOT have mutated to job-84
	savedChar, err := ecoCharRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedChar.JobID != "job-01" {
		t.Fatalf("expected JobID to remain job-01 after rollback, got %s", savedChar.JobID)
	}
}

// TestChangeJob_Gambler_FromPlayboy_RequiresItemOwnership verifies that changing to
// job-46 from job-08 requires possessing item-039 in inventory even though consumption is exempt.
func TestChangeJob_Gambler_FromPlayboy_RequiresItemOwnership(t *testing.T) {
	ctx := context.Background()

	runTest := func(t *testing.T, currentJob, oldJob string) {
		char := corecharacter.Character{
			ID:         "char-playboy",
			JobID:      currentJob,
			OldJobID:   oldJob,
			Level:      50,
			Gender:     "unspecified",
			CasinoWins: 10,
			OverLevel:  true,
		}
		state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
		repo := &repositoryStub{value: state}
		ecoCharRepo := &errCharRepoStub{char: char}

		// 1. Without item-039 in inventory -> returns ErrRequiredItem
		invEmpty, _ := coreinventory.New(char.ID)
		invRepo := &errInventoryRepoStub{inv: invEmpty}

		ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
		if err != nil {
			t.Fatal(err)
		}

		svc, err := NewService(
			repo,
			WithCharacterRepository(ecoCharRepo),
			WithInventoryRepository(invRepo),
			WithEconomy(ecoSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
		if !errors.Is(err, ErrRequiredItem) {
			t.Fatalf("expected ErrRequiredItem when lacking item-039, got %v", err)
		}
		if ecoCharRepo.char.JobID != currentJob {
			t.Fatalf("expected JobID to remain %s, got %s", currentJob, ecoCharRepo.char.JobID)
		}

		// 2. With item-039 in inventory -> succeeds, and item-039 is NOT consumed (exempt)
		diceItem, _ := item.NewInstance("item-039", 1)
		_ = invRepo.inv.Add(diceItem)

		updatedChar, _, err := svc.ChangeJob(ctx, char.ID, "job-46")
		if err != nil {
			t.Fatalf("expected success when owning item-039: %v", err)
		}
		if updatedChar.JobID != "job-46" {
			t.Fatalf("expected job-46, got %s", updatedChar.JobID)
		}
		if invRepo.inv.Quantity("item-039") != 1 {
			t.Fatalf("expected item-039 preserved (exempt from consumption), remaining: %d", invRepo.inv.Quantity("item-039"))
		}
	}

	t.Run("current_job_is_playboy", func(t *testing.T) {
		runTest(t, "job-08", "job-01")
	})

	t.Run("old_job_is_playboy", func(t *testing.T) {
		runTest(t, "job-01", "job-08")
	})
}
