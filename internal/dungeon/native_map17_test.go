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

// Map17 main floor (map/17/1.cgi), x across, y down. Start S (11,16); B (7,14)
// below b (7,13); switch 7 (9,4) beside a 9 at (10,4); stairs U at (3,16) and
// (16,16); hidden stairs 2 (9,14) below door z (9,12).
func map17Hero(id string) corecharacter.Character {
	c := createTestChar(id, 60, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID, c.Stats.MaxMP, c.Stats.MP = 40, "job-01", 900, 900
	return c
}

func newMap17Fixture(t *testing.T, store dungeon.ActiveExpeditionStore, id string) *nativeFixture {
	t.Helper()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap17())
	f := newNativeFixtureOn(t, store, map17Hero(id), dungeon.WithCustomDungeons(catalog))
	if _, err := f.svc.StartExpedition(context.Background(), id, dungeon.NativeMap17ID); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.floats, f.rng.intCalls, f.rng.floatCalls = nil, nil, 0, 0
	return f
}

// consume marks once-events as spent and clears any encounter.
func (f *nativeFixture) consume(t *testing.T, id string, keys ...string) {
	t.Helper()
	exp := f.reload(t, id)
	exp.ConsumedEvents = append(exp.ConsumedEvents, keys...)
	exp.Encounter = nil
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
}

func map17StateOf(t *testing.T, exp dungeon.ActiveExpedition) (st struct {
	Under  int    `json:"under"`
	Raised bool   `json:"raised"`
	Field  string `json:"field"`
}) {
	t.Helper()
	if len(exp.MapState) > 0 {
		if err := json.Unmarshal(exp.MapState, &st); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestNativeMap17_UnlistedWithFloorLimits(t *testing.T) {
	f := newNativeFixture(t, map17Hero("lead"))
	if slices.ContainsFunc(f.svc.ListDungeons(99), func(d dungeon.Dungeon) bool { return d.ID == dungeon.NativeMap17ID }) {
		t.Fatal("map17 listed before the secret room lands")
	}
	if _, err := f.svc.StartExpedition(context.Background(), "lead", dungeon.NativeMap17ID); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Fatalf("start: %v", err)
	}
	d := dungeon.NativeMap17()
	if d.RoundLimit != 200 || d.LeaderJobLevel != 40 || d.Initial != 1 || len(d.Variants) != 5 {
		t.Fatalf("map17 %+v", d)
	}
	if main := d.Variants[0]; main.StartX != 11 || main.StartY != 16 || main.Width != 20 || main.Height != 20 {
		t.Fatalf("main floor geometry %+v", main)
	}
	// Every main-floor stair lands on the underground stair at its coordinate.
	for y, row := range d.Variants[0].Grid {
		for x := range len(row) {
			if row[x] != 'U' {
				continue
			}
			for v := 1; v <= 3; v++ {
				if got := d.Variants[v].Grid[y][x]; got != 'u' || d.Variants[v].RoundLimit != 0 {
					t.Fatalf("floor %d at (%d,%d): %q", v+1, x, y, got)
				}
			}
		}
	}
}

// Encounters fire when 5r ≤ t: t = 1 + visited j/k, or 0 with the key; b
// switches the pool; 9 never spawns.
func TestNativeMap17_EncounterRate(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		consumed []string
		r        float64
		monster  string
	}{
		{"t=1 boundary", nil, 0.2, "nm17-blood-hand"},
		{"t=1 above", nil, 0.2001, ""},
		{"t=2", []string{"j"}, 0.4, "nm17-blood-hand"},
		{"t=3", []string{"j", "k"}, 0.6, "nm17-blood-hand"},
		{"t=3 above", []string{"j", "k"}, 0.6001, ""},
		{"key", []string{"j", "k", "Y"}, 0.0001, ""},
		{"after b", []string{"b"}, 0.2, "nm17-sphinx-a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "rate")
			f.consume(t, "rate", tc.consumed...)
			f.rng.floats, f.rng.ints = []float64{tc.r}, []int{0}
			res, err := f.svc.Move(ctx, "rate", dungeon.DirectionEast)
			if err != nil {
				t.Fatal(err)
			}
			enc := f.reload(t, "rate").Encounter
			if tc.monster == "" {
				if enc != nil || res.EventType != dungeon.EventMove {
					t.Fatalf("spawned %+v", enc)
				}
				return
			}
			if enc == nil || enc.Monster.ID != tc.monster {
				t.Fatalf("encounter %+v, want %s", enc, tc.monster)
			}
		})
	}
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "nine")
	f.moveTo(t, "nine", 10, 3) // (10,4) below is a 9
	f.rng.floats = []float64{0}
	if _, err := f.svc.Move(ctx, "nine", dungeon.DirectionSouth); err != nil || f.rng.calls() != 0 || f.reload(t, "nine").Encounter != nil {
		t.Fatalf("9 drew %d or spawned: %v", f.rng.calls(), err)
	}
}

