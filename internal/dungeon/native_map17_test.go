package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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
	f := newNativeFixtureOn(t, store, map17Hero(id))
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

// Map17 is listed for leader job level 40.
func TestNativeMap17_CatalogAndFloorLimits(t *testing.T) {
	hero := map17Hero("lead")
	hero.JobLevel = 39
	f := newNativeFixture(t, hero)
	for _, level := range []int{39, 40} {
		listed := slices.ContainsFunc(f.svc.ListDungeons(level), func(d dungeon.Dungeon) bool { return d.ID == dungeon.NativeMap17ID })
		if listed != (level == 40) {
			t.Fatalf("job level %d: listed=%t", level, listed)
		}
	}
	if _, err := f.svc.StartExpedition(context.Background(), "lead", dungeon.NativeMap17ID); !errors.Is(err, dungeon.ErrJobLevelRequired) {
		t.Fatalf("start at job level 39: %v", err)
	}
	d := mapDef(t, dungeon.NativeMap17ID)
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

// Underground floor _1 (variant 2), x across, y down: J (3,14) below j (3,13);
// K (5,4) west of k (6,4); V (16,6) above v (16,7); centre x (10–12,13), rest
// n (9,11) above (9,12), key Y (9,10).
func newMap17Party(t *testing.T) *nativeFixture {
	t.Helper()
	f := newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), map17Hero("lead"))
	f.chars.chars["ally"] = map17Hero("ally")
	f.startNativeParty(t, dungeon.NativeMap17ID, 1, 11, 16)
	return f
}

func TestNativeMap17_TreasurePairs(t *testing.T) {
	testMap14Stores(t, treasurePairs)
}

