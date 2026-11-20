package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Main floor (map/14/1.cgi), x across, y down; points A (8,8), B (1,1),
// C (1,8), D (4,1), E (9,4):
//
//	11010001101
//	0B01D000000
//	10011100001
//	01000000001
//	011010100E0
//	00010S00110
//	00110000100
//	00000101100
//	0C010000A00
//	00000110000
func map14Hero(id string) corecharacter.Character {
	c := createTestChar(id, 40, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID = 30, "job-01"
	return c
}

// startMap14 starts a solo run whose allocation draws are allot (after the
// variant draw).
func (f *nativeFixture) startMap14(t *testing.T, id string, allot ...int) dungeon.ActiveExpedition {
	t.Helper()
	f.rng.ints = append([]int{0}, allot...)
	if _, err := f.svc.StartExpedition(context.Background(), id, dungeon.NativeMap14ID); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
	return f.reload(t, id)
}

func (f *nativeFixture) setFloor(t *testing.T, id string, variant, x, y int) {
	t.Helper()
	exp := f.reload(t, id)
	exp.MapVariant, exp.PosX, exp.PosY = variant, x, y
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
}

func TestNativeMap14_CatalogAndLeaderAdmission(t *testing.T) {
	ctx := context.Background()
	hero := map14Hero("lead")
	hero.JobLevel = 29
	f := newNativeFixture(t, hero)
	d := mapDef(t, dungeon.NativeMap14ID)
	if d.RoundLimit != 200 || d.LeaderJobLevel != 30 || len(d.Variants) != 4 || d.Initial != 1 {
		t.Fatalf("map14 %+v", d)
	}
	if !reflect.DeepEqual(d.Variants[0].Pools, d.Variants[3].Pools) {
		t.Fatal("treasure ship does not use the main floor encounter pool")
	}
	for _, level := range []int{29, 30} {
		listed := slices.ContainsFunc(f.svc.ListDungeons(level), func(d dungeon.Dungeon) bool { return d.ID == dungeon.NativeMap14ID })
		if listed != (level == 30) {
			t.Fatalf("job level %d: listed=%t", level, listed)
		}
	}
	if _, err := f.svc.StartExpedition(ctx, hero.ID, dungeon.NativeMap14ID); !errors.Is(err, dungeon.ErrJobLevelRequired) {
		t.Fatalf("solo start at job level 29: %v", err)
	}
	ally := map14Hero("ally")
	ally.JobLevel = 1
	f.chars.chars[ally.ID] = ally
	roster := []string{hero.ID, ally.ID}
	if _, err := f.svc.StartPartyExpedition(ctx, hero.ID, roster, dungeon.NativeMap14ID, "p"); !errors.Is(err, dungeon.ErrJobLevelRequired) {
		t.Fatalf("party start at leader job level 29: %v", err)
	}
	hero.JobLevel = 30
	f.chars.chars[hero.ID] = hero
	if _, err := f.svc.StartPartyExpedition(ctx, hero.ID, roster, dungeon.NativeMap14ID, "p"); err != nil {
		t.Fatalf("party start at leader job level 30 with a level-1 joiner: %v", err)
	}
}

// Start draws three of A–E without replacement; the run never redraws them,
// and the API projection never shows them.
func TestNativeMap14_StartAllotsThreePoints(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		allot  []int
		active string // their marks, in grid order: B, D, E, C, A
	}{{[]int{0, 0, 0}, "ABC"}, {[]int{4, 3, 2}, "EDC"}} {
		f := newNativeFixture(t, map14Hero("allot"))
		exp := f.startMap14(t, "allot", tc.allot...)
		raw, err := json.Marshal(exp)
		if err != nil || strings.Contains(string(raw), "active") {
			t.Fatalf("projection leaks map state %s err=%v", raw, err)
		}
		for _, p := range []struct {
			sym  byte
			x, y int
		}{{'A', 8, 8}, {'B', 1, 1}, {'C', 1, 8}, {'D', 4, 1}, {'E', 9, 4}} {
			f.setFloor(t, "allot", 1, p.x-1, p.y)
			if p.sym == 'B' || p.sym == 'C' {
				f.setFloor(t, "allot", 1, p.x+1, p.y)
			}
			v, err := f.svc.ObserveExpedition(ctx, "allot")
			if err != nil || v.Map == nil {
				t.Fatalf("observe %+v err=%v", v, err)
			}
			col := 2
			if p.sym == 'B' || p.sym == 'C' {
				col = 0
			}
			want := "□"
			if strings.IndexByte(tc.active, p.sym) >= 0 {
				want = "★"
			}
			if got := v.Map.Tiles[1][col]; got != want {
				t.Fatalf("allot %v: %c shows %q, want %q", tc.allot, p.sym, got, want)
			}
		}
		if f.rng.calls() != 0 {
			t.Fatalf("observation drew %d", f.rng.calls())
		}
	}
}

