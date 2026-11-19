package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Variant 1 (map/7/1.cgi), x across, y down:
//
//	01213
//	T0B1A
//	10110
//	00000
//	0101T
//	01S10
//	01011
//	00001
//	D1101
//	01t0b
//	4101K
func map7Hero(id, job string, hp, attack int) corecharacter.Character {
	c := createTestChar(id, 30, hp, attack, 900)
	c.JobLevel, c.JobID = 15, job
	return c
}

func (f *nativeFixture) placeMap7(t *testing.T, id string, variant, x, y, round int) {
	t.Helper()
	f.placeNative(t, dungeon.NativeMap7ID, 60, id, variant, x, y, round)
}

func (f *nativeFixture) startMap7Party(t *testing.T, variant, x, y int) {
	t.Helper()
	f.rng.ints = []int{variant - 1}
	if _, err := f.svc.StartPartyExpedition(context.Background(), "lead", []string{"lead", "ally"}, dungeon.NativeMap7ID, "p"); err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, "lead", x, y)
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
}

func TestNativeMap7_LayoutsHideTheLastTreasureBehindTheRock(t *testing.T) {
	var d dungeon.Dungeon
	for _, c := range dungeon.DefaultDungeonCatalog() {
		if c.ID == dungeon.NativeMap7ID {
			d = c
		}
	}
	if d.RoundLimit != 60 || d.LeaderJobLevel != 15 || len(d.Variants) != 6 {
		t.Fatalf("map7 catalog %+v", d)
	}
	for i, fl := range d.Variants {
		if fl.Width != 5 || fl.Height != 11 || fl.StartX != 2 || fl.StartY != 5 {
			t.Fatalf("variant %d geometry %+v", i+1, fl)
		}
		for _, sym := range []byte("KDABb4") {
			if n := len(cellOf(fl, sym)); n != 1 {
				t.Fatalf("variant %d has %d %c cells", i+1, n, sym)
			}
		}
		closed, open := reachable(fl, false), reachable(fl, true)
		if !closed[cellOf(fl, 'K')[0]] || closed[cellOf(fl, '4')[0]] || !open[cellOf(fl, '4')[0]] {
			t.Fatalf("variant %d: K must be reachable and 4 only through D", i+1)
		}
	}
}

// K is any mover's switch; it opens D through the shared flag once, and both
// marks disappear (map/7/1.cgi:20–21,30).
func TestNativeMap7_SwitchOpensTheRock(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		switchOpensRock(t, dungeon.NewMemoryExpeditionRepository(), "switch")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map7-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		switchOpensRock(t, store, id)
	})
}