// The first of a pair is a treasure when rand(2) < 1, else the box; either way
// the visit counts, and the partner is then always a treasure.
func treasurePairs(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	for _, tc := range []struct {
		r    float64
		want dungeon.TileEventType
	}{{0.49, dungeon.EventTreasure}, {0.5, dungeon.EventBoss}} {
		f := newMap17Fixture(t, store, id)
		f.setFloor(t, id, 2, 3, 14)
		f.consume(t, id, "J")
		before := f.reload(t, id)
		f.rng.floats = []float64{tc.r}
		res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth)
		if err != nil || res.EventType != tc.want {
			t.Fatalf("j at r=%v: event %s err=%v", tc.r, res.EventType, err)
		}
		after := f.reload(t, id)
		if !slices.Contains(after.ConsumedEvents, "j") || tc.want == dungeon.EventBoss && after.Encounter.Monster.ID != "nm17-calamity-box" {
			t.Fatalf("j: consumed %v encounter %+v", after.ConsumedEvents, after.Encounter)
		}
		f.rejectStaleMap14Step(t, id, before, after)
		f.setFloor(t, id, 2, 5, 4)
		f.consume(t, id, "K")
		f.rng.floats = []float64{0.99}
		if res, err := f.svc.Move(ctx, id, dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventTreasure {
			t.Fatalf("partner k: event %s err=%v", res.EventType, err)
		}
		if _, err := f.svc.Escape(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
}

// J fights once; a spent ambush is a plain passage.
func TestNativeMap17_RobberAmbushOnce(t *testing.T) {
	ctx := context.Background()
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "rob")
	f.setFloor(t, "rob", 2, 3, 15)
	if res, err := f.svc.Move(ctx, "rob", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventBattle {
		t.Fatalf("J: event %s err=%v", res.EventType, err)
	}
	if exp := f.reload(t, "rob"); exp.Encounter == nil || exp.Encounter.Monster.ID != "nm17-robber-chief" || !slices.Contains(exp.ConsumedEvents, "J") {
		t.Fatalf("J encounter %+v consumed %v", exp.Encounter, exp.ConsumedEvents)
	}
	f.consume(t, "rob")
	f.setFloor(t, "rob", 2, 3, 15)
	if res, err := f.svc.Move(ctx, "rob", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove || f.reload(t, "rob").Encounter != nil {
		t.Fatalf("spent J: event %s err=%v", res.EventType, err)
	}
}

// v fights on every entry after its 1/2 trap draw; the trap lands before the
// fight, so a member it fells starts it fallen. V marks once. Both cases get
// the same combat draws, so the leader's losses differ by the trap alone.
func TestNativeMap17_CorridorAmbush(t *testing.T) {
	ctx := context.Background()
	var losses [2]int
	for i, floats := range [][]float64{{0.49, 0, 0}, {0.5}} {
		f := newMap17Party(t)
		f.setFloor(t, "lead", 2, 16, 5)
		if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth); err != nil {
			t.Fatal(err)
		}
		if exp := f.reload(t, "lead"); !slices.Contains(exp.ConsumedEvents, "V") {
			t.Fatalf("V not spent: %v", exp.ConsumedEvents)
		}
		before := memberOf(t, f.reload(t, "lead"), "lead").CurrentHP
		f.rng.floats = floats
		res, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth)
		if err != nil || res.EventType != dungeon.EventBoss {
			t.Fatalf("v: event %s err=%v", res.EventType, err)
		}
		if i == 0 {
			if len(res.MemberDamages) != 2 || res.MemberDamages["lead"] != 90 || res.MemberDamages["ally"] != 90 {
				t.Fatalf("v trap damages=%v, want lead:90 ally:90", res.MemberDamages)
			}
		} else if len(res.MemberDamages) != 0 {
			t.Fatalf("v skipped trap damages=%v, want empty", res.MemberDamages)
		}
		after := f.reload(t, "lead")
		if after.Encounter == nil || after.Encounter.Monster.ID != "nm17-mask" || slices.Contains(after.ConsumedEvents, "v") {
			t.Fatalf("v encounter %+v consumed %v", after.Encounter, after.ConsumedEvents)
		}
		losses[i] = before - memberOf(t, after, "lead").CurrentHP
		// v has no once-key: the next entry fights again, and its trap can
		// fell a weak member before the fight.
		f.consume(t, "lead")
		f.moveTo(t, "lead", 16, 6)
		f.setVitality(t, "lead", "ally", 50, 0)
		f.rng.floats = []float64{0.49, 0, 0}
		res, err = f.svc.Move(ctx, "lead", dungeon.DirectionSouth)
		if err != nil || res.EventType != dungeon.EventBoss {
			t.Fatalf("v again: event %s err=%v", res.EventType, err)
		}
		if len(res.MemberDamages) != 2 || res.MemberDamages["lead"] != 90 || res.MemberDamages["ally"] != 90 {
			t.Fatalf("v again trap damages=%v, want lead:90 ally:90", res.MemberDamages)
		}
		if memberOf(t, f.reload(t, "lead"), "ally").CurrentHP != 0 {
			t.Fatal("trap did not fell the ally")
		}
	}
	if losses[0]-losses[1] != 90 {
		t.Fatalf("leader losses %v: trap should add int(100 × 0.9)", losses)
	}
}

// x and X stay inert without the key; with it every x fights after its 1/4
// trap draw.
func TestNativeMap17_KeyArmsCentre(t *testing.T) {
	ctx := context.Background()
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "x")
	f.setFloor(t, "x", 2, 11, 13)
	res, err := f.svc.Move(ctx, "x", dungeon.DirectionEast)
	if err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 || len(res.MemberDamages) != 0 {
		t.Fatalf("x without key: event %s draws %d damages=%v err=%v", res.EventType, f.rng.calls(), res.MemberDamages, err)
	}
	var losses [2]int
	for i, floats := range [][]float64{{0.24, 0}, {0.25}} {
		f.consume(t, "x", "Y")
		f.moveTo(t, "x", 11, 13)
		before := f.reload(t, "x").CurrentHP
		f.rng.floats = floats
		res, err := f.svc.Move(ctx, "x", dungeon.DirectionEast)
		if err != nil || res.EventType != dungeon.EventBoss {
			t.Fatalf("x with key: event %s err=%v", res.EventType, err)
		}
		if i == 0 {
			if len(res.MemberDamages) != 1 || res.MemberDamages["x"] != 90 {
				t.Fatalf("x trap damages=%v, want x:90", res.MemberDamages)
			}
		} else if len(res.MemberDamages) != 0 {
			t.Fatalf("x skipped trap damages=%v, want empty", res.MemberDamages)
		}
		after := f.reload(t, "x")
		if after.Encounter == nil || after.Encounter.Monster.ID != "nm17-great-mask" {
			t.Fatalf("x encounter %+v", after.Encounter)
		}
		losses[i] = before - after.CurrentHP
		if i == 0 {
			fightRes, err := f.svc.Fight(ctx, "x")
			if err != nil || len(fightRes.MemberDamages) != 0 {
				t.Fatalf("fight after trap: damages=%v err=%v", fightRes.MemberDamages, err)
			}
		}
	}
	if losses[0]-losses[1] != 90 {
		t.Fatalf("losses %v: trap should add int(100 × 0.9)", losses)
	}

	// Trap damage is also reported when the ambush battle ends in victory.
	hero := map17Hero("win")
	hero.Stats.Attack, hero.Stats.Agility = 10_000, 10_000
	fw := newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), hero)
	if _, err := fw.svc.StartExpedition(ctx, "win", dungeon.NativeMap17ID); err != nil {
		t.Fatal(err)
	}
	fw.consume(t, "win", "Y")
	fw.setFloor(t, "win", 2, 11, 13)
	fw.rng.floats = []float64{0.24, 0}
	winRes, err := fw.svc.Move(ctx, "win", dungeon.DirectionEast)
	if err != nil || winRes.CombatOutcome != corebattle.OutcomeWin || len(winRes.MemberDamages) != 1 || winRes.MemberDamages["win"] != 90 {
		t.Fatalf("win ambush: outcome=%s damages=%v err=%v", winRes.CombatOutcome, winRes.MemberDamages, err)
	}
}