// Each active point's first entry assigns a destination no point holds yet;
// R returns at the same coordinate; re-entry reuses the destination.
func TestNativeMap14_PointsRouteAndReturn(t *testing.T) {
	testMap14Stores(t, pointsRoute)
}

func testMap14Stores(t *testing.T, test func(*testing.T, dungeon.ActiveExpeditionStore, string)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) {
		test(t, dungeon.NewMemoryExpeditionRepository(), "map14")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map14-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		test(t, store, id)
	})
}

var map14Directions = map[byte]dungeon.Direction{
	'N': dungeon.DirectionNorth, 'S': dungeon.DirectionSouth, 'E': dungeon.DirectionEast, 'W': dungeon.DirectionWest,
}

func (f *nativeFixture) walkMap14(t *testing.T, id, path string) {
	t.Helper()
	for i := range len(path) {
		res, err := f.svc.Move(context.Background(), id, map14Directions[path[i]])
		if err != nil || res.EventType != dungeon.EventMove {
			t.Fatalf("walk %s, step %d: event %s, err=%v", path, i+1, res.EventType, err)
		}
	}
}

func pointsRoute(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	f := newNativeFixtureOn(t, store, map14Hero(id))
	f.startMap14(t, id, 0, 0, 0) // A, B, C active
	enter := func(path string, draws []int, variant, x, y int) dungeon.ActiveExpedition {
		t.Helper()
		last := len(path) - 1
		f.walkMap14(t, id, path[:last])
		before := f.reload(t, id)
		f.rng.ints, f.rng.intCalls, f.rng.floatCalls = draws, 0, 0
		f.walkMap14(t, id, path[last:])
		exp := f.reload(t, id)
		if exp.MapVariant != variant || exp.PosX != x || exp.PosY != y || exp.Round != before.Round+1 || f.rng.intCalls != len(draws) || f.rng.floatCalls != 0 {
			t.Fatalf("enter %s: variant %d at (%d,%d), round %d, draws %d/%d", path, exp.MapVariant, exp.PosX, exp.PosY, exp.Round, f.rng.intCalls, f.rng.floatCalls)
		}
		f.rejectStaleMap14Step(t, id, before, exp)
		return exp
	}
	enter("ESSSEE", []int{1}, 3, 8, 8) // A → _K
	enter("EENNWWWWWWWNN", []int{0}, 1, 3, 4)
	firstShip := enter("NWNWN", []int{1, 0, 9, 0, 7, 0}, 4, 1, 1) // B → _T, then disable sites
	var route struct {
		Disabled string `json:"disabled_treasure"`
	}
	if err := json.Unmarshal(firstShip.MapState, &route); err != nil || route.Disabled != "codne" {
		t.Fatalf("first ship state %s, err=%v", firstShip.MapState, err)
	}
	enter("SSSS", []int{0}, 1, 1, 5)
	if again := enter("SSEEENENNNWWWNWN", nil, 4, 1, 1); string(again.MapState) != string(firstShip.MapState) {
		t.Fatalf("ship re-entry changed route state to %s", again.MapState)
	}
	enter("SSSS", []int{0}, 1, 1, 5)
	enter("SSS", []int{0}, 2, 1, 8) // C → _P
	enter("NNEEEESSS", []int{0}, 1, 5, 9)
	enter("NEEE", nil, 3, 8, 8) // A again
}

func (f *nativeFixture) rejectStaleMap14Step(t *testing.T, id string, before, after dungeon.ActiveExpedition) {
	t.Helper()
	_, err := f.store.Step(context.Background(), id, dungeon.StepParams{
		ExpectedExpeditionID: before.ID, ExpectedRevision: &before.Revision, Combat: true,
		NewX: before.PosX, NewY: before.PosY, RoundDelta: 1, Now: time.Now().UTC(),
		MapVariant: before.MapVariant, MapState: before.MapState, EventKey: "stale", Encounter: before.Encounter,
	})
	if !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
		t.Fatalf("stale step err=%v", err)
	}
	got := f.reload(t, id)
	if got.Revision != after.Revision || got.MapVariant != after.MapVariant ||
		string(got.MapState) != string(after.MapState) || !slices.Equal(got.ConsumedEvents, after.ConsumedEvents) ||
		!reflect.DeepEqual(got.Encounter, after.Encounter) {
		t.Fatal("stale step changed the accepted route or event")
	}
}