func switchOpensRock(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newNativeFixtureOn(t, store, map7Hero(id, "job-01", 5000, 900))
	south := func(x, y int) string { // the tile south of (x, y)
		f.moveTo(t, id, x, y)
		v, err := f.svc.ObserveExpedition(ctx, id)
		if err != nil || v.Map == nil {
			t.Fatalf("observe %+v err=%v", v, err)
		}
		return v.Map.Tiles[2][1]
	}
	f.placeMap7(t, id, 1, 4, 9, 1)
	if k, d := south(4, 9), south(0, 7); k != "★" || d != "岩" {
		t.Fatalf("marks before the switch %q %q", k, d)
	}
	f.moveTo(t, id, 4, 9)
	stale := f.reload(t, id)
	for range 2 {
		f.moveTo(t, id, 4, 9) // north of K is the boss b
		if res, err := f.svc.Move(ctx, id, dungeon.DirectionSouth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
			t.Fatalf("switch %+v err=%v rng=%d", res, err, f.rng.calls())
		}
		if exp := f.reload(t, id); !slices.Equal(exp.ConsumedEvents, []string{"DK"}) {
			t.Fatalf("switch flag %v", exp.ConsumedEvents)
		}
	}
	rev := stale.Revision
	if _, err := store.Step(ctx, id, dungeon.StepParams{ExpectedExpeditionID: stale.ID, ExpectedRevision: &rev,
		NewX: 4, NewY: 10, RoundDelta: 1, EventKey: "DK", Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
		t.Fatalf("stale switch err=%v", err)
	}
	if k, d := south(4, 9), south(0, 7); k != "□" || d != "□" {
		t.Fatalf("marks after the switch %q %q", k, d)
	}
	if _, err := f.svc.Move(ctx, id, dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, id); exp.PosX != 0 || exp.PosY != 8 || !slices.Equal(exp.ConsumedEvents, []string{"DK"}) {
		t.Fatalf("open rock %+v", exp)
	}
}

// Variant 6 defines no K mark, so the switch looks like a passage.
func TestNativeMap7_Variant6SwitchIsUnmarked(t *testing.T) {
	f := newNativeFixture(t, map7Hero("v6", "job-01", 5000, 900))
	f.placeMap7(t, "v6", 6, 1, 1, 1)
	v, err := f.svc.ObserveExpedition(context.Background(), "v6")
	if err != nil || v.Map == nil || v.Map.Tiles[0][0] != "□" {
		t.Fatalf("variant 6 view %+v err=%v", v.Map, err)
	}
}

// A denied entry spends the round and returns to the only approach: --$py in
// variants 1, 5 and 6, ++$py in 2–4. A wall hit spends nothing.
func TestNativeMap7_DeniedRockSpendsTheRoundAndReturns(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		variant, x, y int
		dir           dungeon.Direction
	}{{1, 0, 7, dungeon.DirectionSouth}, {2, 4, 3, dungeon.DirectionNorth}} {
		t.Run(fmt.Sprint(tc.variant), func(t *testing.T) {
			f := newNativeFixture(t, map7Hero("denied", "job-01", 5000, 900))
			f.placeMap7(t, "denied", tc.variant, tc.x, tc.y, 10)
			if res, err := f.svc.Move(ctx, "denied", tc.dir); err != nil || res.EventType != dungeon.EventMove {
				t.Fatalf("denied %+v err=%v", res, err)
			}
			exp := f.reload(t, "denied")
			if exp.PosX != tc.x || exp.PosY != tc.y || exp.Round != 11 || len(exp.ConsumedEvents) != 0 || f.rng.calls() != 0 {
				t.Fatalf("denied state %+v rng=%d", exp, f.rng.calls())
			}
			if tc.variant != 1 {
				return
			}
			if _, err := f.svc.Move(ctx, "denied", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrImpassableWall) {
				t.Fatalf("wall err=%v", err)
			}
			if after := f.reload(t, "denied"); after.Revision != exp.Revision || after.Round != 11 {
				t.Fatalf("wall changed %+v", after)
			}
		})
	}
}

// Only the acting character's job 4 or 25 breaks the rock; another member's
// job does not count, and the switch then does nothing.
func TestNativeMap7_OnlyTheActingJob4Or25BreaksTheRock(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map7Hero("lead", "job-01", 5000, 900))
	f.chars.chars["ally"] = map7Hero("ally", "job-04", 5000, 900)
	f.startMap7Party(t, 1, 0, 7)
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "lead"); exp.PosY != 7 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("ally job 4 opened the rock for the actor %+v", exp)
	}
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "lead"); exp.PosY != 8 || !slices.Equal(exp.ConsumedEvents, []string{"DK"}) {
		t.Fatalf("acting job 4 %+v", exp)
	}
	f.moveTo(t, "lead", 4, 9)
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth); err != nil || f.rng.calls() != 0 {
		t.Fatalf("switch after the rock err=%v rng=%d", err, f.rng.calls())
	}
	if exp := f.reload(t, "lead"); !slices.Equal(exp.ConsumedEvents, []string{"DK"}) {
		t.Fatalf("switch after the rock %v", exp.ConsumedEvents)
	}

	g := newNativeFixture(t, map7Hero("j25", "job-25", 5000, 900))
	g.placeMap7(t, "j25", 2, 4, 3, 1)
	if _, err := g.svc.Move(ctx, "j25", dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	if exp := g.reload(t, "j25"); exp.PosY != 2 || !slices.Equal(exp.ConsumedEvents, []string{"DK"}) {
		t.Fatalf("acting job 25 %+v", exp)
	}
}

// The limit applies to a denied entry like any step and forces no wipeout.
func TestNativeMap7_DeniedRockAtRoundLimit(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map7Hero("limit", "job-01", 5000, 900))
	f.placeMap7(t, "limit", 1, 0, 7, 59)
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionNorth); !errors.Is(err, dungeon.ErrMovementExhausted) {
		t.Fatalf("limit err=%v", err)
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || after.Round != 60 || after.Status != dungeon.StatusExploring || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}