// An exact-lethal ambush trap persists zero HP and the untouched boss so
// Escape remains available, while a survivor enters combat at post-trap HP.
func TestNativeMap17_AllFallenTrapPersistsAmbush(t *testing.T) {
	testMap14Stores(t, allFallenTrapPersistsAmbush)

	// Party v ambush where the last living member falls to the trap also stays exploring.
	ctx := context.Background()
	party := newMap17Party(t)
	party.setFloor(t, "lead", 2, 16, 6)
	party.setVitality(t, "lead", "lead", 90, 0)
	party.setVitality(t, "lead", "ally", 0, 0)
	party.rng.floats = []float64{0.49, 0, 0}
	partyRes, err := party.svc.Move(ctx, "lead", dungeon.DirectionSouth)
	if err != nil || partyRes.IsFinished || partyRes.CombatOutcome != corebattle.OutcomeDefeat ||
		len(partyRes.MemberDamages) != 2 || partyRes.MemberDamages["lead"] != 90 || partyRes.MemberDamages["ally"] != 90 {
		t.Fatalf("party v lethal trap: res=%+v err=%v", partyRes, err)
	}
	partyAfter := party.reload(t, "lead")
	if partyAfter.Status != dungeon.StatusExploring || partyAfter.CurrentHP != 0 ||
		partyAfter.Encounter == nil || partyAfter.Encounter.Monster.ID != "nm17-mask" || partyAfter.Encounter.Enemies[0].HP != 800 {
		t.Fatalf("party v after: %+v", partyAfter)
	}
	if _, err := party.svc.Escape(ctx, "lead"); err != nil {
		t.Fatal(err)
	}
}

