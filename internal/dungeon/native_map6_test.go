package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Variant 1 (map/6/1.cgi); 2 and 5 have 203 on top, 3 has 11D and 4 C11 on row 9:
//
//	020  1X1  101  000  A1B  050  101  101  000
//	C1D  060  101  101  000  E1F  070  101  1S1
func map6Hero(id string, hp int) corecharacter.Character {
	c := createTestChar(id, 30, hp, 900, 900)
	c.JobLevel = 13
	return c
}

func newMap6Fixture(t *testing.T, store dungeon.ActiveExpeditionStore, hero corecharacter.Character) *nativeFixture {
	t.Helper()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap6())
	return newNativeFixtureOn(t, store, hero, dungeon.WithCustomDungeons(catalog))
}

// Map6 stays unlisted and start-rejected until its gold outcomes land (#2417).
func TestNativeMap6_UnlistedUntilGold(t *testing.T) {
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap6ID {
			t.Fatal("map6 listed before its gold outcomes")
		}
	}
	f := newNativeFixture(t, map6Hero("unlisted", 500))
	if _, err := f.svc.StartExpedition(context.Background(), "unlisted", dungeon.NativeMap6ID); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Fatalf("start err=%v", err)
	}
}

func TestNativeMap6_Layouts(t *testing.T) {
	d := dungeon.NativeMap6()
	if d.RoundLimit != 30 || d.LeaderJobLevel != 13 || len(d.Variants) != 5 {
		t.Fatalf("map6 %+v", d)
	}
	for i, fl := range d.Variants {
		all := strings.Join(fl.Grid, "")
		if fl.Width != 3 || fl.Height != 18 || fl.StartX != 1 || fl.StartY != 17 {
			t.Fatalf("variant %d geometry %+v", i+1, fl)
		}
		for _, sym := range []string{"X", "2", "5", "6", "7", "A", "B", "E", "F"} {
			if strings.Count(all, sym) != 1 {
				t.Fatalf("variant %d has %d %s cells", i+1, strings.Count(all, sym), sym)
			}
		}
		wantCD := 2
		if i == 2 || i == 3 {
			wantCD = 1
		}
		if strings.Count(all, "C")+strings.Count(all, "D") != wantCD {
			t.Fatalf("variant %d row 9 %q", i+1, fl.Grid[9])
		}
		if (i == 0) != (strings.Count(all, "3") == 0) {
			t.Fatalf("variant %d treasure 3 count", i+1)
		}
		// Every non-wall cell, the hidden one included, connects the start to the top.
		seen := map[[2]int]bool{{1, 17}: true}
		queue := [][2]int{{1, 17}}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, dlt := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				q := [2]int{p[0] + dlt[0], p[1] + dlt[1]}
				if q[0] < 0 || q[1] < 0 || q[0] >= 3 || q[1] >= 18 || seen[q] || fl.Grid[q[1]][q[0]] == '1' {
					continue
				}
				seen[q] = true
				queue = append(queue, q)
			}
		}
		if !seen[[2]int{0, 0}] || !seen[[2]int{2, 0}] {
			t.Fatalf("variant %d top unreachable", i+1)
		}
	}
}

// approach is the cell north of each choice cell; the move is south.
var map6Approach = map[byte][2]int{'A': {0, 3}, 'B': {2, 3}, 'C': {0, 8}, 'D': {2, 8}, 'E': {0, 13}, 'F': {2, 13}}

