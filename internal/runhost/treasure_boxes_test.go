package runhost

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGenerateTreasureBoxes(t *testing.T) {
	pool := []string{"item-001", "item-002", "item-003"}
	boxes := GenerateTreasureBoxes(3, TreasurePools{Items: pool}, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC), func(n int) int { return 0 })

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
		orb := dayOfWeekOrb(date, func(n int) int { return tt.rngVal })
		if orb != tt.wantOrb {
			t.Errorf("GetDayOfWeekOrb(%v) = %s, want %s", tt.weekday, orb, tt.wantOrb)
		}
	}
}

func TestGetDayOfWeekOrb_JSTBoundary(t *testing.T) {
	// 2026-08-23 23:00 UTC is Sunday in UTC, but 2026-08-24 08:00 JST (Monday) in JST (UTC+9).
	utcSundayNight := time.Date(2026, 8, 23, 23, 0, 0, 0, time.UTC)
	orb := dayOfWeekOrb(utcSundayNight, func(n int) int { return 0 })
	// Should evaluate in JST to Monday (item-060), NOT UTC Sunday.
	if orb != "item-060" {
		t.Errorf("GetDayOfWeekOrb(UTC Sunday 23:00 = JST Monday 08:00) = %s, want item-060", orb)
	}
}

func TestGenerateTreasureBoxesPools(t *testing.T) {
	pools := TreasurePools{
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
	boxes := GenerateTreasureBoxes(3, pools, now, rng)

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

// Legacy add_treasure pushes the weekday orb inside the chest loop, so chest i
// draws its tool from the stage pool plus i+1 orbs (#2118).
func TestGenerateTreasureBoxesPools_OrbsAccumulatePerChest(t *testing.T) {
	for _, tc := range []struct {
		name      string
		now       time.Time
		wantSunny int // rng(6) draws for Sunday's random orb
	}{
		{name: "weekday", now: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)},
		{name: "sunday", now: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC), wantSunny: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var toolPoolSizes []int
			sunny := 0
			rng := func(n int) int {
				switch n {
				case 4:
					return 2 // tool
				case 6:
					sunny++
				case 7: // box name
				default:
					toolPoolSizes = append(toolPoolSizes, n)
				}
				return n - 1
			}
			pools := TreasurePools{Weapons: []string{"weapon-01"}, Items: []string{"item-001", "item-002", "item-003", "item-004", "item-005", "item-006", "item-007"}}
			boxes := GenerateTreasureBoxes(3, pools, tc.now, rng)
			if want := []int{8, 9, 10}; !slices.Equal(toolPoolSizes, want) {
				t.Fatalf("tool pool sizes = %v, want %v", toolPoolSizes, want)
			}
			if sunny != tc.wantSunny {
				t.Errorf("sunday orb draws = %d, want %d", sunny, tc.wantSunny)
			}
			for i, b := range boxes {
				if !strings.HasPrefix(b.ItemID, "item-06") {
					t.Errorf("box %d = %q, want an orb", i, b.ItemID)
				}
			}
		})
	}
}

// A legacy empty pool entry yields an empty chest, never an invented item (#2118).
func TestGenerateTreasureBoxesPools_EmptyEntryStaysEmpty(t *testing.T) {
	pools := TreasurePools{Items: []string{"", "item-001"}}
	boxes := GenerateTreasureBoxes(1, pools, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC), func(int) int { return 0 })
	if len(boxes) != 1 || boxes[0].ItemID != "" {
		t.Fatalf("boxes = %+v, want one empty chest", boxes)
	}
}

func TestIsTreasureBlessingAcceptsOnlyStoredChapelValue(t *testing.T) {
	// The chapel adapter supplies chapel.BlessingDrop ("DROP"); display names and numbers never reach a room library.
	for _, b := range []string{"DROP", "drop", "4", "宝箱がほしい", ""} {
		if got := IsTreasureBlessing(b); got != (b == "DROP") {
			t.Errorf("IsTreasureBlessing(%q) = %v", b, got)
		}
	}
}

// add_treasure without a room count (Challenge): $#partys, the 1/5 blessing
// roll, then one fewer per fallen member; the loop 0..count is inclusive.
func TestTreasureCountDefaultsToPartyMinusOne(t *testing.T) {
	never := func(int) int { t.Fatal("unplanned draw"); return 0 }
	for _, tc := range []struct {
		name                 string
		count, total, fallen int
		blessed              bool
		roll, want           int
	}{
		{name: "solo", total: 1, want: 1},
		{name: "party of four", total: 4, want: 4},
		{name: "fallen members remove boxes", total: 4, fallen: 2, want: 2},
		{name: "blessing hit", total: 2, blessed: true, roll: 0, want: 3},
		{name: "blessing miss", total: 2, blessed: true, roll: 1, want: 2},
		{name: "room count kept", count: 6, total: 2, want: 7},
		{name: "never negative", total: 1, fallen: 3},
	} {
		rng := never
		if tc.blessed {
			rng = func(n int) int {
				if n != 5 {
					t.Fatalf("draw rand(%d), want rand(5)", n)
				}
				return tc.roll
			}
		}
		if got := TreasureCount(tc.count, tc.total, tc.fallen, tc.blessed, rng); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
