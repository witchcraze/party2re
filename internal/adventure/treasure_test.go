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
		// Legacy vs_monster.cgi:92 ($#partys+1)*3 = 2*3 = 6. Inclusive 0..6 produces 7 boxes.
		if got != 7 {
			t.Errorf("CalculateTreasureCount for %s: got %d, want 7 (2 * 3 + 1)", stageID, got)
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

func TestCalculateTreasureCount_PartySizeAndSpecialStages(t *testing.T) {
	specialStages := []string{"stage-17", "17", "stage-20", "20", "stage-21", "21"}
	normalStages := []string{"stage-01", "stage-10", "stage-00", ""}

	for _, stage := range specialStages {
		// In special stages (17, 20, 21), initial count = 3 * N.
		// Inclusive loop 0..count yields 3*N + 1 boxes when 0 dead.
		tests := []struct {
			partySize int
			wantBoxes int
		}{
			{partySize: 1, wantBoxes: 4},  // 1*3 + 1 = 4
			{partySize: 2, wantBoxes: 7},  // 2*3 + 1 = 7
			{partySize: 3, wantBoxes: 10}, // 3*3 + 1 = 10
			{partySize: 4, wantBoxes: 13}, // 4*3 + 1 = 13
		}
		for _, tc := range tests {
			got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
				StageID:      stage,
				AliveMembers: tc.partySize,
				TotalMembers: tc.partySize,
			})
			if got != tc.wantBoxes {
				t.Errorf("stage %s, partySize %d: got %d, want %d", stage, tc.partySize, got, tc.wantBoxes)
			}
		}
	}

	for _, stage := range normalStages {
		// In normal stages, initial count = N - 1.
		// Inclusive loop 0..count yields N boxes when 0 dead.
		tests := []struct {
			partySize int
			wantBoxes int
		}{
			{partySize: 1, wantBoxes: 1}, // 0 + 1 = 1
			{partySize: 2, wantBoxes: 2}, // 1 + 1 = 2
			{partySize: 3, wantBoxes: 3}, // 2 + 1 = 3
			{partySize: 4, wantBoxes: 4}, // 3 + 1 = 4
		}
		for _, tc := range tests {
			got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
				StageID:      stage,
				AliveMembers: tc.partySize,
				TotalMembers: tc.partySize,
			})
			if got != tc.wantBoxes {
				t.Errorf("normal stage %s, partySize %d: got %d, want %d", stage, tc.partySize, got, tc.wantBoxes)
			}
		}
	}
}

func TestCalculateTreasureCount_DeadMembersPenalty(t *testing.T) {
	// Normal stage: count = total - 1 - dead = alive - 1. Boxes = alive.
	testsNormal := []struct {
		total, alive, dead, want int
	}{
		{total: 4, alive: 3, dead: 1, want: 3},
		{total: 4, alive: 2, dead: 2, want: 2},
		{total: 4, alive: 1, dead: 3, want: 1},
		{total: 4, alive: 0, dead: 4, want: 0},
		{total: 2, alive: 1, dead: 1, want: 1},
		{total: 2, alive: 0, dead: 2, want: 0},
	}
	for _, tc := range testsNormal {
		got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
			StageID:      "stage-01",
			AliveMembers: tc.alive,
			DeadMembers:  tc.dead,
			TotalMembers: tc.total,
		})
		if got != tc.want {
			t.Errorf("normal stage total=%d, alive=%d, dead=%d: got %d, want %d", tc.total, tc.alive, tc.dead, got, tc.want)
		}
	}

	// Special stage 17: count = 3*total - dead. Boxes = 3*total - dead + 1.
	testsSpecial := []struct {
		total, alive, dead, want int
	}{
		{total: 4, alive: 3, dead: 1, want: 12}, // 3*4 - 1 + 1 = 12
		{total: 4, alive: 2, dead: 2, want: 11}, // 3*4 - 2 + 1 = 11
		{total: 4, alive: 1, dead: 3, want: 10}, // 3*4 - 3 + 1 = 10
		{total: 4, alive: 0, dead: 4, want: 0},  // Alive <= 0 clamped to 0
		{total: 2, alive: 1, dead: 1, want: 6},  // 3*2 - 1 + 1 = 6
		{total: 2, alive: 0, dead: 2, want: 0},  // Alive <= 0 clamped to 0
	}
	for _, tc := range testsSpecial {
		got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
			StageID:      "stage-17",
			AliveMembers: tc.alive,
			DeadMembers:  tc.dead,
			TotalMembers: tc.total,
		})
		if got != tc.want {
			t.Errorf("special stage 17 total=%d, alive=%d, dead=%d: got %d, want %d", tc.total, tc.alive, tc.dead, got, tc.want)
		}
	}
}

