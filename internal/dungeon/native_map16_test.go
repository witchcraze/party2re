package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// Map16 main floor (map/16/1.cgi), x across, y down: S (3,6); D (3,1) above
// d (3,2); C (7,3) above c (7,4); B (6,8) below b (6,7); X (1,4) above stairs
// 2 (1,5). Spring floor _1 (variant 2): landing (1,5) west of Y (2,5); springs
// R at (4,0), (7,5), (3,8); E (4,3) west of e (5,3).
func map16Hero(id string) corecharacter.Character {
	c := createTestChar(id, 60, 1_000_000, 1, 900) // survives and never wins
	c.JobID, c.Stats.MaxMP = "job-01", 900
	return c
}

// map16Monday is a Monday in JST.
var map16Monday = time.Date(2026, 11, 30, 12, 0, 0, 0, timer.JST)

func newMap16Fixture(t *testing.T, store dungeon.ActiveExpeditionStore, id string, now time.Time) *nativeFixture {
	t.Helper()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap16())
	f := newNativeFixtureOn(t, store, map16Hero(id), dungeon.WithCustomDungeons(catalog), dungeon.WithClock(func() time.Time { return now }))
	if _, err := f.svc.StartExpedition(context.Background(), id, dungeon.NativeMap16ID); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.floats, f.rng.intCalls, f.rng.floatCalls = nil, nil, 0, 0
	return f
}

type map16State struct {
	Site       string   `json:"site,omitempty"`
	SpringUsed bool     `json:"spring_used,omitempty"`
	Kinds      []string `json:"kinds,omitempty"`
	Depleted   []string `json:"depleted,omitempty"`
}

func map16StateOf(t *testing.T, exp dungeon.ActiveExpedition) (st map16State) {
	t.Helper()
	if len(exp.MapState) > 0 {
		if err := json.Unmarshal(exp.MapState, &st); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func (f *nativeFixture) setMap16State(t *testing.T, id string, st map16State, round int) {
	t.Helper()
	exp := f.reload(t, id)
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	exp.MapState, exp.Round = raw, round
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
}

func (f *nativeFixture) move16(t *testing.T, id string, dir dungeon.Direction) dungeon.ActiveExpedition {
	t.Helper()
	if _, err := f.svc.Move(context.Background(), id, dir); err != nil {
		t.Fatal(err)
	}
	return f.reload(t, id)
}

// Map16 stays unlisted until its sealed floors land; every floor of the
// transcription is 8×9 and the first two are fully reachable.
func TestNativeMap16_UnlistedAndLayout(t *testing.T) {
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap16ID {
			t.Fatal("map16 listed before its sealed floors")
		}
	}
	f := newNativeFixture(t, map16Hero("unlisted"))
	if _, err := f.svc.StartExpedition(context.Background(), "unlisted", dungeon.NativeMap16ID); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Fatalf("start err=%v", err)
	}
	d := dungeon.NativeMap16()
	if d.RoundLimit != 150 || d.Initial != 1 || d.LeaderJobLevel != 0 || len(d.Variants) != 4 {
		t.Fatalf("map16 %+v", d)
	}
	for i, fl := range d.Variants {
		if fl.Width != 8 || fl.Height != 9 || fl.RoundLimit != 0 {
			t.Fatalf("floor %d geometry %dx%d limit %d", i+1, fl.Width, fl.Height, fl.RoundLimit)
		}
	}
	main, spring := d.Variants[0], d.Variants[1]
	if main.StartX != 3 || main.StartY != 6 || spring.Grid[5][1] != '0' {
		t.Fatalf("start (%d,%d), _1 landing %q", main.StartX, main.StartY, spring.Grid[5][1])
	}
	spring.StartX, spring.StartY = 1, 5
	for _, fl := range []dungeon.Floor{main, spring} {
		seen := reachable(fl, true)
		for y, row := range fl.Grid {
			for x := range len(row) {
				if row[x] != '1' && !seen[[2]int{x, y}] {
					t.Fatalf("(%d,%d) %q unreachable", x, y, row[x])
				}
			}
		}
	}
}

