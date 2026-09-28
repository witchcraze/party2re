package adventure_test

import (
	"testing"
	"time"

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

func TestCalculateTreasureCount_BlessingBonus(t *testing.T) {
	// rng(5) == 0: triggers +1 box (20% chance)
	gotTriggered := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:        4,
		HasTreasureBlessing: true,
		Rng:                 func(n int) int { return 0 },
	})
	if gotTriggered != 5 {
		t.Errorf("gotTriggered = %d, want 5 (4 + 1)", gotTriggered)
	}

	// rng(5) == 1: does not trigger
	gotNotTriggered := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:        4,
		HasTreasureBlessing: true,
		Rng:                 func(n int) int { return 1 },
	})
	if gotNotTriggered != 4 {
		t.Errorf("gotNotTriggered = %d, want 4", gotNotTriggered)
	}

	// HasTreasureBlessing = false: does not trigger even if rng returns 0
	gotDisabled := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:        4,
		HasTreasureBlessing: false,
		Rng:                 func(n int) int { return 0 },
	})
	if gotDisabled != 4 {
		t.Errorf("gotDisabled = %d, want 4", gotDisabled)
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

func TestCalculateTreasureCount_StageMultiplier(t *testing.T) {
	for _, stageID := range []string{"stage-17", "stage-20", "stage-21"} {
		got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
			AliveMembers: 2,
			StageID:      stageID,
		})
		if got != 6 {
			t.Errorf("CalculateTreasureCount for %s: got %d, want 6 (2 * 3)", stageID, got)
		}
	}

	// Normal stage should not have 3x multiplier
	gotNormal := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers: 2,
		StageID:      "stage-00",
	})
	if gotNormal != 2 {
		t.Errorf("CalculateTreasureCount for stage-00: got %d, want 2", gotNormal)
	}
}

func TestCalculateTreasureCount_GamblerBonus(t *testing.T) {
	// Gambler bonus: rng(3) - rng(2)
	// Case 1: Max positive bonus: rng(3) = 2, rng(2) = 0 -> +2
	gotPos := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers: 4,
		HasGambler:   true,
		Rng: func(n int) int {
			if n == 3 {
				return 2
			}
			return 0
		},
	})
	if gotPos != 6 {
		t.Errorf("gotPos = %d, want 6 (4 + 2)", gotPos)
	}

	// Case 2: Negative penalty without Lucky Pendant: rng(3) = 0, rng(2) = 1 -> -1 (4 - 1 = 3)
	gotNeg := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers: 4,
		HasGambler:   true,
		Rng: func(n int) int {
			if n == 3 {
				return 0
			}
			return 1
		},
	})
	if gotNeg != 3 {
		t.Errorf("gotNeg = %d, want 3 (4 - 1)", gotNeg)
	}

	// Case 3: Negative penalty WITH Lucky Pendant: guarded to not fall below baseCount (4)
	gotGuarded := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:    4,
		HasGambler:      true,
		HasLuckyPendant: true,
		Rng: func(n int) int {
			if n == 3 {
				return 0
			}
			return 1
		},
	})
	if gotGuarded < 4 {
		t.Errorf("gotGuarded = %d, expected >= 4 with Lucky Pendant", gotGuarded)
	}
}

func TestGetDayOfWeekOrb(t *testing.T) {
	tests := []struct {
		weekday time.Weekday
		rngVal  int
		wantOrb string
	}{
		{time.Monday, 0, "item-060"},
		{time.Tuesday, 0, "item-061"},
		{time.Wednesday, 0, "item-062"},
		{time.Thursday, 0, "item-063"},
		{time.Friday, 0, "item-064"},
		{time.Saturday, 0, "item-065"},
		{time.Sunday, 2, "item-062"}, // Sunday with rng(6) = 2 -> 60 + 2 = 62
		{time.Sunday, 5, "item-065"}, // Sunday with rng(6) = 5 -> 60 + 5 = 65
	}

	for _, tt := range tests {
		// 2026-08-23 was Sunday (0), 2026-08-24 Monday (1), etc.
		date := time.Date(2026, 8, 23+int(tt.weekday), 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
		orb := adventure.GetDayOfWeekOrb(date, func(n int) int { return tt.rngVal })
		if orb != tt.wantOrb {
			t.Errorf("GetDayOfWeekOrb(%v) = %s, want %s", tt.weekday, orb, tt.wantOrb)
		}
	}
}

func TestGetDayOfWeekOrb_JSTBoundary(t *testing.T) {
	// 2026-08-23 23:00 UTC is Sunday in UTC, but 2026-08-24 08:00 JST (Monday) in JST (UTC+9).
	utcSundayNight := time.Date(2026, 8, 23, 23, 0, 0, 0, time.UTC)
	orb := adventure.GetDayOfWeekOrb(utcSundayNight, func(n int) int { return 0 })
	// Should evaluate in JST to Monday (item-060), NOT UTC Sunday.
	if orb != "item-060" {
		t.Errorf("GetDayOfWeekOrb(UTC Sunday 23:00 = JST Monday 08:00) = %s, want item-060", orb)
	}
}

func TestGenerateTreasureBoxesWithPools(t *testing.T) {
	pools := adventure.StageTreasurePools{
		Weapons: []string{"weapon-01"},
		Armors:  []string{"armor-01"},
		Items:   []string{"item-001"},
	}

	// rand(4) + 1:
	// if rng(4) == 0 -> v = 1 -> weapon
	// if rng(4) == 1 -> v = 2 -> armor
	// if rng(4) == 2 -> v = 3 -> item
	// if rng(4) == 3 -> v = 4 (clamped to 3) -> item
	// When n == 4, roll category:
	// box 0: 0 -> v=1 (weapon)
	// box 1: 1 -> v=2 (armor)
	// box 2: 2 -> v=3 (item)
	categoryRolls := []int{0, 1, 2}
	rollIdx := 0
	rng := func(n int) int {
		if n == 4 && rollIdx < len(categoryRolls) {
			r := categoryRolls[rollIdx]
			rollIdx++
			return r
		}
		return 0
	}

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) // Monday -> item-060
	boxes := adventure.GenerateTreasureBoxesWithPools(3, pools, now, rng)

	if len(boxes) != 3 {
		t.Fatalf("expected 3 boxes, got %d", len(boxes))
	}
	if boxes[0].ItemID != "weapon-01" {
		t.Errorf("box[0] ItemID = %s, want weapon-01", boxes[0].ItemID)
	}
	if boxes[1].ItemID != "armor-01" {
		t.Errorf("box[1] ItemID = %s, want armor-01", boxes[1].ItemID)
	}
}