func TestNativeMap14_TreasureSites(t *testing.T) {
	testMap14Stores(t, treasureSites)
}

func treasureSites(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newNativeFixtureOn(t, store, map14Hero(id))
	start := f.startMap14(t, id, 0, 0, 0)
	f.setFloor(t, id, 1, 7, 8)
	f.rng.ints = []int{2, 0, 9, 0, 7, 0} // _T; disable c, o, d, n, e
	f.walkMap14(t, id, "E")
	ship := mapDef(t, dungeon.NativeMap14ID).Variants[3]
	mark := func(pos, from [2]int) string {
		t.Helper()
		view, err := f.svc.ObserveExpedition(ctx, id)
		if err != nil || view.Map == nil {
			t.Fatalf("observe %+v, err=%v", view, err)
		}
		return view.Map.Tiles[1+pos[1]-from[1]][1+pos[0]-from[0]]
	}
	for i := range len("cdefghijklno") {
		site := "cdefghijklno"[i]
		pos := cellOf(ship, site)[0]
		from, _ := approach(ship, pos)
		f.setFloor(t, id, 4, from[0], from[1])
		want := "宝"
		if strings.IndexByte("codne", site) >= 0 {
			want = "□"
		}
		if got := mark(pos, from); got != want || f.rng.calls() != 0 {
			t.Fatalf("initial mark for %c = %q (want %q), draws=%d", site, got, want, f.rng.calls())
		}
	}
	for _, tc := range []struct {
		site    byte
		draws   []float64
		event   dungeon.TileEventType
		monster string
	}{
		{'c', []float64{0}, dungeon.EventMove, ""},
		{'k', []float64{0.49, 0.99}, dungeon.EventTreasure, ""},
		{'f', []float64{0.5, 0.49}, dungeon.EventBoss, "nm14-ship-mimic"},
		{'h', []float64{0.5, 0.5}, dungeon.EventBoss, "nm14-armored-chest"},
	} {
		pos := cellOf(ship, tc.site)[0]
		from, dir := approach(ship, pos)
		f.setFloor(t, id, 4, from[0], from[1])
		before := f.reload(t, id)
		f.rng.floats = tc.draws
		res, err := f.svc.Move(ctx, id, dir)
		if err != nil || res.EventType != tc.event {
			t.Fatalf("site %c: event=%s err=%v", tc.site, res.EventType, err)
		}
		after := f.reload(t, id)
		flag := string(tc.site)
		flags := slices.Clone(before.ConsumedEvents)
		if tc.event != dungeon.EventMove {
			flags = append(flags, flag)
		}
		if !slices.Equal(after.ConsumedEvents, flags) || string(after.MapState) != string(before.MapState) {
			t.Fatalf("site %c persisted flags %v, map state %s", tc.site, after.ConsumedEvents, after.MapState)
		}
		enc := after.Encounter
		switch tc.event {
		case dungeon.EventMove:
			if enc != nil || after.PosX != pos[0] || after.PosY != pos[1] || f.rng.calls() != 0 {
				t.Fatalf("disabled site dispatched: position (%d,%d), encounter %+v, draws %d", after.PosX, after.PosY, enc, f.rng.calls())
			}
		case dungeon.EventTreasure:
			if enc == nil || enc.EventType != tc.event || len(enc.Chests) != 1 || enc.Chests[0].Opened || f.rng.calls() != 1 {
				t.Fatalf("chests %+v, draws %d", enc, f.rng.calls())
			}
		case dungeon.EventBoss:
			if enc == nil || enc.EventType != tc.event || enc.Monster.ID != tc.monster || len(enc.Enemies) != 1 || enc.Enemies[0].HP <= 0 {
				t.Fatalf("site %c pending encounter %+v", tc.site, enc)
			}
		}
		if enc != nil && (enc.X != pos[0] || enc.Y != pos[1]) {
			t.Fatalf("site %c pending destination (%d,%d)", tc.site, enc.X, enc.Y)
		}
		// Attempt to restore the pre-ship state and overwrite the accepted encounter.
		before.MapState = start.MapState
		f.rejectStaleMap14Step(t, id, before, after)
		after.Encounter, after.PosX, after.PosY = nil, from[0], from[1] // revisit after clearing the encounter
		if err := store.SaveActiveExpedition(ctx, after); err != nil {
			t.Fatal(err)
		}
		if got := mark(pos, from); got != "□" {
			t.Fatalf("site %c after visit is marked %q", tc.site, got)
		}
		f.rng.floats, f.rng.intCalls, f.rng.floatCalls = []float64{0}, 0, 0
		if res, err := f.svc.Move(ctx, id, dir); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
			t.Fatalf("site %c repeated: event=%s draws=%d err=%v", tc.site, res.EventType, f.rng.calls(), err)
		}
		f.rng.floats = nil
	}
}

