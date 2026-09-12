package adventure_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
)

func TestCalculateTreasureCount_BaseCases(t *testing.T) {
	tests := []struct {
		name         string
		aliveMembers int
		want         int
	}{
		{name: "no alive members", aliveMembers: 0, want: 0},
		{name: "negative alive members clamped", aliveMembers: -2, want: 0},
		{name: "1 alive member (solo)", aliveMembers: 1, want: 1},
		{name: "2 alive members", aliveMembers: 2, want: 2},
		{name: "3 alive members", aliveMembers: 3, want: 3},
		{name: "4 alive members (full party)", aliveMembers: 4, want: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
				AliveMembers: tc.aliveMembers,
			})
			if got != tc.want {
				t.Errorf("CalculateTreasureCount(%d) = %d, want %d", tc.aliveMembers, got, tc.want)
			}
		})
	}
}

func TestCalculateTreasureCount_MerchantBonus(t *testing.T) {
	// Merchant always adds exactly +1 box
	got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers: 4,
		HasMerchant:  true,
	})
	if got != 5 {
		t.Errorf("got %d boxes with merchant, want 5", got)
	}
}

func TestCalculateTreasureCount_TreasureHunterBonus(t *testing.T) {
	// rng returns 0: bonus is 1 + 0 = 1
	gotMin := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:      4,
		HasTreasureHunter: true,
		Rng:               func(n int) int { return 0 },
	})
	if gotMin != 5 {
		t.Errorf("gotMin = %d, want 5 (4 + 1)", gotMin)
	}

	// rng returns 1: bonus is 1 + 1 = 2
	gotMax := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:      4,
		HasTreasureHunter: true,
		Rng:               func(n int) int { return 1 },
	})
	if gotMax != 6 {
		t.Errorf("gotMax = %d, want 6 (4 + 2)", gotMax)
	}
}

func TestCalculateTreasureCount_LuckyPendantBonus(t *testing.T) {
	// rng(3) == 0: triggers +1 box
	gotTriggered := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:    4,
		HasLuckyPendant: true,
		Rng:             func(n int) int { return 0 },
	})
	if gotTriggered != 5 {
		t.Errorf("gotTriggered = %d, want 5 (4 + 1)", gotTriggered)
	}

	// rng(3) == 1: does not trigger
	gotNotTriggered := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:    4,
		HasLuckyPendant: true,
		Rng:             func(n int) int { return 1 },
	})
	if gotNotTriggered != 4 {
		t.Errorf("gotNotTriggered = %d, want 4", gotNotTriggered)
	}
}

func TestCalculateTreasureCount_CombinedBonuses(t *testing.T) {
	// Full 4-player party with Merchant (+1), Treasure Hunter (+2 with rng=1), Lucky Pendant (+1 with rng=0)
	// Base (4) + Merchant (1) + TH (2) + Pendant (1) = 8
	deterministicRng := func(n int) int {
		if n == 2 {
			return 1 // max TH bonus: 1 + 1 = 2
		}
		return 0 // triggers pendant: 0 < 1
	}

	got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:      4,
		HasMerchant:       true,
		HasTreasureHunter: true,
		HasLuckyPendant:   true,
		Rng:               deterministicRng,
	})
	if got != 8 {
		t.Errorf("CalculateTreasureCount with all bonuses = %d, want 8", got)
	}
}

func TestGenerateTreasureBoxes(t *testing.T) {
	pool := []string{"item-001", "item-002", "item-003"}
	boxes := adventure.GenerateTreasureBoxes(3, pool, func(n int) int { return 0 })

	if len(boxes) != 3 {
		t.Fatalf("expected 3 boxes, got %d", len(boxes))
	}
	if boxes[0].ID != "box-1" || boxes[0].Name != "普通の宝箱A" || boxes[0].ItemID != "item-001" {
		t.Errorf("unexpected box[0]: %+v", boxes[0])
	}
	if boxes[1].ID != "box-2" || boxes[1].Name != "普通の宝箱B" {
		t.Errorf("unexpected box[1]: %+v", boxes[1])
	}
	if boxes[2].ID != "box-3" || boxes[2].Name != "普通の宝箱C" {
		t.Errorf("unexpected box[2]: %+v", boxes[2])
	}
}