func allFallenTrapPersistsAmbush(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newMap17Fixture(t, store, id)
	f.consume(t, id, "Y")
	f.setFloor(t, id, 2, 11, 13)
	exp := f.reload(t, id)
	exp.AccumulatedExp, exp.AccumulatedGold = 7, 9
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.setVitality(t, id, id, 90, 0)
	before := f.reload(t, id)

	f.rng.floats = []float64{0.24, 0}
	res, err := f.svc.Move(ctx, id, dungeon.DirectionEast)
	if err != nil || res.IsFinished || res.EventType != dungeon.EventBoss || res.CombatOutcome != corebattle.OutcomeDefeat {
		t.Fatalf("lethal ambush: res=%+v err=%v", res, err)
	}
	if len(res.MemberDamages) != 1 || res.MemberDamages[id] != 90 {
		t.Fatalf("lethal ambush damages=%v, want %s:90", res.MemberDamages, id)
	}

	after := f.reload(t, id)
	if after.Status != dungeon.StatusExploring || after.PosX != 12 || after.PosY != 13 || after.Round != before.Round+1 ||
		after.CurrentHP != 0 || memberOf(t, after, id).CurrentHP != 0 ||
		after.AccumulatedExp != 7 || after.AccumulatedGold != 9 || len(memberOf(t, after, id).ConsumedItems) != 0 {
		t.Fatalf("after lethal ambush: %+v", after)
	}
	if after.Encounter == nil || after.Encounter.Monster.ID != "nm17-great-mask" ||
		len(after.Encounter.Enemies) != 1 || after.Encounter.Enemies[0].HP != 1300 || after.Encounter.Enemies[0].MaxHP != 1300 {
		t.Fatalf("after encounter: %+v", after.Encounter)
	}
	f.rejectStaleMap14Step(t, id, before, after)
	if _, err := f.svc.Fight(ctx, id); !errors.Is(err, dungeon.ErrActorFallen) {
		t.Fatalf("fight when all fallen: err=%v", err)
	}
	esc, err := f.svc.Escape(ctx, id)
	h := f.repo.histories[id]
	if err != nil || esc.Expedition.Status != dungeon.StatusEscaped || len(h) != 1 || h[0].ExpReward != 7 || h[0].GoldReward != 9 {
		t.Fatalf("escape after lethal ambush: %+v err=%v history=%+v", esc, err, h)
	}

	// Surviving with 1 HP reaches the resolver at post-trap HP.
	surv := newMap17Fixture(t, store, id)
	surv.consume(t, id, "Y")
	surv.setFloor(t, id, 2, 11, 13)
	surv.setVitality(t, id, id, 91, 0)
	surv.rng.floats = []float64{0.24, 0}
	survRes, err := surv.svc.Move(ctx, id, dungeon.DirectionEast)
	if err != nil || survRes.BattleResult == nil || survRes.BattleResult.Turns == 0 || len(survRes.MemberDamages) != 1 || survRes.MemberDamages[id] != 90 {
		t.Fatalf("survivor ambush: res=%+v err=%v", survRes, err)
	}
	if got := surv.reload(t, id); got.PosX != 12 || got.PosY != 13 || got.Encounter == nil {
		t.Fatalf("survivor after: %+v", got)
	}
	if _, err := surv.svc.Escape(ctx, id); err != nil {
		t.Fatal(err)
	}
}

// The key opens main-floor z on every entry; without it z returns the party
// and spends the round.
func TestNativeMap17_KeyDoor(t *testing.T) {
	ctx := context.Background()
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "z")
	f.moveTo(t, "z", 9, 13)
	before := f.reload(t, "z")
	if _, err := f.svc.Move(ctx, "z", dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	if got := f.reload(t, "z"); got.PosY != 13 || got.Round != before.Round+1 {
		t.Fatalf("locked z: y %d round %d/%d", got.PosY, got.Round, before.Round)
	}
	// Y is picked up underground, then the door passes every time.
	f.setFloor(t, "z", 2, 9, 9)
	if _, err := f.svc.Move(ctx, "z", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if got := f.reload(t, "z"); !slices.Contains(got.ConsumedEvents, "Y") {
		t.Fatalf("Y not taken: %v", got.ConsumedEvents)
	}
	f.setFloor(t, "z", 1, 9, 13)
	for range 2 {
		if _, err := f.svc.Move(ctx, "z", dungeon.DirectionNorth); err != nil {
			t.Fatal(err)
		}
		if got := f.reload(t, "z"); got.PosY != 12 {
			t.Fatalf("keyed z: y %d", got.PosY)
		}
		f.moveTo(t, "z", 9, 13)
	}
}

// n heals once: MP goes to living members only, by the 0.9–1.2 roll, capped.
func TestNativeMap17_Rest(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		leadMP, want int
	}{{100, 339}, {800, 900}} {
		f := newMap17Party(t)
		f.setFloor(t, "lead", 2, 9, 12)
		f.setVitality(t, "lead", "lead", 1_000_000, tc.leadMP)
		f.setVitality(t, "lead", "ally", 0, 10)
		if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionNorth); err != nil {
			t.Fatal(err)
		}
		exp := f.reload(t, "lead")
		if lead, ally := memberOf(t, exp, "lead"), memberOf(t, exp, "ally"); lead.CurrentMP != tc.want || ally.CurrentMP != 10 || !slices.Contains(exp.ConsumedEvents, "n") {
			t.Fatalf("rest: lead MP %d (want %d) ally MP %d consumed %v", lead.CurrentMP, tc.want, ally.CurrentMP, exp.ConsumedEvents)
		}
		f.moveTo(t, "lead", 9, 12)
		f.setVitality(t, "lead", "lead", 1_000_000, 0)
		if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionNorth); err != nil || memberOf(t, f.reload(t, "lead"), "lead").CurrentMP != 0 {
			t.Fatalf("spent rest healed again: %v", err)
		}
	}
}

