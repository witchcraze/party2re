package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// Main floor (map/13/1.cgi), x across, y down: channels R (2,2), W (6,2),
// B (2,6), Y (6,6); reroll C (4,4); altar A (4,6) above the start S (4,7).
// Channel bits: B 8, Y 4, R 2, W 1; a target draw i is the mask i+1.
func map13Hero(id string) corecharacter.Character {
	c := createTestChar(id, 90, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID = 80, "job-01"
	return c
}

func newMap13Fixture(t *testing.T, store dungeon.ActiveExpeditionStore, id string) *nativeFixture {
	t.Helper()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap13())
	f := newNativeFixtureOn(t, store, map13Hero(id), dungeon.WithCustomDungeons(catalog))
	if _, err := f.svc.StartExpedition(context.Background(), id, dungeon.NativeMap13ID); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.floats, f.rng.intCalls, f.rng.floatCalls = nil, nil, 0, 0
	return f
}

type map13StateView struct {
	Channels  int  `json:"channels"`
	Target    int  `json:"target"`
	Activated bool `json:"activated"`
	Disclosed int  `json:"disclosed"`
	Rerolled  bool `json:"rerolled"`
}

func map13StateOf(t *testing.T, exp dungeon.ActiveExpedition) (st map13StateView) {
	t.Helper()
	if len(exp.MapState) > 0 {
		if err := json.Unmarshal(exp.MapState, &st); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// enter moves from the approach cell into a main-floor event cell.
func (f *nativeFixture) enterMap13(t *testing.T, id string, from [2]int, dir dungeon.Direction) dungeon.ActiveExpedition {
	t.Helper()
	f.moveTo(t, id, from[0], from[1])
	if res, err := f.svc.Move(context.Background(), id, dir); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("enter from %v: event %s err=%v", from, res.EventType, err)
	}
	return f.reload(t, id)
}

var (
	map13FromB     = [2]int{3, 6}
	map13FromC     = [2]int{3, 4}
	map13FromAltar = [2]int{4, 7}
)

func TestNativeMap13_UnlistedAndLayouts(t *testing.T) {
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap13ID {
			t.Fatal("map13 listed before its destination events")
		}
	}
	f := newNativeFixture(t, map13Hero("unlisted"))
	if _, err := f.svc.StartExpedition(context.Background(), "unlisted", dungeon.NativeMap13ID); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Fatalf("start err=%v", err)
	}
	if exp, err := f.svc.GetActiveExpedition(context.Background(), "unlisted"); err != nil || exp != nil {
		t.Fatalf("rejected start created %+v err=%v", exp, err)
	}

	d := dungeon.NativeMap13()
	if d.RoundLimit != 80 || d.LeaderJobLevel != 80 || d.Initial != 1 || len(d.Variants) != 3 {
		t.Fatalf("map13 %+v", d)
	}
	legacy := [3]string{
		// 1.cgi:9–17
		"1,1,1,1,1,1,1,1,1|1,1,1,1,0,1,1,1,1|1,1,R,0,0,0,W,1,1|1,1,0,0,0,0,0,1,1|1,0,0,0,C,0,0,0,1|1,1,0,0,0,0,0,1,1|1,1,B,0,A,0,Y,1,1|1,1,1,1,S,1,1,1,1|1,1,1,1,1,1,1,1,1",
		// _1.cgi:9–17
		"1,1,1,1,h,1,1,1,1|1,1,1,f,e,g,1,1,1|1,1,1,c,d,b,1,1,1|1,1,1,1,V,1,1,1,1|1,1,1,1,Q,1,1,1,1|1,1,1,1,Z,1,1,1,1|1,1,1,1,0,1,1,1,1|1,1,1,1,1,1,1,1,1|1,1,1,1,1,1,1,1,1",
		// _EX.cgi:9–17
		"1,1,1,1,h,1,1,1,1|1,1,1,f,e,g,1,1,1|1,1,1,c,d,b,1,1,1|1,1,1,1,V,1,1,1,1|1,1,1,1,Q,1,1,1,1|1,1,1,1,0,1,1,1,1|1,1,1,1,0,1,1,1,1|1,1,1,1,1,1,1,1,1|1,1,1,1,1,1,1,1,1",
	}
	for i, fl := range d.Variants {
		if got := strings.Join(fl.Grid, "|"); got != strings.ReplaceAll(legacy[i], ",", "") {
			t.Fatalf("variant %d grid %s", i+1, got)
		}
		if fl.Width != 9 || fl.Height != 9 || d.Variants[i].RoundLimit != 0 {
			t.Fatalf("variant %d geometry %+v", i+1, fl)
		}
		if i > 0 && fl.Grid[6][4] != '0' {
			t.Fatalf("altar coordinate is not a passage on variant %d", i+1)
		}
	}
	if main := d.Variants[0]; main.StartX != 4 || main.StartY != 7 {
		t.Fatalf("start (%d,%d)", main.StartX, main.StartY)
	}
}

