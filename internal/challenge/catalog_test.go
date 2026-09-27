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
	if len(tiers) != 9 {
		t.Fatalf("expected 9 authentic stages, got %d", len(tiers))
	}

	expectedIDs := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8"}
	found := make(map[string]bool)
	for _, tier := range tiers {
		found[tier.ID] = true
	}
	for _, id := range expectedIDs {
		if !found[id] {
			t.Errorf("expected stage %q not found in catalog", id)
		}
	}

	// Spot-check stage 0 loaded from JSON.
	for _, tier := range tiers {
		if tier.ID == "0" {
			if tier.MaxParticipants != 1 {
				t.Errorf("stage 0: expected MaxParticipants=1, got %d", tier.MaxParticipants)
			}
			if tier.NeedJoin != "hp_400_u" {
				t.Errorf("stage 0: expected NeedJoin=hp_400_u, got %q", tier.NeedJoin)
			}
			if tier.BaseMonster.BaseHP != 150 {
				t.Errorf("stage 0: expected BaseHP=150, got %d", tier.BaseMonster.BaseHP)
			}
			if tier.TreasureRound != 10 {
				t.Errorf("stage 0: expected TreasureRound=10, got %d", tier.TreasureRound)
			}
			if len(tier.TreasureItemPool) != 2 {
				t.Errorf("stage 0: expected 2 treasure items, got %d", len(tier.TreasureItemPool))
			}
		}
	}
}