// Main-floor 0 and the approaches spawn at 1/2 from _data.cgi; X spawns an
// ordinary crystal-rock group once.
func TestNativeMap16_MainFloorEncounters(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		from      [2]int
		dir       dungeon.Direction
		r         float64
		wantSpawn bool
	}{
		{"0 boundary", [2]int{3, 6}, dungeon.DirectionNorth, 0.5, true},
		{"0 above", [2]int{3, 6}, dungeon.DirectionNorth, 0.5001, false},
		{"b boundary", [2]int{5, 7}, dungeon.DirectionEast, 0.5, true},
		{"d above", [2]int{3, 3}, dungeon.DirectionNorth, 0.5001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMap16Fixture(t, dungeon.NewMemoryExpeditionRepository(), "main", map16Monday)
			f.moveTo(t, "main", tc.from[0], tc.from[1])
			f.rng.floats, f.rng.ints = []float64{tc.r}, []int{0}
			res, err := f.svc.Move(ctx, "main", tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			// The hero may beat the weak group within the step, so read the spawn from the result.
			if spawned := len(res.Monsters) > 0; spawned != tc.wantSpawn || spawned && res.Monsters[0].ID != "nm16-night-bat" {
				t.Fatalf("spawned %v, want spawn %t", monsterIDs(res.Monsters), tc.wantSpawn)
			}
		})
	}
	f := newMap16Fixture(t, dungeon.NewMemoryExpeditionRepository(), "x", map16Monday)
	f.moveTo(t, "x", 2, 4)
	f.rng.floats = []float64{0.99, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // size int(0.99 × 3.1) + 1 = 4
	exp := f.move16(t, "x", dungeon.DirectionWest)
	if exp.Encounter == nil || len(exp.Encounter.Monsters) != 4 || !slices.Contains(exp.ConsumedEvents, "X") {
		t.Fatalf("X: encounter %+v consumed %v", exp.Encounter, exp.ConsumedEvents)
	}
	for _, id := range monsterIDs(exp.Encounter.Monsters) {
		if id != "nm16-crystal-rock" {
			t.Fatalf("X spawned %s", id)
		}
	}
	f.consume(t, "x")
	f.moveTo(t, "x", 2, 4)
	f.rng.floats = []float64{0}
	if exp = f.move16(t, "x", dungeon.DirectionWest); exp.Encounter != nil || f.rng.calls() != 0 {
		t.Fatalf("spent X drew %d: %+v", f.rng.calls(), exp.Encounter)
	}
	// The stairs below X lead to _1 at the same coordinate, dispatching nothing.
	before := f.reload(t, "x")
	if exp = f.move16(t, "x", dungeon.DirectionSouth); exp.MapVariant != 2 || exp.PosX != 1 || exp.PosY != 5 || exp.Round != before.Round+1 || exp.Encounter != nil || exp.RoundLimit != 150 {
		t.Fatalf("stairs: variant %d at (%d,%d) round %d enc %+v", exp.MapVariant, exp.PosX, exp.PosY, exp.Round, exp.Encounter)
	}
	if _, err := f.svc.Move(ctx, "x", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrImpassableWall) {
		t.Fatalf("landing west err=%v", err)
	}
}

func TestNativeMap16_SiteFlag(t *testing.T) {
	testMap14Stores(t, siteFlag)
}

// Entering a site activates it and its approach clears it, on both floors.
func siteFlag(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	f := newMap16Fixture(t, store, id, map16Monday)
	defer func() { _, _ = f.svc.Escape(context.Background(), id) }()
	if st := map16StateOf(t, f.reload(t, id)); st.Site != "" {
		t.Fatalf("site at start %q", st.Site)
	}
	f.moveTo(t, id, 6, 7)
	before := f.reload(t, id)
	at := f.move16(t, id, dungeon.DirectionSouth)
	if map16StateOf(t, at).Site != "B" || at.Encounter != nil {
		t.Fatalf("B: state %s enc %+v", at.MapState, at.Encounter)
	}
	f.rejectStaleMap14Step(t, id, before, at)
	if map16StateOf(t, f.move16(t, id, dungeon.DirectionNorth)).Site != "" {
		t.Fatal("b kept the site")
	}
	f.setFloor(t, id, 2, 5, 3)
	if map16StateOf(t, f.move16(t, id, dungeon.DirectionWest)).Site != "E" {
		t.Fatal("E did not activate")
	}
	if map16StateOf(t, f.move16(t, id, dungeon.DirectionEast)).Site != "" {
		t.Fatal("e kept the site")
	}
}