// Channels toggle with no draw and no once flag; observation shows the
// active ones by color and draws nothing.
func TestNativeMap13_ChannelsToggle(t *testing.T) {
	ctx := context.Background()
	f := newMap13Fixture(t, memStore(), "toggle")
	tile := func() string {
		t.Helper()
		v, err := f.svc.ObserveExpedition(ctx, "toggle")
		if err != nil || v.Map == nil || v.ChannelTarget != nil {
			t.Fatalf("observe %+v err=%v", v, err)
		}
		return v.Map.Tiles[1][0] // B west of (3,6)
	}
	for i, want := range []int{8, 0, 8} {
		exp := f.enterMap13(t, "toggle", map13FromB, dungeon.DirectionWest)
		if st := map13StateOf(t, exp); st.Channels != want || len(exp.ConsumedEvents) != 0 || f.rng.calls() != 0 {
			t.Fatalf("entry %d: state %+v, flags %v, draws %d", i+1, st, exp.ConsumedEvents, f.rng.calls())
		}
		f.moveTo(t, "toggle", map13FromB[0], map13FromB[1])
		wantTile := "☆"
		if want != 0 {
			wantTile = "青"
		}
		if got := tile(); got != wantTile || f.rng.calls() != 0 {
			t.Fatalf("entry %d: B shows %q, draws %d", i+1, got, f.rng.calls())
		}
	}
	f.moveTo(t, "toggle", 4, 5)
	v, err := f.svc.ObserveExpedition(ctx, "toggle")
	if err != nil || v.Map.Tiles[0][1] != "灰" || v.Map.Tiles[2][1] != "★" {
		t.Fatalf("reroll/altar marks %v err=%v", v.Map.Tiles, err)
	}
}

func memStore() dungeon.ActiveExpeditionStore { return dungeon.NewMemoryExpeditionRepository() }

func newMap13Party(t *testing.T, x, y int) *nativeFixture {
	t.Helper()
	f := newNativeFixtureOn(t, memStore(), map13Hero("lead"), dungeon.WithCustomDungeons(append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap13())))
	f.chars.chars["ally"] = map13Hero("ally")
	f.startNativeParty(t, dungeon.NativeMap13ID, 1, x, y)
	return f
}

// The first altar visit draws at both bounds and discloses on mismatch; later
// visits never redraw; a reroll always redraws and flags a disclosed target.
func TestNativeMap13_AltarAndReroll(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		draw, target int
	}{{0, 1}, {14, 15}} {
		f := newMap13Fixture(t, memStore(), "altar")
		f.rng.ints = []int{tc.draw}
		exp := f.enterMap13(t, "altar", map13FromAltar, dungeon.DirectionNorth)
		if st := map13StateOf(t, exp); st != (map13StateView{Target: tc.target, Activated: true, Disclosed: tc.target}) || exp.MapVariant != 1 || f.rng.intCalls != 1 {
			t.Fatalf("draw %d: state %+v variant %d draws %d", tc.draw, st, exp.MapVariant, f.rng.intCalls)
		}
		f.rng.ints, f.rng.intCalls = []int{3}, 0
		if exp = f.enterMap13(t, "altar", map13FromAltar, dungeon.DirectionNorth); map13StateOf(t, exp).Target != tc.target || f.rng.intCalls != 0 {
			t.Fatalf("revisit redrew: %+v", map13StateOf(t, exp))
		}
	}

	// A reroll before activation is superseded by the altar's first draw.
	f := newMap13Fixture(t, memStore(), "reroll")
	f.rng.ints = []int{4}
	exp := f.enterMap13(t, "reroll", map13FromC, dungeon.DirectionEast)
	if st := map13StateOf(t, exp); st != (map13StateView{Target: 5}) || f.rng.intCalls != 1 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("pre-activation reroll %+v", st)
	}
	if v, err := f.svc.ObserveExpedition(ctx, "reroll"); err != nil || v.ChannelTarget != nil {
		t.Fatalf("undisclosed target observed: %+v err=%v", v.ChannelTarget, err)
	}
	f.rng.ints, f.rng.intCalls = []int{9}, 0
	exp = f.enterMap13(t, "reroll", map13FromAltar, dungeon.DirectionNorth)
	if st := map13StateOf(t, exp); st != (map13StateView{Target: 10, Activated: true, Disclosed: 10}) {
		t.Fatalf("altar after reroll %+v", st)
	}
	f.rng.ints = []int{1}
	exp = f.enterMap13(t, "reroll", map13FromC, dungeon.DirectionEast)
	if st := map13StateOf(t, exp); st != (map13StateView{Target: 2, Activated: true, Disclosed: 10, Rerolled: true}) {
		t.Fatalf("post-disclosure reroll %+v", st)
	}
	f.rng.ints = nil
	exp = f.enterMap13(t, "reroll", map13FromAltar, dungeon.DirectionNorth)
	if st := map13StateOf(t, exp); st != (map13StateView{Target: 2, Activated: true, Disclosed: 2}) {
		t.Fatalf("second disclosure %+v", st)
	}
}

