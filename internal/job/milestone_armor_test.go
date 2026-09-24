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
)

// equipmentRepoStub implements EquipmentRepository for tests.
type equipmentRepoStub struct {
	equipment coreequipment.Equipment
}

func (e *equipmentRepoStub) FindByCharacterID(_ context.Context, _ string) (coreequipment.Equipment, error) {
	return e.equipment, nil
}

func (e *equipmentRepoStub) Save(_ context.Context, value coreequipment.Equipment) error {
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
