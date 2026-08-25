package custom_skill_test

import (
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

// TestLoadCatalog_EmbeddedJSON verifies that the embedded JSON skill catalog
// is correctly parsed and contains the expected entries.
func TestLoadCatalog_EmbeddedJSON(t *testing.T) {
	repo := newMockRepo()
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{}}

	svc, err := custom_skill.NewService(repo, charRepo, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	catalog := svc.ListCatalog()
	if len(catalog) == 0 {
		t.Fatal("expected non-empty skill catalog, got 0 entries")
	}

	expectedIDs := []string{
		"gem_strike", "gem_cure", "gem_barrier",
		"slash", "power_strike", "fireball", "heal",
		"shadow_strike", "greater_heal", "dark_flame",
		"berserk_rush", "dragon_breath", "meteor",
	}
	found := make(map[string]bool)
	for _, s := range catalog {
		found[s.ID] = true
	}
	for _, id := range expectedIDs {
		if !found[id] {
			t.Errorf("expected skill %q not found in catalog", id)
		}
	}

	// Spot-check a specific skill value loaded from JSON.
	skill, err := svc.GetSkill("gem_strike")
	if err != nil {
		t.Fatalf("GetSkill(gem_strike): %v", err)
	}
	if skill.MPCost != 5 {
		t.Errorf("gem_strike: expected MPCost=5, got %d", skill.MPCost)
	}
	if skill.Power != 25 {
		t.Errorf("gem_strike: expected Power=25, got %d", skill.Power)
	}
	if skill.Kind != "damage" {
		t.Errorf("gem_strike: expected Kind=damage, got %q", skill.Kind)
	}
	if skill.RequiredJobID != "" {
		t.Errorf("gem_strike: expected RequiredJobID empty, got %q", skill.RequiredJobID)
	}

	// Verify list is sorted by ID.
	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].ID > catalog[i].ID {
			t.Errorf("catalog not sorted: %q > %q at index %d", catalog[i-1].ID, catalog[i].ID, i)
		}
	}
}