// Every member observes the same disclosure, and observation never mutates.
func TestNativeMap13_DisclosureSharedByMembers(t *testing.T) {
	ctx := context.Background()
	f := newMap13Party(t, map13FromAltar[0], map13FromAltar[1])
	f.rng.ints = []int{9} // B+R: 1010
	f.enterMap13(t, "lead", map13FromAltar, dungeon.DirectionNorth)
	f.rng.ints = []int{0}
	before := f.enterMap13(t, "lead", map13FromC, dungeon.DirectionEast)
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	for _, id := range []string{"lead", "ally"} {
		v, err := f.svc.ObserveExpedition(ctx, id)
		if err != nil || v.ChannelTarget == nil || !slices.Equal(v.ChannelTarget.Channels, []string{"blue", "red"}) || !v.ChannelTarget.RerolledSince {
			t.Fatalf("%s observes %+v err=%v", id, v.ChannelTarget, err)
		}
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), "white") {
			t.Fatalf("%s observation leaks the rerolled target: %s", id, raw)
		}
	}
	if after := f.reload(t, "lead"); after.Revision != before.Revision || f.rng.calls() != 0 {
		t.Fatalf("observation mutated: revision %d→%d, draws %d", before.Revision, after.Revision, f.rng.calls())
	}
}

// _is_EX: solo k = 1..3 normal without a draw and k = 4 EX; rosters of 2–4
// take EX when 5r ≤ k; n ≥ 5 always EX.
func TestNativeMap13_RouteBoundaries(t *testing.T) {
	const normal, ex = 2, 3
	for k := 1; k <= 4; k++ {
		rng := &scriptedRNG{}
		want := normal
		if k == 4 {
			want = ex
		}
		if got := dungeon.Map13Route(k, 1, rng); got != want || rng.calls() != 0 {
			t.Fatalf("solo k=%d: route %d, draws %d", k, got, rng.calls())
		}
		if got := dungeon.Map13Route(k, 5, rng); got != ex || rng.calls() != 0 {
			t.Fatalf("n=5 k=%d: route %d, draws %d", k, got, rng.calls())
		}
	}
	for n := 2; n <= 4; n++ {
		for k := 1; k <= 3; k++ {
			at := float64(k) / 5
			for _, tc := range []struct {
				r    float64
				want int
			}{{at, ex}, {math.Nextafter(at, 1), normal}} {
				rng := &scriptedRNG{floats: []float64{tc.r}}
				if got := dungeon.Map13Route(k, n, rng); got != tc.want || rng.floatCalls != 1 {
					t.Fatalf("n=%d k=%d r=%v: route %d, draws %d", n, k, tc.r, got, rng.floatCalls)
				}
			}
		}
		rng := &scriptedRNG{}
		if got := dungeon.Map13Route(4, n, rng); got != ex || rng.calls() != 0 {
			t.Fatalf("n=%d k=4: route %d, draws %d", n, got, rng.calls())
		}
	}
}