// T paralyses the party, then an encounter follows on every entry (event_T).
func TestNativeMap7_ParalysisPrecedesEncounter(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map7Hero("gas", "job-01", 1_000_000, 1))
	f.placeMap7(t, "gas", 1, 1, 1, 1) // east of T at (0,1)
	res, err := f.svc.Move(ctx, "gas", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventBattle || res.Monster == nil {
		t.Fatalf("gas %+v err=%v", res, err)
	}
	exp := f.reload(t, "gas")
	if exp.Encounter == nil || exp.Encounter.Statuses["gas"] != string(corebattle.StatusParalyze) || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("pending gas encounter %+v", exp)
	}
}

// t hits every member with _trap_d(150), or 140 in variant 3.
func TestNativeMap7_HeatTrapHitsEveryMember(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		variant, x, y, want int
	}{{1, 3, 9, 500 - 135}, {3, 1, 1, 500 - 126}} {
		t.Run(fmt.Sprint(tc.variant), func(t *testing.T) {
			f := newNativeFixture(t, map7Hero("lead", "job-01", 500, 900))
			f.chars.chars["ally"] = map7Hero("ally", "job-01", 500, 900)
			f.startMap7Party(t, tc.variant, tc.x, tc.y)
			f.rng.floats = []float64{0, 0}
			if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionWest); err != nil {
				t.Fatal(err)
			}
			exp := f.reload(t, "lead")
			if memberOf(t, exp, "lead").CurrentHP != tc.want || memberOf(t, exp, "ally").CurrentHP != tc.want || len(exp.ConsumedEvents) != 0 {
				t.Fatalf("trap %+v %v", exp.Members, exp.ConsumedEvents)
			}
		})
	}
}

// Each event draws one of stages 8–11, and the fixed group replaces its boss
// one time in three (_data.cgi).
func TestNativeMap7_BossDrawsTheFixedGroupOneInThree(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		pool  int
		fixed bool
	}{{0, false}, {1, false}, {2, true}, {11, true}} {
		t.Run(fmt.Sprint(tc.pool), func(t *testing.T) {
			f := newNativeFixture(t, map7Hero("boss", "job-01", 1_000_000, 1))
			f.placeMap7(t, "boss", 1, 4, 2, 1) // south of A at (4,1)
			f.rng.ints = []int{tc.pool}
			res, err := f.svc.Move(ctx, "boss", dungeon.DirectionNorth)
			if err != nil || res.EventType != dungeon.EventBoss || res.Monster == nil || (res.Monster.ID == "nm7-ifrit") != tc.fixed {
				t.Fatalf("boss %+v err=%v", res.Monster, err)
			}
		})
	}
}

// A, b, B and each treasure symbol fire once per run; re-entry after the
// encounter is cleared draws nothing.
func TestNativeMap7_BossesAndTreasureFireOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name          string
		variant, x, y int
		dir           dungeon.Direction
		want          dungeon.TileEventType
	}{
		{"A", 1, 4, 2, dungeon.DirectionNorth, dungeon.EventBoss},
		{"b", 1, 3, 9, dungeon.DirectionEast, dungeon.EventBoss},
		{"B", 1, 1, 1, dungeon.DirectionEast, dungeon.EventBoss},
		{"4", 1, 0, 9, dungeon.DirectionSouth, dungeon.EventTreasure},
		{"2", 3, 4, 4, dungeon.DirectionSouth, dungeon.EventTreasure},
		{"3", 2, 0, 9, dungeon.DirectionSouth, dungeon.EventTreasure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map7Hero("once", "job-01", 1_000_000, 1))
			f.placeMap7(t, "once", tc.variant, tc.x, tc.y, 1)
			if res, err := f.svc.Move(ctx, "once", tc.dir); err != nil || res.EventType != tc.want {
				t.Fatalf("first %+v err=%v", res, err)
			}
			exp := f.reload(t, "once")
			exp.Encounter, exp.PosX, exp.PosY = nil, tc.x, tc.y
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			if res, err := f.svc.Move(ctx, "once", tc.dir); err != nil || res.EventType != dungeon.EventMove || res.Monster != nil {
				t.Fatalf("repeat %+v err=%v", res, err)
			}
		})
	}
}