// Underground marks; ambush cells look like passages.
func TestNativeMap17_UndergroundMarks(t *testing.T) {
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "marks")
	f.setFloor(t, "marks", 2, 16, 5)
	view, err := f.svc.ObserveExpedition(context.Background(), "marks")
	if err != nil || view.Map.Tiles[2][1] != "◇" {
		t.Fatalf("V %v err=%v", view.Map.Tiles, err)
	}
	f.setFloor(t, "marks", 2, 11, 13)
	if view, err = f.svc.ObserveExpedition(context.Background(), "marks"); err != nil || view.Map.Tiles[1][0] != "□" || view.Map.Tiles[1][2] != "□" {
		t.Fatalf("x cells %v err=%v", view.Map.Tiles, err)
	}
	f.setFloor(t, "marks", 2, 9, 12)
	if view, err = f.svc.ObserveExpedition(context.Background(), "marks"); err != nil || view.Map.Tiles[0][1] != "☆" {
		t.Fatalf("n %v err=%v", view.Map.Tiles, err)
	}
}

// Hidden stairs 2 lead to the closet on every entry; its U returns to (9,14)
// and its rest N heals once.
func TestNativeMap17_HiddenStairs(t *testing.T) {
	ctx := context.Background()
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "two")
	for range 2 {
		f.moveTo(t, "two", 9, 13)
		if _, err := f.svc.Move(ctx, "two", dungeon.DirectionSouth); err != nil {
			t.Fatal(err)
		}
		if exp := f.reload(t, "two"); exp.MapVariant != 5 || exp.PosX != 9 || exp.PosY != 14 || exp.RoundLimit != 200 {
			t.Fatalf("2: variant %d at (%d,%d)", exp.MapVariant, exp.PosX, exp.PosY)
		}
		for _, dir := range []dungeon.Direction{dungeon.DirectionSouth, dungeon.DirectionSouth, dungeon.DirectionNorth, dungeon.DirectionNorth} {
			if _, err := f.svc.Move(ctx, "two", dir); err != nil {
				t.Fatal(err)
			}
		}
		if exp := f.reload(t, "two"); exp.MapVariant != 1 || exp.PosX != 9 || exp.PosY != 14 || !slices.Contains(exp.ConsumedEvents, "N") {
			t.Fatalf("closet U: variant %d at (%d,%d) consumed %v", exp.MapVariant, exp.PosX, exp.PosY, exp.ConsumedEvents)
		}
	}
}

func TestNativeMap17_Ascent(t *testing.T) {
	testMap14Stores(t, ascent)
}

