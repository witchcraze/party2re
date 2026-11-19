package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
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

func newMap14Fixture(t *testing.T, store dungeon.ActiveExpeditionStore, hero corecharacter.Character) *nativeFixture {
	t.Helper()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap14())
	return newNativeFixtureOn(t, store, hero, dungeon.WithCustomDungeons(catalog))
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

func TestNativeMap14_UnlistedUntilTreasureShip(t *testing.T) {
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap14ID {
			t.Fatal("map14 listed before #2432")
		}
	}
	f := newNativeFixture(t, map14Hero("unlisted"))
	if _, err := f.svc.StartExpedition(context.Background(), "unlisted", dungeon.NativeMap14ID); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Fatalf("start err=%v", err)
	}
	d := dungeon.NativeMap14()
	if d.RoundLimit != 200 || d.LeaderJobLevel != 30 || len(d.Variants) != 4 || d.Initial != 1 {
		t.Fatalf("map14 %+v", d)
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
		f := newMap14Fixture(t, dungeon.NewMemoryExpeditionRepository(), map14Hero("allot"))
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
	t.Run("memory", func(t *testing.T) {
		pointsRoute(t, dungeon.NewMemoryExpeditionRepository(), "route")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map14-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		pointsRoute(t, store, id)
	})
}

func pointsRoute(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newMap14Fixture(t, store, map14Hero(id))
	f.startMap14(t, id, 0, 0, 0) // A, B, C active
	enter := func(x, y int, dir dungeon.Direction, draw []int, wantVariant, wantDraws int) dungeon.ActiveExpedition {
		t.Helper()
		f.setFloor(t, id, 1, x, y)
		before := f.reload(t, id)
		f.rng.ints = draw
		if res, err := f.svc.Move(ctx, id, dir); err != nil || res.EventType != dungeon.EventMove {
			t.Fatalf("enter %+v err=%v", res, err)
		}
		exp := f.reload(t, id)
		if exp.MapVariant != wantVariant || exp.Round != before.Round+1 || f.rng.intCalls != wantDraws || f.rng.floatCalls != 0 {
			t.Fatalf("enter from (%d,%d): variant %d round %d draws %d/%d", x, y, exp.MapVariant, exp.Round, f.rng.intCalls, f.rng.floatCalls)
		}
		rev := before.Revision
		if _, err := store.Step(ctx, id, dungeon.StepParams{ExpectedExpeditionID: before.ID, ExpectedRevision: &rev,
			NewX: x, NewY: y, RoundDelta: 1, Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
			t.Fatalf("stale step err=%v", err)
		}
		return exp
	}
	// A: draw 1 of free [_P _K _T] → _K (3), keeping (8,8).
	if exp := enter(7, 8, dungeon.DirectionEast, []int{1}, 3, 1); exp.PosX != 8 || exp.PosY != 8 {
		t.Fatalf("A at (%d,%d)", exp.PosX, exp.PosY)
	}
	// Back through _K's R at (3,4): main floor, same coordinate, round carried.
	f.setFloor(t, id, 3, 3, 5)
	before := f.reload(t, id)
	if _, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, id); exp.MapVariant != 1 || exp.PosX != 3 || exp.PosY != 4 || exp.Round != before.Round+1 {
		t.Fatalf("return %+v", exp)
	}
	// B: draw 1 of free [_P _T] → _T (4); C: the last type _P (2).
	enter(2, 1, dungeon.DirectionWest, []int{1}, 4, 1)
	enter(2, 8, dungeon.DirectionWest, []int{0}, 2, 1)
	// A again: its destination, no draw.
	enter(7, 8, dungeon.DirectionEast, nil, 3, 0)
}

// An inactive point is a passage: rand(2) <= 1 spawns from the main pool.
func TestNativeMap14_InactivePointIsAPassage(t *testing.T) {
	ctx := context.Background()
	f := newMap14Fixture(t, dungeon.NewMemoryExpeditionRepository(), map14Hero("idle"))
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

// _P's R at (5,9) lands on a main-floor wall; the party walks out.
func TestNativeMap14_ReturnIntoAWall(t *testing.T) {
	ctx := context.Background()
	f := newMap14Fixture(t, dungeon.NewMemoryExpeditionRepository(), map14Hero("wall"))
	f.startMap14(t, "wall", 0, 0, 0)
	f.setFloor(t, "wall", 2, 5, 8)
	if _, err := f.svc.Move(ctx, "wall", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "wall"); exp.MapVariant != 1 || exp.PosX != 5 || exp.PosY != 9 {
		t.Fatalf("landing %+v", exp)
	}
	f.rng.floats = []float64{0.99}
	if _, err := f.svc.Move(ctx, "wall", dungeon.DirectionNorth); err != nil {
		t.Fatalf("walk out err=%v", err)
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
			f := newMap14Fixture(t, dungeon.NewMemoryExpeditionRepository(), map14Hero("once"))
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
