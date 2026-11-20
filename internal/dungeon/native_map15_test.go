package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// Main floor (map/15/1.cgi), x across, y down; B (0,3), A (2,3), C (4,3),
// stairs 2 (2,0) to _1 and 4 (2,8) to _2:
//
//	00200
//	01010
//	01010
//	B1A1C
//	01010
//	00000
//	11S11
//	11011
//	10401
func map15Hero(id string) corecharacter.Character {
	c := createTestChar(id, 60, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID = 50, "job-01"
	return c
}

func newMap15Fixture(t *testing.T, store dungeon.ActiveExpeditionStore, id string) *nativeFixture {
	t.Helper()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap15())
	f := newNativeFixtureOn(t, store, map15Hero(id), dungeon.WithCustomDungeons(catalog))
	if _, err := f.svc.StartExpedition(context.Background(), id, dungeon.NativeMap15ID); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
	return f
}

// Map15 stays unlisted and start-rejected until its south branch lands (#2443).
func TestNativeMap15_UnlistedWithFloorLimits(t *testing.T) {
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap15ID {
			t.Fatal("map15 listed before its branches")
		}
	}
	f := newNativeFixture(t, map15Hero("unlisted"))
	if _, err := f.svc.StartExpedition(context.Background(), "unlisted", dungeon.NativeMap15ID); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Fatalf("start err=%v", err)
	}
	d := dungeon.NativeMap15()
	if d.RoundLimit != 30 || d.LeaderJobLevel != 50 || d.Initial != 1 || len(d.Variants) != 3 {
		t.Fatalf("map15 %+v", d)
	}
	main := d.Variants[0]
	if main.StartX != 2 || main.StartY != 6 || main.Width != 5 || main.Height != 9 {
		t.Fatalf("main floor geometry %+v", main)
	}
	// Each stair keeps the coordinate and lands on a passage.
	for i, tc := range []struct {
		x, y, limit int
	}{{2, 0, 60}, {2, 8, 50}} {
		fl := d.Variants[i+1]
		if fl.Grid[tc.y][tc.x] != '0' || fl.RoundLimit != tc.limit {
			t.Fatalf("floor %d landing %q, limit %d", i+2, fl.Grid[tc.y][tc.x], fl.RoundLimit)
		}
	}
}

func TestNativeMap15_ExclusivePool(t *testing.T) {
	testMap14Stores(t, exclusivePool)
}