func TestNativeMap17_SealingGate(t *testing.T) {
	testMap14Stores(t, sealingGate)
}

// B fights once; b installs den; afterwards B turns the party back north and
// spends the round.
func sealingGate(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newMap17Fixture(t, store, id)
	f.moveTo(t, id, 7, 15)
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventBoss {
		t.Fatalf("B: event %s err=%v", res.EventType, err)
	}
	boss := f.reload(t, id)
	if boss.Encounter == nil || boss.Encounter.Monster.ID != "nm17-sphinx-guard-a" || !slices.Contains(boss.ConsumedEvents, "B") {
		t.Fatalf("B encounter %+v consumed %v", boss.Encounter, boss.ConsumedEvents)
	}
	f.consume(t, id)
	before := f.reload(t, id)
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("b: event %s err=%v", res.EventType, err)
	}
	sealed := f.reload(t, id)
	if st := map17StateOf(t, sealed); st.Field != "den" || !slices.Contains(sealed.ConsumedEvents, "b") || sealed.PosY != 13 {
		t.Fatalf("b state %s consumed %v y %d", sealed.MapState, sealed.ConsumedEvents, sealed.PosY)
	}
	f.rejectStaleMap14Step(t, id, before, sealed)
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionSouth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
		t.Fatalf("sealed B: event %s draws %d err=%v", res.EventType, f.rng.calls(), err)
	}
	if back := f.reload(t, id); back.PosX != 7 || back.PosY != 13 || back.Round != sealed.Round+1 || back.Encounter != nil {
		t.Fatalf("sealed B: (%d,%d) round %d/%d", back.PosX, back.PosY, back.Round, sealed.Round)
	}
	view, err := f.svc.ObserveExpedition(ctx, id)
	if err != nil || view.Map.Tiles[2][1] != "■" {
		t.Fatalf("B after b shows %q err=%v", view.Map.Tiles[2][1], err)
	}
}

func TestNativeMap17_UndergroundDrawnOnce(t *testing.T) {
	testMap14Stores(t, undergroundDrawnOnce)
}