// Each choice draws rand(k) < 1 for its first outcome: just below 1/k takes
// it, 1/k takes the second (map/6/<n>.cgi:35–45). Random bases draw 0.5.
func TestNativeMap6_ChoiceOutcomesAtBoundaries(t *testing.T) {
	const below = 1e-9
	for _, tc := range []struct {
		variant       int
		cell          byte
		odds          float64
		first, second string
	}{
		{1, 'A', 3, "monster:nm6-silver-drop", "monster:nm6-slime"},
		{1, 'B', 7, "monster:nm6-silver-king", "monster:nm6-king-slime"},
		{1, 'C', 2, "trap", "none"}, // gold pickup: #2417
		{1, 'D', 2, "heal", "none"}, // gold loss: #2417
		{1, 'E', 3, "boss:stage", "monster:stage"},
		{1, 'F', 3, "rock", "none"},
		{2, 'A', 5, "treasure", "boss:nm6-man-eater,nm6-mimic,nm6-pandora"},
		{2, 'B', 3, "boss:stage", "monster:stage"},
		{2, 'C', 0, "boss:stage", "boss:stage"},
		{2, 'D', 3, "monster:stage", "rock"},
		{2, 'E', 0, "boss:nm6-pup-a,nm6-cerberus,nm6-pup-b", "boss:nm6-pup-a,nm6-cerberus,nm6-pup-b"},
		{2, 'F', 0, "boss:nm6-cub-a,nm6-shadow-panther,nm6-cub-b", "boss:nm6-cub-a,nm6-shadow-panther,nm6-cub-b"},
		{3, 'A', 0, "boss:stage", "boss:stage"},
		{3, 'B', 6, "treasure", "boss:nm6-man-eater,nm6-mimic,nm6-pandora"},
		{3, 'D', 0, "none", "none"},
		{3, 'E', 2, "boss:stage", "monster:stage"},
		{3, 'F', 2, "rock", "none"},
		{4, 'C', 0, "none", "none"},
		{5, 'C', 3, "heal", "rock"},
		{5, 'D', 3, "trap", "heal"},
		{5, 'E', 5, "treasure", "boss:nm6-man-eater,nm6-mimic,nm6-pandora"},
		{5, 'F', 3, "none", "none"}, // gold pickup / loss: #2417
	} {
		for side, want := range []string{tc.first, tc.second} {
			draw := 0.0
			if tc.odds > 0 {
				draw = 1/tc.odds - below
				if side == 1 {
					draw = 1 / tc.odds
				}
			}
			got := map6Outcome(t, tc.variant, tc.cell, draw)
			if got != want {
				t.Errorf("variant %d %c draw %v: %s, want %s", tc.variant, tc.cell, draw, got, want)
			}
		}
	}
}

// map6Outcome enters one choice cell at 300/500 HP and classifies its effect.
func map6Outcome(t *testing.T, variant int, cell byte, draw float64) string {
	t.Helper()
	ctx := context.Background()
	f := newMap6Fixture(t, dungeon.NewMemoryExpeditionRepository(), map6Hero("out", 500))
	at := map6Approach[cell]
	f.placeNative(t, dungeon.NativeMap6ID, "out", variant, at[0], at[1], 2)
	f.setVitality(t, "out", "out", 300, 0)
	f.rng.floats = []float64{draw, 0.5}
	res, err := f.svc.Move(ctx, "out", dungeon.DirectionSouth)
	if err != nil {
		t.Fatalf("variant %d %c: %v", variant, cell, err)
	}
	exp := f.reload(t, "out")
	if !slices.ContainsFunc(exp.ConsumedEvents, func(k string) bool { return strings.ContainsRune(k, rune(cell)) }) {
		t.Fatalf("variant %d %c flag missing: %v", variant, cell, exp.ConsumedEvents)
	}
	if len(res.Monsters) != 0 {
		kind := "monster"
		if res.EventType == dungeon.EventBoss {
			kind = "boss"
		}
		if !strings.HasPrefix(res.Monsters[0].ID, "nm6-") {
			return kind + ":stage"
		}
		return kind + ":" + strings.Join(slices.Compact(monsterIDs(res.Monsters)), ",") // an ordinary group repeats its one source
	}
	if res.EventType == dungeon.EventTreasure {
		return "treasure"
	}
	switch hp := memberOf(t, exp, "out").CurrentHP; hp {
	case 300:
		return "none"
	case 300 - 119: // int(100 × 1.197): base rand(200) = 100
		return "trap"
	case 300 - 126: // int(120 × 1.05): the member roll draws 0.5
		return "rock"
	case 300 + 59: // int(50 × 1.197): base rand(100) = 50
		return "heal"
	default:
		return fmt.Sprintf("hp:%d", hp)
	}
}

// A pair runs once: the other side, a re-entry and a reload draw nothing.
func TestNativeMap6_PairOnceAcrossReload(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		pairOnce(t, dungeon.NewMemoryExpeditionRepository(), "pair")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "pair-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		pairOnce(t, store, id)
	})
}