// A match transfers at the altar coordinate, spending its round under the
// same limit; stale competing steps never replace the accepted route.
func TestNativeMap13_TransferCarry(t *testing.T) {
	testMap14Stores(t, func(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
		f := newMap13Fixture(t, store, id)
		f.enterMap13(t, id, map13FromB, dungeon.DirectionWest) // B on: target 8 is draw 7
		exp := f.reload(t, id)
		exp.Round = 78
		if err := store.SaveActiveExpedition(context.Background(), exp); err != nil {
			t.Fatal(err)
		}
		f.moveTo(t, id, map13FromAltar[0], map13FromAltar[1])
		before := f.reload(t, id)
		f.rng.ints = []int{7}
		after := f.enterMap13(t, id, map13FromAltar, dungeon.DirectionNorth)
		if after.MapVariant != 2 || after.PosX != 4 || after.PosY != 6 || after.Round != 79 || after.RoundLimit != 80 || f.rng.calls() != 1 {
			t.Fatalf("transfer %+v, draws %d", after, f.rng.calls())
		}
		if st := map13StateOf(t, after); st != (map13StateView{Channels: 8, Target: 8, Activated: true}) {
			t.Fatalf("transferred state %+v", st)
		}
		if v, err := f.svc.ObserveExpedition(context.Background(), id); err != nil || v.Floor != 2 || v.FloorName != "幻魔の広間" || v.ChannelTarget != nil {
			t.Fatalf("destination view %+v err=%v", v, err)
		}
		f.rejectStaleMap14Step(t, id, before, after)

		// A destination event cell is a passage until its unit lands.
		f.rng.intCalls, f.rng.floatCalls = 0, 0
		if res, err := f.svc.Move(context.Background(), id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
			t.Fatalf("Z cell: %+v draws %d err=%v", res.EventType, f.rng.calls(), err)
		}
		exhausted := f.reload(t, id)
		if exhausted.Round != 80 || len(exhausted.ConsumedEvents) != 0 {
			t.Fatalf("round %d flags %v", exhausted.Round, exhausted.ConsumedEvents)
		}
		f.rng.intCalls, f.rng.floatCalls = 0, 0
		if _, err := f.svc.Move(context.Background(), id, dungeon.DirectionSouth); !errors.Is(err, dungeon.ErrMovementExhausted) || f.rng.calls() != 0 {
			t.Fatalf("round 80 move err=%v draws %d", err, f.rng.calls())
		}
		if got := f.reload(t, id); got.Revision != exhausted.Revision || !got.Controls(id).CanEscape {
			t.Fatalf("exhausted run changed or lost Escape: %+v", got)
		}
	})
}

// A two-member match draws the route: 5r ≤ k is EX.
func TestNativeMap13_PartyMatchDrawsRoute(t *testing.T) {
	for _, tc := range []struct {
		r    float64
		want int
	}{{0.2, 3}, {math.Nextafter(0.2, 1), 2}} {
		f := newMap13Party(t, map13FromB[0], map13FromB[1])
		f.enterMap13(t, "lead", map13FromB, dungeon.DirectionWest)
		f.moveTo(t, "lead", map13FromAltar[0], map13FromAltar[1])
		f.rng.ints, f.rng.floats = []int{7}, []float64{tc.r}
		if exp := f.enterMap13(t, "lead", map13FromAltar, dungeon.DirectionNorth); exp.MapVariant != tc.want || f.rng.intCalls != 1 || f.rng.floatCalls != 1 {
			t.Fatalf("r=%v: variant %d, draws %d/%d", tc.r, exp.MapVariant, f.rng.intCalls, f.rng.floatCalls)
		}
	}
}

// Passages spawn from each floor's pool: _data.cgi's weights on the main
// floor and _data_EX.cgi's single monster on EX.
func TestNativeMap13_PassagePools(t *testing.T) {
	ctx := context.Background()
	f := newMap13Fixture(t, memStore(), "pool")
	f.moveTo(t, "pool", 4, 5)
	f.rng.floats, f.rng.ints = []float64{0.5, 0}, []int{20}
	res, err := f.svc.Move(ctx, "pool", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventBattle || !slices.Equal(monsterIDs(res.Monsters), []string{"nm13-crocter"}) {
		t.Fatalf("main passage %+v err=%v", res.Monsters, err)
	}
	f.setFloor(t, "pool", 3, 4, 6)
	exp := f.reload(t, "pool")
	exp.Encounter = nil
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.floats, f.rng.ints = []float64{0.5, 0}, nil
	res, err = f.svc.Move(ctx, "pool", dungeon.DirectionNorth)
	if err != nil || res.EventType != dungeon.EventBattle || !slices.Equal(monsterIDs(res.Monsters), []string{"nm13-phantom-envoy"}) {
		t.Fatalf("EX passage %+v err=%v", res.Monsters, err)
	}
}