// The first stair draws the floor; every later stair reuses it, and u returns
// to the same coordinate.
func undergroundDrawnOnce(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	for draw := range 3 {
		f := newMap17Fixture(t, store, id)
		f.moveTo(t, id, 4, 16)
		before := f.reload(t, id)
		f.rng.ints = []int{draw}
		if _, err := f.svc.Move(ctx, id, dungeon.DirectionWest); err != nil {
			t.Fatal(err)
		}
		down := f.reload(t, id)
		if down.MapVariant != draw+2 || down.PosX != 3 || down.PosY != 16 || down.Round != before.Round+1 || down.RoundLimit != 200 || map17StateOf(t, down).Under != draw+1 {
			t.Fatalf("draw %d: variant %d at (%d,%d) round %d state %s", draw, down.MapVariant, down.PosX, down.PosY, down.Round, down.MapState)
		}
		f.rejectStaleMap14Step(t, id, before, down)
		// Step off the landing and back onto u.
		if _, err := f.svc.Move(ctx, id, dungeon.DirectionEast); err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Move(ctx, id, dungeon.DirectionWest); err != nil {
			t.Fatal(err)
		}
		if up := f.reload(t, id); up.MapVariant != 1 || up.PosX != 3 || up.PosY != 16 {
			t.Fatalf("u: variant %d at (%d,%d)", up.MapVariant, up.PosX, up.PosY)
		}
		f.moveTo(t, id, 15, 16)
		f.rng.ints, f.rng.intCalls = []int{(draw + 1) % 3}, 0
		if _, err := f.svc.Move(ctx, id, dungeon.DirectionEast); err != nil {
			t.Fatal(err)
		}
		if again := f.reload(t, id); again.MapVariant != draw+2 || f.rng.intCalls != 0 || string(again.MapState) != string(down.MapState) {
			t.Fatalf("second stair: variant %d draws %d state %s", again.MapVariant, f.rng.intCalls, again.MapState)
		}
		if _, err := f.svc.Escape(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeMap17_Switches(t *testing.T) {
	testMap14Stores(t, switches)
}

// Switch 7 at (9,4) warps to (4,10); its branches depend on the raised walls.
func switches(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		raised bool
		floats []float64
		want   string // lowered, nothing, raised, warp, treasure
	}{
		{"nothing", false, []float64{0.33}, "nothing"},
		{"raise", false, []float64{0.34, 0.24}, "raised"},
		{"warp", false, []float64{0.34, 0.25, 0.49}, "warp"},
		{"treasure", false, []float64{0.34, 0.25, 0.5}, "treasure"},
		{"lower", true, []float64{0.49}, "lowered"},
		{"raised nothing", true, []float64{0.5, 0.33}, "nothing"},
		{"raised warp", true, []float64{0.5, 0.34, 0.49}, "warp"},
		{"raised treasure", true, []float64{0.5, 0.34, 0.5}, "treasure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMap17Fixture(t, store, id)
			defer func() { _, _ = f.svc.Escape(ctx, id) }()
			if tc.raised {
				exp := f.reload(t, id)
				exp.MapState = json.RawMessage(`{"raised":true}`)
				if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
					t.Fatal(err)
				}
			}
			f.moveTo(t, id, 8, 4)
			before := f.reload(t, id)
			f.rng.floats = tc.floats
			if _, err := f.svc.Move(ctx, id, dungeon.DirectionEast); err != nil {
				t.Fatal(err)
			}
			after := f.reload(t, id)
			raised := map17StateOf(t, after).Raised
			x, y := after.PosX, after.PosY
			if f.rng.floatCalls != len(tc.floats) || !slices.Contains(after.ConsumedEvents, "7") {
				t.Fatalf("draws %d consumed %v", f.rng.floatCalls, after.ConsumedEvents)
			}
			switch tc.want {
			case "lowered", "nothing", "raised":
				if raised != (tc.want == "raised" || tc.want == "nothing" && tc.raised) || x != 9 || y != 4 || after.Encounter != nil {
					t.Fatalf("%s: raised %t at (%d,%d) enc %+v", tc.want, raised, x, y, after.Encounter)
				}
			case "warp":
				if raised != tc.raised || x != 4 || y != 10 || after.Encounter != nil || after.Round != before.Round+1 {
					t.Fatalf("warp: raised %t at (%d,%d) round %d", raised, x, y, after.Round)
				}
				return
			case "treasure":
				if raised != tc.raised || x != 9 || y != 4 || after.Encounter == nil || after.Encounter.EventType != dungeon.EventTreasure {
					t.Fatalf("treasure: raised %t at (%d,%d) enc %+v", raised, x, y, after.Encounter)
				}
				return
			}
			f.rejectStaleMap14Step(t, id, before, after)
			// The switch is spent, and the 9 beside it follows the walls.
			f.moveTo(t, id, 8, 4)
			f.rng.floats = nil
			if _, err := f.svc.Move(ctx, id, dungeon.DirectionEast); err != nil || f.rng.calls() != 0 {
				t.Fatalf("spent switch drew %d err=%v", f.rng.calls(), err)
			}
			if string(f.reload(t, id).MapState) != string(after.MapState) {
				t.Fatal("spent switch changed state")
			}
			view, err := f.svc.ObserveExpedition(ctx, id)
			if err != nil || (view.Map.Tiles[1][2] == "■") != raised || view.Map.Tiles[1][1] != "●" {
				t.Fatalf("9 shows %q with raised=%t err=%v", view.Map.Tiles[1][2], raised, err)
			}
			mid := f.reload(t, id)
			_, err = f.svc.Move(ctx, id, dungeon.DirectionEast)
			got := f.reload(t, id)
			if raised {
				if !errors.Is(err, dungeon.ErrImpassableWall) || got.Round != mid.Round || got.PosX != 9 {
					t.Fatalf("raised 9: err=%v round %d/%d x %d", err, got.Round, mid.Round, got.PosX)
				}
			} else if err != nil || got.PosX != 10 {
				t.Fatalf("lowered 9: err=%v x %d", err, got.PosX)
			}
		})
	}
}

// Observation shows the marks and hides the hidden stairs and the drawn floor.
func TestNativeMap17_Observation(t *testing.T) {
	ctx := context.Background()
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "view")
	f.moveTo(t, "view", 9, 13)
	view, err := f.svc.ObserveExpedition(ctx, "view")
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Map.Tiles; got[0][1] != "扉" || got[2][1] != "■" || got[1][2] != "□" {
		t.Fatalf("around z/2: %v", got)
	}
	f.moveTo(t, "view", 9, 3)
	if view, err = f.svc.ObserveExpedition(ctx, "view"); err != nil || view.Map.Tiles[2][1] != "◇" {
		t.Fatalf("switch 7 shows %v err=%v", view.Map.Tiles, err)
	}
	f.moveTo(t, "view", 4, 16)
	f.rng.ints = []int{1}
	if _, err := f.svc.Move(ctx, "view", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(f.reload(t, "view"))
	if err != nil || strings.Contains(string(raw), "under") {
		t.Fatalf("projection leaks map state %s err=%v", raw, err)
	}
}