// A–C draw each of outcomes 2, 3, 4 exactly once; a spent trigger is a plain
// passage, and nothing redraws after reload.
func exclusivePool(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newMap15Fixture(t, store, id)
	// From (2,4) the top row of the radius-1 view is y=3 with A in the middle.
	tiles := func() []string {
		t.Helper()
		f.moveTo(t, id, 2, 4)
		view, err := f.svc.ObserveExpedition(ctx, id)
		if err != nil || view.Map == nil {
			t.Fatalf("observe %+v err=%v", view, err)
		}
		return view.Map.Tiles[0]
	}
	if got := tiles(); got[1] != "◎" || f.rng.calls() != 0 {
		t.Fatalf("A before use %q, draws %d", got[1], f.rng.calls())
	}
	drawn := ""
	for _, tc := range []struct {
		trigger byte
		x       int
		ints    []int
		floats  []float64
		event   dungeon.TileEventType
		monster string
		outcome string
	}{
		{'A', 2, []int{2}, nil, dungeon.EventBattle, "nm15-private", "4"},        // last of 234; always spawns
		{'B', 0, []int{0}, []float64{0.5}, dungeon.EventBoss, "nm15-blaze", "2"}, // first of 23; second group
		{'C', 4, nil, nil, dungeon.EventBoss, "nm15-bomb", "3"},                  // the only one left
	} {
		f.moveTo(t, id, tc.x, 4)
		before := f.reload(t, id)
		f.rng.ints, f.rng.floats = tc.ints, tc.floats
		res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth)
		if err != nil || res.EventType != tc.event {
			t.Fatalf("%c: event %s err=%v", tc.trigger, res.EventType, err)
		}
		after := f.reload(t, id)
		drawn += tc.outcome
		var st struct {
			Drawn string `json:"drawn"`
		}
		if err := json.Unmarshal(after.MapState, &st); err != nil || st.Drawn != drawn {
			t.Fatalf("%c: map state %s err=%v", tc.trigger, after.MapState, err)
		}
		if enc := after.Encounter; enc == nil || enc.Monster.ID != tc.monster || !slices.Contains(after.ConsumedEvents, string(tc.trigger)) {
			t.Fatalf("%c: encounter %+v flags %v", tc.trigger, enc, after.ConsumedEvents)
		}
		f.rejectStaleMap14Step(t, id, before, after)
		after.Encounter = nil
		if err := store.SaveActiveExpedition(ctx, after); err != nil {
			t.Fatal(err)
		}
		f.moveTo(t, id, tc.x, 4)
		f.rng.ints, f.rng.floats = nil, nil
		if res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
			t.Fatalf("%c repeated: event %s draws %d err=%v", tc.trigger, res.EventType, f.rng.calls(), err)
		}
		if again := f.reload(t, id); string(again.MapState) != string(after.MapState) {
			t.Fatalf("%c repeated changed state to %s", tc.trigger, again.MapState)
		}
	}
	if got := tiles(); got[1] != "□" {
		t.Fatalf("A after use %q", got[1])
	}
	raw, err := json.Marshal(f.reload(t, id))
	if err != nil || strings.Contains(string(raw), "drawn") {
		t.Fatalf("projection leaks map state %s err=%v", raw, err)
	}
}

// Outcome 2 picks the first group when rand(2) < 1.
func TestNativeMap15_FirstGroup(t *testing.T) {
	f := newMap15Fixture(t, dungeon.NewMemoryExpeditionRepository(), "group")
	f.moveTo(t, "group", 2, 4)
	f.rng.ints, f.rng.floats = []int{0}, []float64{0.49}
	if res, err := f.svc.Move(context.Background(), "group", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventBoss {
		t.Fatalf("event %s err=%v", res.EventType, err)
	}
	if enc := f.reload(t, "group").Encounter; enc == nil || enc.Monster.ID != "nm15-inferno" {
		t.Fatalf("encounter %+v", enc)
	}
}

// The stairs move to _1 and _2 at the same coordinate under their limits.
func TestNativeMap15_Stairs(t *testing.T) {
	for _, tc := range []struct {
		fromY   int
		dir     dungeon.Direction
		variant int
		toY     int
		limit   int
	}{{1, dungeon.DirectionNorth, 2, 0, 60}, {7, dungeon.DirectionSouth, 3, 8, 50}} {
		f := newMap15Fixture(t, dungeon.NewMemoryExpeditionRepository(), "stairs")
		f.moveTo(t, "stairs", 2, tc.fromY)
		before := f.reload(t, "stairs")
		if res, err := f.svc.Move(context.Background(), "stairs", tc.dir); err != nil || res.EventType != dungeon.EventMove {
			t.Fatalf("event %s err=%v", res.EventType, err)
		}
		after := f.reload(t, "stairs")
		if after.MapVariant != tc.variant || after.PosX != 2 || after.PosY != tc.toY || after.Round != before.Round+1 || after.RoundLimit != tc.limit {
			t.Fatalf("stairs to %d: %+v", tc.variant, after)
		}
	}
}

// Map15 enemies carry their legacy battle fields (__data.cgi:91–109).
func TestNativeMap15_EnemyFields(t *testing.T) {
	m := dungeon.NativeMap15().Variants[0].Pools[0].Monsters[1]
	a := m.EnemyCombatForTest()
	if m.ID != "nm15-roughneck" || a.Temporary != "null_magic" || !slices.Equal(a.ItemDefinitionIDs, []string{"item-147"}) || a.Jobs.OldJobID != "job-50" {
		t.Fatalf("roughneck %+v %+v", m, a)
	}
}