func pairOnce(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newMap6Fixture(t, store, map6Hero(id, 500))
	f.placeNative(t, dungeon.NativeMap6ID, id, 1, 2, 13, 2)
	f.rng.floats = []float64{0} // F: falling rock
	if _, err := f.svc.Move(ctx, id, dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, id); !slices.Equal(exp.ConsumedEvents, []string{"EF"}) || memberOf(t, exp, id).CurrentHP != 500-143 {
		t.Fatalf("F %v %+v", exp.ConsumedEvents, exp.Members)
	}
	// Passages draw their base spawn (0.99: none); the other side and F itself draw nothing.
	for _, step := range []struct {
		d     dungeon.Direction
		event bool
	}{{dungeon.DirectionNorth, false}, {dungeon.DirectionWest, false}, {dungeon.DirectionWest, false}, {dungeon.DirectionSouth, true},
		{dungeon.DirectionNorth, false}, {dungeon.DirectionEast, false}, {dungeon.DirectionEast, false}, {dungeon.DirectionSouth, true}} {
		f.rng.intCalls, f.rng.floatCalls = 0, 0
		res, err := f.svc.Move(ctx, id, step.d)
		if err != nil || len(res.Monsters) != 0 || res.EventType != dungeon.EventMove || step.event && f.rng.calls() != 0 {
			t.Fatalf("after F, %s: %+v err=%v rng=%d", step.d, res, err, f.rng.calls())
		}
	}
	if exp := f.reload(t, id); !slices.Equal(exp.ConsumedEvents, []string{"EF"}) || exp.PosX != 2 || exp.PosY != 14 {
		t.Fatalf("E/F replay %+v", exp)
	}
}

// _trap_d hits fallen members. _heals gives a fallen member's share to the
// actor (_check_party, _skill.cgi:9638). Both truncate only after the
// multiplier, so a base below 1 still moves 1 HP.
func TestNativeMap6_TrapAndHealTargetsWithFractionalBase(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name             string
		cell             byte
		floats           []float64
		wantLead, wantAl int
	}{
		{"trap", 'C', []float64{0, 0.0049}, 299, 0}, // base 0.98: int(0.98 × 1.197) = 1
		{"heal", 'D', []float64{0, 0.0099}, 302, 0}, // base 0.99: 1 for the actor, 1 for the fallen ally's share
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMap6Fixture(t, dungeon.NewMemoryExpeditionRepository(), map6Hero("lead", 500))
			f.chars.chars["ally"] = map6Hero("ally", 500)
			at := map6Approach[tc.cell]
			f.startNativeParty(t, dungeon.NativeMap6ID, 1, at[0], at[1])
			f.setVitality(t, "lead", "lead", 300, 0)
			f.setVitality(t, "lead", "ally", 0, 0)
			f.rng.floats = tc.floats
			if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth); err != nil {
				t.Fatal(err)
			}
			exp := f.reload(t, "lead")
			if memberOf(t, exp, "lead").CurrentHP != tc.wantLead || memberOf(t, exp, "ally").CurrentHP != tc.wantAl {
				t.Fatalf("%+v", exp.Members)
			}
		})
	}
}

// X is the _data.cgi sage group in every variant, once per run.
func TestNativeMap6_SagesInEveryVariant(t *testing.T) {
	ctx := context.Background()
	for v := 1; v <= 5; v++ {
		f := newMap6Fixture(t, dungeon.NewMemoryExpeditionRepository(), map6Hero("sage", 500_000))
		f.placeNative(t, dungeon.NativeMap6ID, "sage", v, 1, 2, 2)
		res, err := f.svc.Move(ctx, "sage", dungeon.DirectionNorth)
		if err != nil || res.EventType != dungeon.EventBoss || len(res.Monsters) == 0 || !slices.Contains(monsterIDs(res.Monsters), "nm6-sage-1") {
			t.Fatalf("variant %d X %+v err=%v", v, res, err)
		}
	}
}

// Variants 3/4 show their only route on row 9 as a wall until it is entered.
func TestNativeMap6_HiddenRouteRevealedOnEntry(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		variant int
		x       int
	}{{3, 2}, {4, 0}} {
		f := newMap6Fixture(t, dungeon.NewMemoryExpeditionRepository(), map6Hero("hide", 500))
		f.placeNative(t, dungeon.NativeMap6ID, "hide", tc.variant, 1, 8, 2)
		view := func() []string {
			v, err := f.svc.ObserveExpedition(ctx, "hide")
			if err != nil || v.Map == nil {
				t.Fatalf("observe %+v err=%v", v, err)
			}
			return v.Map.Tiles[2]
		}
		if row := view(); !slices.Equal(row, []string{"■", "■", "■"}) {
			t.Fatalf("variant %d before %v", tc.variant, row)
		}
		f.moveTo(t, "hide", tc.x, 10)
		res, err := f.svc.Move(ctx, "hide", dungeon.DirectionNorth)
		if err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
			t.Fatalf("variant %d entry %+v err=%v rng=%d", tc.variant, res, err, f.rng.calls())
		}
		f.moveTo(t, "hide", 1, 8)
		if row := view(); row[tc.x] != "□" || row[1] != "■" {
			t.Fatalf("variant %d after %v", tc.variant, row)
		}
	}
}
