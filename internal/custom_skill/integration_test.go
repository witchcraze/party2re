package custom_skill_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/custom_skill"
	"github.com/witchcraze/party2re/internal/database"
)

func TestCustomSkillIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "Skill Integrator Hero")
	if err != nil {
		t.Fatal(err)
	}

	// Update character to Lv 20, Job: 剣士 (job-02)
	_, err = db.ExecContext(ctx, "UPDATE characters SET level = 20, job_id = 'job-02' WHERE id = ?", char.ID)
	if err != nil {
		t.Fatal(err)
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	jobRepo, err := database.NewCharacterJobRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	skillRepo, err := database.NewCustomSkillRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := custom_skill.NewService(skillRepo, charRepo, jobRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial Loadout: empty
	loadout, err := service.GetLoadout(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetLoadout failed: %v", err)
	}
	if len(loadout.Slots) != 0 || loadout.MaxSlots != 4 {
		t.Errorf("unexpected initial loadout: %#v", loadout)
	}

	// 2. Equip Slash in Slot 1
	loadout, err = service.EquipSkill(ctx, char.ID, 1, "slash", 8)
	if err != nil {
		t.Fatalf("EquipSkill slash failed: %v", err)
	}
	if len(loadout.Slots) != 1 || loadout.Slots[0].SkillID != "slash" {
		t.Errorf("unexpected loadout: %#v", loadout)
	}

	// 3. Equip Gem Strike in Slot 2
	loadout, err = service.EquipSkill(ctx, char.ID, 2, "gem_strike", 5)
	if err != nil {
		t.Fatalf("EquipSkill gem_strike failed: %v", err)
	}
	if len(loadout.Slots) != 2 {
		t.Errorf("expected 2 slots, got %d", len(loadout.Slots))
	}

	// 4. Unequip Slot 1
	loadout, err = service.UnequipSlot(ctx, char.ID, 1)
	if err != nil {
		t.Fatalf("UnequipSlot failed: %v", err)
	}
	if len(loadout.Slots) != 1 || loadout.Slots[0].SkillID != "gem_strike" {
		t.Errorf("unexpected loadout after unequip: %#v", loadout)
	}
}