func TestCalculateTreasureCount_CanonicalJobAndItemAliases(t *testing.T) {
	// Verify Merchant job aliases
	merchantAliases := []string{"job-07", "job-7", "7", "merchant", "商人"}
	for _, alias := range merchantAliases {
		if !adventure.IsMerchantJob(alias) {
			t.Errorf("IsMerchantJob(%q) = false, want true", alias)
		}
	}
	nonMerchant := []string{"job-01", "hero", "warrior", "78", "81", ""}
	for _, alias := range nonMerchant {
		if adventure.IsMerchantJob(alias) {
			t.Errorf("IsMerchantJob(%q) = true, want false", alias)
		}
	}

	// Verify Treasure Hunter job aliases
	thAliases := []string{"job-78", "78", "treasure_hunter", "treasurehunter", "トレジャーハンター", "トレハン"}
	for _, alias := range thAliases {
		if !adventure.IsTreasureHunterJob(alias) {
			t.Errorf("IsTreasureHunterJob(%q) = false, want true", alias)
		}
	}
	nonTH := []string{"job-07", "7", "81", "thief", ""}
	for _, alias := range nonTH {
		if adventure.IsTreasureHunterJob(alias) {
			t.Errorf("IsTreasureHunterJob(%q) = true, want false", alias)
		}
	}

	// Verify Gambler / Pirate job aliases
	gamblerAliases := []string{"job-81", "81", "gambler", "pirate", "海賊", "勝負師"}
	for _, alias := range gamblerAliases {
		if !adventure.IsGamblerJob(alias) {
			t.Errorf("IsGamblerJob(%q) = false, want true", alias)
		}
	}
	nonGambler := []string{"job-07", "job-78", "7", "78", ""}
	for _, alias := range nonGambler {
		if adventure.IsGamblerJob(alias) {
			t.Errorf("IsGamblerJob(%q) = true, want false", alias)
		}
	}

	// Verify Lucky Pendant item aliases
	pendantAliases := []string{"item-191", "191", "lucky_pendant", "lucky-pendant", "海賊ハット", "ラッキーペンダント"}
	for _, alias := range pendantAliases {
		if !adventure.IsLuckyPendantID(alias) {
			t.Errorf("IsLuckyPendantID(%q) = false, want true", alias)
		}
	}
	nonPendant := []string{"item-001", "item-215", "158", ""}
	for _, alias := range nonPendant {
		if adventure.IsLuckyPendantID(alias) {
			t.Errorf("IsLuckyPendantID(%q) = true, want false", alias)
		}
	}
}

func TestCalculateTreasureCount_GamblerPendantProtection(t *testing.T) {
	// When Gambler rolls -1 in normal stage with 4 party members (count = 3):
	// Without Lucky Pendant: count becomes 2 -> 3 boxes.
	// With Lucky Pendant: count guarded at total - 1 (3). If pendant roll does not trigger -> 4 boxes!
	rngGamblerNeg := func(n int) int {
		if n == 3 {
			return 0 // Gambler rng(3) = 0
		}
		if n == 2 {
			return 1 // Gambler rng(2) = 1 -> delta = -1
		}
		return 1 // Pendant rng(3) does not trigger (must be 0 to trigger)
	}

	gotWithoutPendant := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		StageID:         "stage-01",
		AliveMembers:    4,
		TotalMembers:    4,
		HasGambler:      true,
		HasLuckyPendant: false,
		Rng:             rngGamblerNeg,
	})
	if gotWithoutPendant != 3 {
		t.Errorf("Gambler -1 without pendant: got %d, want 3", gotWithoutPendant)
	}

	// Call 1: Gambler rng(3) -> 0
	// Call 2: Gambler rng(2) -> 1
	// Call 3: Lucky pendant rng(3) -> 1 (no trigger)
	callCount := 0
	seqRng := func(n int) int {
		callCount++
		switch callCount {
		case 1:
			return 0 // Gambler rng(3)
		case 2:
			return 1 // Gambler rng(2) -> -1 penalty
		case 3:
			return 1 // Pendant rng(3) -> no bonus
		default:
			return 0
		}
	}
	gotGuarded := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		StageID:         "stage-01",
		AliveMembers:    4,
		TotalMembers:    4,
		HasGambler:      true,
		HasLuckyPendant: true,
		Rng:             seqRng,
	})
	if gotGuarded != 4 {
		t.Errorf("Gambler -1 WITH pendant (guarded at 4): got %d, want 4", gotGuarded)
	}
}
