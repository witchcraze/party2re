package adventure_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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

func TestCalculateTreasureCount_StageMultiplier(t *testing.T) {
	// Legacy vs_monster.cgi:92 ($#partys+1)*3 = 2*3 = 6. Inclusive 0..6 produces 7 boxes.
	if got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{AliveMembers: 2, TripleTreasure: true}); got != 7 {
		t.Errorf("triple treasure: got %d, want 7", got)
	}
	if got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{AliveMembers: 2}); got != 2 {
		t.Errorf("ordinary stage: got %d, want 2", got)
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

// Stage 0–5 tool pools keep the single legacy empty entry (stage/0.cgi–5.cgi @treasures).
func TestInitialStageCatalog_EmptyTreasureEntries(t *testing.T) {
	catalog, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range catalog.Stages() {
		want := 0
		if n, err := strconv.Atoi(strings.TrimPrefix(s.ID, "stage-")); err == nil && n <= 5 {
			want = 1
		}
		got := 0
		for _, pool := range [][]string{s.TreasureWeapons, s.TreasureArmors, s.TreasureItems} {
			got += slices.Index(pool, "") + 1
		}
		if got != want {
			t.Errorf("%s empty entries = %d, want %d", s.ID, got, want)
		}
	}
}

func TestCalculateTreasureCount_PartySizeAndSpecialStages(t *testing.T) {
	// Triple stages start at 3N, others at N-1; the inclusive loop adds one box.
	for n := 1; n <= 4; n++ {
		for _, triple := range []bool{true, false} {
			want := n
			if triple {
				want = 3*n + 1
			}
			if got := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{TripleTreasure: triple, AliveMembers: n, TotalMembers: n}); got != want {
				t.Errorf("triple=%v partySize=%d: got %d, want %d", triple, n, got, want)
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
			TripleTreasure: true,
			AliveMembers:   tc.alive,
			DeadMembers:    tc.dead,
			TotalMembers:   tc.total,
		})
		if got != tc.want {
			t.Errorf("special stage 17 total=%d, alive=%d, dead=%d: got %d, want %d", tc.total, tc.alive, tc.dead, got, tc.want)
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

func TestParticipantHasLuckyPendant(t *testing.T) {
	// 1. Participant without gear
	pNoGear := corebattle.MustNewParticipant("p1", 100, 10, 10)
	if adventure.ParticipantHasLuckyPendant(pNoGear) {
		t.Errorf("ParticipantHasLuckyPendant with no gear = true, want false")
	}

	// 2. Participant with Lucky Pendant equipped in accessory slot
	pEquipped := corebattle.MustNewParticipant("p2", 100, 10, 10)
	pEquipped.Gear = &corebattle.BattleGearStats{AccessoryID: adventure.ItemLuckyPendant}
	if !adventure.ParticipantHasLuckyPendant(pEquipped) {
		t.Errorf("ParticipantHasLuckyPendant with equipped accessory = false, want true")
	}

	// 3. Participant with Lucky Pendant only in inventory (unequipped)
	pUnequipped := corebattle.MustNewParticipant("p3", 100, 10, 10)
	pUnequipped.Gear = &corebattle.BattleGearStats{AccessoryID: "item-012"}
	pUnequipped.ItemDefinitionIDs = []string{adventure.ItemLuckyPendant}
	if adventure.ParticipantHasLuckyPendant(pUnequipped) {
		t.Errorf("ParticipantHasLuckyPendant with unequipped inventory item = true, want false")
	}
}

func TestCalculateTreasureCount_LegacyFalsyFallback(t *testing.T) {
	// Legacy _npc_action.cgi:499 (my $count = shift || $#partys):
	// When $count is 0 in Perl, it is falsy and falls back to $#partys (total - 1).
	// Scenario: Standard stage, 2-player party (total=2).
	// Initial count = 1.
	// Gambler rolls delta = -1 (rng(3)=0, rng(2)=1).
	// count becomes 1 - 1 = 0.
	// Legacy fallback resets count to $#partys = 1.
	// Final boxes spawned = 0..1 = 2 boxes!
	rngGamblerNeg := func(n int) int {
		if n == 3 {
			return 0
		}
		if n == 2 {
			return 1
		}
		return 1
	}

	// 2 alive, 0 dead: fallback resets count to 1 -> 2 boxes
	got2Alive := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:    2,
		TotalMembers:    2,
		HasGambler:      true,
		HasLuckyPendant: false,
		Rng:             rngGamblerNeg,
	})
	if got2Alive != 2 {
		t.Errorf("2-player party with Gambler delta -1 (fallback to 1): got %d, want 2", got2Alive)
	}

	// 1 alive, 1 dead: fallback resets count to 1, dead penalty subtracts 1 -> count = 0 -> 1 box
	got1Alive1Dead := adventure.CalculateTreasureCount(adventure.TreasureCalculationInput{
		AliveMembers:    1,
		DeadMembers:     1,
		TotalMembers:    2,
		HasGambler:      true,
		HasLuckyPendant: false,
		Rng:             rngGamblerNeg,
	})
	if got1Alive1Dead != 1 {
		t.Errorf("2-player party (1 dead) with Gambler delta -1: got %d, want 1", got1Alive1Dead)
	}
}