// _1's 0 spawns at 1/3 before Y; after Y, at 1/6 a gem boss of an unrecorded
// kind, an ordinary group once all are recorded, else a spawn at 1/3.
func TestNativeMap16_SpringFloorField(t *testing.T) {
	all := []string{"i", "j", "k", "l", "m", "n"}
	for _, tc := range []struct {
		name   string
		y      bool
		kinds  []string
		floats []float64
		ints   []int
		want   string
		after  []string // nil: the kinds stay as given
	}{
		{"before Y boundary", false, nil, []float64{0.3333}, []int{0}, "nm16-black-dragon", nil},
		{"before Y above", false, nil, []float64{0.3334}, nil, "", nil},
		{"gem boss", true, nil, []float64{0.1666}, []int{3}, "nm16-emerald-demon", []string{"l"}},
		{"gem boss skips recorded", true, []string{"i", "j", "k", "l", "m"}, []float64{0}, []int{4}, "nm16-amethyst-demon", all},
		{"all recorded", true, all, []float64{0}, []int{0}, "nm16-black-dragon", all},
		{"after Y group", true, nil, []float64{0.1667, 0.3333}, []int{0}, "nm16-black-dragon", nil},
		{"after Y none", true, nil, []float64{0.1667, 0.3334}, nil, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMap16Fixture(t, dungeon.NewMemoryExpeditionRepository(), "field", map16Monday)
			f.setFloor(t, "field", 2, 2, 4)
			if tc.y {
				f.consume(t, "field", "Y")
			}
			f.setMap16State(t, "field", map16State{Kinds: tc.kinds}, 10)
			f.rng.floats, f.rng.ints = tc.floats, tc.ints
			exp := f.move16(t, "field", dungeon.DirectionNorth)
			if tc.want == "" {
				if exp.Encounter != nil {
					t.Fatalf("spawned %+v", exp.Encounter)
				}
				return
			}
			if exp.Encounter == nil || monsterIDs(exp.Encounter.Monsters)[0] != tc.want {
				t.Fatalf("encounter %+v, want %s", exp.Encounter, tc.want)
			}
			want := tc.after
			if want == nil {
				want = tc.kinds
			}
			if got := map16StateOf(t, exp).Kinds; !slices.Equal(got, want) {
				t.Fatalf("kinds %v, want %v", got, want)
			}
		})
	}
}

// One use empties all three springs; a _1 step landing on a multiple of 20
// refills them before the spring check; a wall hit never refills.
func TestNativeMap16_Springs(t *testing.T) {
	ctx := context.Background()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap16())
	f := newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), map16Hero("lead"), dungeon.WithCustomDungeons(catalog))
	f.chars.chars["ally"] = map16Hero("ally")
	f.startNativeParty(t, dungeon.NativeMap16ID, 1, 3, 6)
	drink := func(round int, from [2]int, dir dungeon.Direction) (healed bool) {
		t.Helper()
		f.setFloor(t, "lead", 2, from[0], from[1])
		st := map16StateOf(t, f.reload(t, "lead"))
		f.setMap16State(t, "lead", st, round)
		f.setVitality(t, "lead", "ally", 500, 0)
		exp := f.move16(t, "lead", dir)
		if !map16StateOf(t, exp).SpringUsed {
			t.Fatalf("round %d: springs full after entering", round+1)
		}
		return memberOf(t, exp, "ally").CurrentMP > 0
	}
	if !drink(10, [2]int{6, 5}, dungeon.DirectionEast) {
		t.Fatal("full spring did not heal")
	}
	if drink(11, [2]int{3, 7}, dungeon.DirectionSouth) {
		t.Fatal("another spring healed after one use")
	}
	// A wall hit beside the spring spends no round and refills nothing.
	f.setFloor(t, "lead", 2, 7, 5)
	f.setMap16State(t, "lead", map16StateOf(t, f.reload(t, "lead")), 39)
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionEast); !errors.Is(err, dungeon.ErrImpassableWall) || !map16StateOf(t, f.reload(t, "lead")).SpringUsed {
		t.Fatalf("wall hit: err=%v state %s", err, f.reload(t, "lead").MapState)
	}
	if !drink(39, [2]int{6, 5}, dungeon.DirectionEast) {
		t.Fatal("round-40 entry did not refill and heal")
	}
	// Any _1 event landing on round 60 refills, without drinking.
	f.setFloor(t, "lead", 2, 2, 4)
	f.setMap16State(t, "lead", map16StateOf(t, f.reload(t, "lead")), 59)
	f.rng.floats = []float64{0.99}
	if map16StateOf(t, f.move16(t, "lead", dungeon.DirectionNorth)).SpringUsed {
		t.Fatal("round-60 step did not refill")
	}
}