// Z fights once; w does nothing before it, then saves its boss and the move
// to the secret room in one step. The boss keeps max HP 12000 over HP 6000.
func ascent(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newMap17Fixture(t, store, id)
	f.moveTo(t, id, 10, 9)
	if _, err := f.svc.Move(ctx, id, dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, id); exp.MapVariant != 1 || exp.Encounter != nil {
		t.Fatalf("w before Z: variant %d encounter %+v", exp.MapVariant, exp.Encounter)
	}
	f.moveTo(t, id, 9, 11)
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventBoss {
		t.Fatalf("Z: event %s err=%v", res.EventType, err)
	}
	if exp := f.reload(t, id); exp.Encounter.Monster.ID != "nm17-pharaoh" || !slices.Contains(exp.ConsumedEvents, "Z") {
		t.Fatalf("Z encounter %+v consumed %v", exp.Encounter, exp.ConsumedEvents)
	}
	f.consume(t, id)
	f.moveTo(t, id, 10, 9)
	if view, err := f.svc.ObserveExpedition(ctx, id); err != nil || view.Map.Tiles[1][0] != "凸" {
		t.Fatalf("w after Z shows %v err=%v", view.Map.Tiles, err)
	}
	before := f.reload(t, id)
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventBoss {
		t.Fatalf("w: event %s err=%v", res.EventType, err)
	}
	after := f.reload(t, id)
	enc := after.Encounter
	if after.MapVariant != 5 || after.PosX != 9 || after.PosY != 9 || after.RoundLimit != 200 || enc == nil || enc.Monster.ID != "nm17-necro-pharaoh" ||
		enc.Enemies[0].MaxHP != 12000 || enc.Enemies[0].HP > 6000 {
		t.Fatalf("w: variant %d at (%d,%d) encounter %+v", after.MapVariant, after.PosX, after.PosY, enc)
	}
	f.rejectStaleMap14Step(t, id, before, after)
}

// Secret-room treasures by visited j/k: L opens at 1; M shows at 1 but opens
// at 2; Q and O show at 2 and never open. D holds fixed chests.
func TestNativeMap17_VaultThresholds(t *testing.T) {
	ctx := context.Background()
	cells := []struct {
		name     byte
		from     [2]int
		dir      dungeon.Direction
		open     int
		viewFrom [2]int
		row, col int
	}{
		{'L', [2]int{9, 9}, dungeon.DirectionWest, 1, [2]int{9, 9}, 1, 0},
		{'M', [2]int{10, 10}, dungeon.DirectionEast, 2, [2]int{10, 10}, 1, 2},
		{'Q', [2]int{9, 10}, dungeon.DirectionSouth, 3, [2]int{10, 10}, 2, 0},
		{'O', [2]int{10, 9}, dungeon.DirectionNorth, 4, [2]int{9, 9}, 0, 2},
	}
	shows := map[byte]int{'L': 1, 'M': 1, 'Q': 2, 'O': 2}
	for visits, keys := range [][]string{nil, {"j"}, {"j", "k"}} {
		for _, c := range cells {
			f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "vault")
			f.consume(t, "vault", keys...)
			f.setFloor(t, "vault", 5, c.viewFrom[0], c.viewFrom[1])
			view, err := f.svc.ObserveExpedition(ctx, "vault")
			if err != nil || (view.Map.Tiles[c.row][c.col] == "宝") != (visits >= shows[c.name]) {
				t.Fatalf("%c at %d visits shows %q err=%v", c.name, visits, view.Map.Tiles[c.row][c.col], err)
			}
			f.moveTo(t, "vault", c.from[0], c.from[1])
			res, err := f.svc.Move(ctx, "vault", c.dir)
			opened := err == nil && res.EventType == dungeon.EventTreasure
			spent := slices.Contains(f.reload(t, "vault").ConsumedEvents, string(c.name))
			if err != nil || opened != (visits >= c.open) || spent != opened {
				t.Fatalf("%c at %d visits: event %s spent %t err=%v", c.name, visits, res.EventType, spent, err)
			}
		}
	}
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "vault")
	f.setFloor(t, "vault", 5, 9, 8)
	if res, err := f.svc.Move(ctx, "vault", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventTreasure || len(f.reload(t, "vault").Encounter.Chests) != 1 {
		t.Fatalf("D: event %s err=%v", res.EventType, err)
	}
}

func TestNativeMap17_FloorNames(t *testing.T) {
	ctx := context.Background()
	f := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "title")
	for _, tc := range []struct {
		variant int
		want    string
	}{
		{1, "王墓"},
		{2, "王墓"},
		{5, "王墓の秘室"},
	} {
		f.setFloor(t, "title", tc.variant, 9, 9)
		view, err := f.svc.ObserveExpedition(ctx, "title")
		if err != nil || view.FloorName != tc.want {
			t.Fatalf("variant %d floor name %+v err=%v", tc.variant, view, err)
		}
	}
}