// An inactive point is a passage: rand(2) <= 1 spawns from the main pool.
func TestNativeMap14_InactivePointIsAPassage(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map14Hero("idle"))
	f.startMap14(t, "idle", 0, 0, 0) // D inactive
	f.setFloor(t, "idle", 1, 5, 1)
	f.rng.floats = []float64{0.99}
	if res, err := f.svc.Move(ctx, "idle", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("quiet %+v err=%v", res, err)
	}
	if exp := f.reload(t, "idle"); exp.MapVariant != 1 || exp.PosX != 4 {
		t.Fatalf("inactive point moved floors %+v", exp)
	}
	f.setFloor(t, "idle", 1, 5, 1)
	f.rng.floats, f.rng.ints = []float64{0.5}, []int{14}
	res, err := f.svc.Move(ctx, "idle", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventBattle || res.Monster == nil || res.Monster.ID != "nm14-turtle-warrior" {
		t.Fatalf("spawn %+v err=%v", res.Monster, err)
	}
}

// The lower exits from _P and _T land on a main-floor wall; the party walks out.
func TestNativeMap14_ReturnIntoAWall(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name           string
		variant, fromY int
		exit           dungeon.Direction
		landingX       int
	}{
		{"_P", 2, 8, dungeon.DirectionSouth, 5},
		{"_T", 4, 9, dungeon.DirectionEast, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map14Hero("wall"))
			f.startMap14(t, "wall", 0, 0, 0)
			f.setFloor(t, "wall", tc.variant, 5, tc.fromY)
			if _, err := f.svc.Move(ctx, "wall", tc.exit); err != nil {
				t.Fatal(err)
			}
			if exp := f.reload(t, "wall"); exp.MapVariant != 1 || exp.PosX != tc.landingX || exp.PosY != 9 {
				t.Fatalf("landing %+v", exp)
			}
			if _, err := f.svc.Move(ctx, "wall", dungeon.DirectionNorth); err != nil {
				t.Fatalf("walk out err=%v", err)
			}
		})
	}
}

// _P's and _K's bosses and treasure fire once per run across revisits.
func TestNativeMap14_DestinationEventsOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name          string
		variant, x, y int
		dir           dungeon.Direction
		want          dungeon.TileEventType
	}{
		{"P", 2, 6, 1, dungeon.DirectionEast, dungeon.EventBoss},
		{"a", 2, 8, 1, dungeon.DirectionSouth, dungeon.EventTreasure},
		{"K", 3, 5, 6, dungeon.DirectionNorth, dungeon.EventBoss},
		{"t", 3, 7, 2, dungeon.DirectionNorth, dungeon.EventTreasure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map14Hero("once"))
			f.startMap14(t, "once", 0, 0, 0)
			f.setFloor(t, "once", tc.variant, tc.x, tc.y)
			if res, err := f.svc.Move(ctx, "once", tc.dir); err != nil || res.EventType != tc.want {
				t.Fatalf("first %+v err=%v", res, err)
			}
			exp := f.reload(t, "once")
			exp.Encounter, exp.MapVariant, exp.PosX, exp.PosY = nil, 1, 0, 0
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			f.setFloor(t, "once", tc.variant, tc.x, tc.y) // a later visit
			if res, err := f.svc.Move(ctx, "once", tc.dir); err != nil || res.EventType != dungeon.EventMove {
				t.Fatalf("repeat %+v err=%v", res, err)
			}
		})
	}
}

// Map-owned enemies keep their legacy held item (ite) like stage enemies do.
func TestNativeMap14_CleioHoldsItem148(t *testing.T) {
	for _, m := range mapDef(t, dungeon.NativeMap14ID).Variants[2].Pools[0].Monsters {
		if m.ID == "nm14-cleio" {
			if items := m.EnemyCombatForTest().ItemDefinitionIDs; !slices.Equal(items, []string{"item-148"}) {
				t.Fatalf("cleio items %v", items)
			}
			return
		}
	}
	t.Fatal("cleio missing from the _K pool")
}