func TestNativeMap16_WeekdayHerald(t *testing.T) {
	testMap14Stores(t, weekdayHerald)
}

// Y fires once with the JST weekday's gem boss; a Sunday draw of 0 spawns
// __data.cgi's knights and records no kind.
func weekdayHerald(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	type tc struct {
		name  string
		now   time.Time
		ints  []int
		want  string
		kinds []string
	}
	cases := []tc{
		{"Sunday 0", map16Monday.AddDate(0, 0, 6), []int{0}, "nm16-white-knight-A", nil},
		{"Sunday 1", map16Monday.AddDate(0, 0, 6), []int{1}, "nm16-gibeon-demon", []string{"i"}},
		{"Sunday 5", map16Monday.AddDate(0, 0, 6), []int{5}, "nm16-topaz-demon", []string{"m"}},
		// 23:30 UTC on Sunday is Monday in JST.
		{"JST Monday", time.Date(2026, 12, 6, 23, 30, 0, 0, time.UTC), nil, "nm16-gibeon-demon", []string{"i"}},
	}
	for i, boss := range []string{"gibeon", "ruby", "sapphire", "emerald", "topaz", "amethyst"} {
		cases = append(cases, tc{"weekday " + boss, map16Monday.AddDate(0, 0, i), nil, "nm16-" + boss + "-demon", []string{string(rune('i' + i))}})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newMap16Fixture(t, store, id, c.now)
			defer func() { _, _ = f.svc.Escape(ctx, id) }()
			f.setFloor(t, id, 2, 1, 5)
			before := f.reload(t, id)
			f.rng.ints = c.ints
			exp := f.move16(t, id, dungeon.DirectionEast)
			if exp.Encounter == nil || monsterIDs(exp.Encounter.Monsters)[0] != c.want || !slices.Contains(exp.ConsumedEvents, "Y") {
				t.Fatalf("Y: encounter %+v consumed %v", exp.Encounter, exp.ConsumedEvents)
			}
			if got := map16StateOf(t, exp).Kinds; !slices.Equal(got, c.kinds) {
				t.Fatalf("kinds %v, want %v", got, c.kinds)
			}
			f.rejectStaleMap14Step(t, id, before, exp)
			f.consume(t, id)
			f.setFloor(t, id, 2, 1, 5)
			if again := f.move16(t, id, dungeon.DirectionEast); again.Encounter != nil || !slices.Equal(map16StateOf(t, again).Kinds, c.kinds) {
				t.Fatalf("spent Y: %+v state %s", again.Encounter, again.MapState)
			}
		})
	}
}

// Observation shows the marks and never the recorded kinds or the springs' flag.
func TestNativeMap16_Observation(t *testing.T) {
	ctx := context.Background()
	f := newMap16Fixture(t, dungeon.NewMemoryExpeditionRepository(), "view", map16Monday)
	tiles := func(variant, x, y int) [][]string {
		t.Helper()
		f.setFloor(t, "view", variant, x, y)
		view, err := f.svc.ObserveExpedition(ctx, "view")
		if err != nil {
			t.Fatal(err)
		}
		return view.Map.Tiles
	}
	if got := tiles(1, 2, 4); got[1][0] != "◎" || got[2][0] != "凸" {
		t.Fatalf("X and stairs %v", got)
	}
	if got := tiles(1, 6, 7); got[2][1] != "★" {
		t.Fatalf("B %v", got)
	}
	if got := tiles(2, 2, 4); got[2][1] != "◎" {
		t.Fatalf("Y %v", got)
	}
	if got := tiles(2, 6, 5); got[1][2] != "○" {
		t.Fatalf("spring %v", got)
	}
	f.consume(t, "view", "X", "Y")
	f.setMap16State(t, "view", map16State{SpringUsed: true, Kinds: []string{"i"}}, 1)
	if got := tiles(2, 6, 5); got[1][2] != "□" {
		t.Fatalf("used spring %v", got)
	}
	if got := tiles(2, 2, 4); got[2][1] != "□" {
		t.Fatalf("spent Y %v", got)
	}
	if got := tiles(1, 2, 4); got[1][0] != "□" {
		t.Fatalf("spent X %v", got)
	}
	raw, err := json.Marshal(f.reload(t, "view"))
	if err != nil || strings.Contains(string(raw), "kinds") || strings.Contains(string(raw), "spring") {
		t.Fatalf("projection leaks map state %s err=%v", raw, err)
	}
}
