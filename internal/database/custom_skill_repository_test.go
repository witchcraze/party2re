package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/custom_skill"
	"github.com/witchcraze/party2re/internal/database"
)

func TestCustomSkillRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "Skill Repo Hero")
	if err != nil {
		t.Fatal(err)
	}

	repo, err := database.NewCustomSkillRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	loadout := custom_skill.CharacterSkillLoadout{
		CharacterID: char.ID,
		MaxSlots:    4,
		Slots: []custom_skill.EquippedSkillSlot{
			{SlotIndex: 1, SkillID: "slash", SkillName: "一閃", Priority: 5, MPCost: 6, Power: 35, Kind: "damage"},
			{SlotIndex: 2, SkillID: "heal", SkillName: "ヒール", Priority: 8, MPCost: 8, Power: 45, Kind: "healing"},
		},
		UpdatedAt: now,
	}

	// 1. SaveLoadout
	if err := repo.SaveLoadout(ctx, loadout); err != nil {
		t.Fatalf("SaveLoadout failed: %v", err)
	}

	// 2. FindLoadout
	fetched, err := repo.FindLoadout(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindLoadout failed: %v", err)
	}
	if fetched == nil || fetched.CharacterID != char.ID || len(fetched.Slots) != 2 {
		t.Fatalf("unexpected fetched loadout: %#v", fetched)
	}
	if fetched.Slots[0].SkillID != "slash" || fetched.Slots[1].SkillID != "heal" {
		t.Errorf("unexpected slots content: %#v", fetched.Slots)
	}

	// 3. Update Loadout
	loadout.Slots = []custom_skill.EquippedSkillSlot{
		{SlotIndex: 1, SkillID: "power_strike", SkillName: "渾身撃", Priority: 7, MPCost: 8, Power: 45, Kind: "damage"},
	}
	if err := repo.SaveLoadout(ctx, loadout); err != nil {
		t.Fatalf("Update SaveLoadout failed: %v", err)
	}

	// 4. Verify Updated Loadout
	updated, err := repo.FindLoadout(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindLoadout after update failed: %v", err)
	}
	if len(updated.Slots) != 1 || updated.Slots[0].SkillID != "power_strike" {
		t.Errorf("unexpected updated slots: %#v", updated.Slots)
	}
}
