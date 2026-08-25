package custom_skill_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

type mockRepo struct {
	loadouts map[string]custom_skill.CharacterSkillLoadout
}

func newMockRepo() *mockRepo {
	return &mockRepo{loadouts: make(map[string]custom_skill.CharacterSkillLoadout)}
}

func (m *mockRepo) SaveLoadout(ctx context.Context, loadout custom_skill.CharacterSkillLoadout) error {
	m.loadouts[loadout.CharacterID] = loadout
	return nil
}

func (m *mockRepo) FindLoadout(ctx context.Context, characterID string) (*custom_skill.CharacterSkillLoadout, error) {
	l, ok := m.loadouts[characterID]
	if !ok {
		return nil, nil
	}
	return &l, nil
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, custom_skill.ErrCharacterNotFound
	}
	return c, nil
}

type mockJobRepo struct {
	jobs map[string]corejob.CharacterJob
}

func (m *mockJobRepo) FindByCharacterID(ctx context.Context, characterID string) (corejob.CharacterJob, error) {
	j, ok := m.jobs[characterID]
	if !ok {
		return corejob.CharacterJob{}, nil
	}
	return j, nil
}

func TestCustomSkillService(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"char-1": {
				ID:    "char-1",
				Name:  "Hero",
				Level: 10,
				JobID: "job-02", // 剣士
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 50, MaxMP: 50},
			},
		},
	}
	jobRepo := &mockJobRepo{
		jobs: map[string]corejob.CharacterJob{
			"char-1": {
				CharacterID:  "char-1",
				CurrentJobID: "job-02",
				MasteredJobs: []string{"job-01"}, // 戦士マスター
			},
		},
	}

	service, err := custom_skill.NewService(repo, charRepo, jobRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Check Catalog
	catalog := service.ListCatalog()
	if len(catalog) == 0 {
		t.Fatal("expected non-empty catalog")
	}

	// 2. Get Available Skills for char-1
	// Should have generic skills, job-02 skills (slash), and job-01 skills (power_strike)
	available, err := service.GetAvailableSkills(ctx, "char-1")
	if err != nil {
		t.Fatalf("GetAvailableSkills failed: %v", err)
	}
	foundSlash := false
	foundPowerStrike := false
	foundMeteor := false
	for _, s := range available {
		if s.ID == "slash" {
			foundSlash = true
		}
		if s.ID == "power_strike" {
			foundPowerStrike = true
		}
		if s.ID == "meteor" {
			foundMeteor = true
		}
	}
	if !foundSlash || !foundPowerStrike {
		t.Errorf("expected slash and power_strike in available skills")
	}
	if foundMeteor {
		t.Errorf("meteor should not be available to Lv 10 剣士 without 賢者 mastery")
	}

	// 3. Equip Skill: Success
	loadout, err := service.EquipSkill(ctx, "char-1", 1, "slash", 5)
	if err != nil {
		t.Fatalf("EquipSkill slash failed: %v", err)
	}
	if len(loadout.Slots) != 1 || loadout.Slots[0].SkillID != "slash" || loadout.Slots[0].Priority != 5 {
		t.Errorf("unexpected loadout: %#v", loadout)
	}

	// 4. Equip Second Skill from Mastered Job: Success
	loadout, err = service.EquipSkill(ctx, "char-1", 2, "power_strike", 8)
	if err != nil {
		t.Fatalf("EquipSkill power_strike failed: %v", err)
	}
	if len(loadout.Slots) != 2 {
		t.Errorf("expected 2 equipped skills, got %d", len(loadout.Slots))
	}

	// 5. Error: Duplicate Skill Equip
	_, err = service.EquipSkill(ctx, "char-1", 3, "slash", 3)
	if err != custom_skill.ErrDuplicateSkillEquip {
		t.Errorf("expected ErrDuplicateSkillEquip, got %v", err)
	}

	// 6. Error: Job Not Learned
	_, err = service.EquipSkill(ctx, "char-1", 3, "fireball", 5)
	if err != custom_skill.ErrSkillNotLearned {
		t.Errorf("expected ErrSkillNotLearned, got %v", err)
	}

	// 7. Error: Slot Out of Bounds
	_, err = service.EquipSkill(ctx, "char-1", 5, "gem_strike", 5)
	if err != custom_skill.ErrSlotOutOfBounds {
		t.Errorf("expected ErrSlotOutOfBounds, got %v", err)
	}

	// 8. Unequip Slot
	loadout, err = service.UnequipSlot(ctx, "char-1", 1)
	if err != nil {
		t.Fatalf("UnequipSlot failed: %v", err)
	}
	if len(loadout.Slots) != 1 || loadout.Slots[0].SkillID != "power_strike" {
		t.Errorf("unexpected loadout after unequip: %#v", loadout)
	}
}
