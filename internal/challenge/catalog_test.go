package challenge_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// TestLoadTiers_EmbeddedJSON verifies that the embedded JSON catalog is
// loaded correctly and contains the expected tier definitions.
func TestLoadTiers_EmbeddedJSON(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: map[string]corecharacter.Character{}}

	svc, err := challenge.NewService(repo, charRepo, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	tiers := svc.ListTiers()
	if len(tiers) == 0 {
		t.Fatal("expected at least one tier, got 0")
	}

	expectedIDs := []string{"novice", "intermediate", "master", "abyss"}
	found := make(map[string]bool)
	for _, tier := range tiers {
		found[tier.ID] = true
	}
	for _, id := range expectedIDs {
		if !found[id] {
			t.Errorf("expected tier %q not found in catalog", id)
		}
	}

	// Spot-check a specific tier value loaded from JSON.
	for _, tier := range tiers {
		if tier.ID == "novice" {
			if tier.MinLevel != 5 {
				t.Errorf("novice tier: expected MinLevel=5, got %d", tier.MinLevel)
			}
			if tier.BaseMonster.BaseHP != 120 {
				t.Errorf("novice tier: expected BaseHP=120, got %d", tier.BaseMonster.BaseHP)
			}
			if tier.MilestoneInterval != 5 {
				t.Errorf("novice tier: expected MilestoneInterval=5, got %d", tier.MilestoneInterval)
			}
			if len(tier.MilestoneItemPool) != 2 {
				t.Errorf("novice tier: expected 2 milestone items, got %d", len(tier.MilestoneItemPool))
			}
		}
	}
}
