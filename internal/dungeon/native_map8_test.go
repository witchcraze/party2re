package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Variant 1 (map/8/1.cgi), x across, y down:
//
//	2A00D0000A3
//	A00000000D0
//	0000000000B
//	000W0000000
//	0000000000C
//	0W000SW0000
//	B0000000000
//	00000000000
//	00000W00W00
//	0C000000040
//	D03000C000B
func map8Hero(id string) corecharacter.Character {
	c := createTestChar(id, 40, 1_000_000, 1, 900)
	c.JobLevel, c.JobID = 17, "job-01"
	return c
}

func TestNativeMap8_LayoutsHaveNoInnerWalls(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap8ID)
	if d.RoundLimit != 50 || d.LeaderJobLevel != 17 || len(d.Variants) != 12 {
		t.Fatalf("map8 catalog %+v", d)
	}
	for i, fl := range d.Variants {
		// Every scatter destination (3–7 on both axes) lies inside the grid.
		if fl.Width < 8 || fl.Height < 8 || len(cellOf(fl, '1')) != 0 || len(cellOf(fl, 'S')) != 1 || len(fl.Pools) != 5 {
			t.Fatalf("variant %d geometry %dx%d pools=%d", i+1, fl.Width, fl.Height, len(fl.Pools))
		}
		if len(cellOf(fl, 'W')) == 0 || len(cellOf(fl, '4')) == 0 {
			t.Fatalf("variant %d lacks W or 4", i+1)
		}
	}
}

// A wall hit spends no round and scatters the party to y, then x, each
// int(rand(5) + 3) (event_1). Any member's hit moves the party.
func TestNativeMap8_WallHitScattersWithoutARound(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		wallScatters(t, dungeon.NewMemoryExpeditionRepository(), "lead")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map8-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id, id+"-ally")
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		wallScatters(t, store, id)
	})
}

func wallScatters(t *testing.T, store dungeon.ActiveExpeditionStore, lead string) {
	ctx := context.Background()
	f := newNativeFixtureOn(t, store, map8Hero(lead))
	ally := lead + "-ally"
	f.chars.chars[ally] = map8Hero(ally)
	f.rng.ints = []int{0}
	if _, err := f.svc.StartPartyExpedition(ctx, lead, []string{lead, ally}, dungeon.NativeMap8ID, "p"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mover    string
		draw     int
		wantX, Y int
	}{{lead, 0, 3, 3}, {ally, 4, 7, 7}} {
		f.moveTo(t, lead, 0, 3)
		before := f.reload(t, lead)
		f.rng.ints = []int{tc.draw, tc.draw}
		res, err := f.svc.Move(ctx, tc.mover, dungeon.DirectionWest)
		if err != nil || res.EventType != dungeon.EventMove || f.rng.intCalls != 2 || f.rng.floatCalls != 0 {
			t.Fatalf("%s wall %+v err=%v rng=%d/%d", tc.mover, res, err, f.rng.intCalls, f.rng.floatCalls)
		}
		exp := f.reload(t, lead)
		if exp.PosX != tc.wantX || exp.PosY != tc.Y || exp.Round != before.Round || exp.Revision != before.Revision+1 || len(exp.ConsumedEvents) != 0 {
			t.Fatalf("%s wall state %+v", tc.mover, exp)
		}
		rev := before.Revision
		if _, err := store.Step(ctx, lead, dungeon.StepParams{ExpectedExpeditionID: before.ID, ExpectedRevision: &rev,
			NewX: 0, NewY: 3, Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
			t.Fatalf("stale step err=%v", err)
		}
	}
	// y is drawn before x.
	f.moveTo(t, lead, 10, 3)
	f.rng.ints = []int{1, 3}
	if _, err := f.svc.Move(ctx, lead, dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, lead); exp.PosX != 6 || exp.PosY != 4 {
		t.Fatalf("draw order: at (%d,%d), want (6,4)", exp.PosX, exp.PosY)
	}
}

// W spends the round, then scatters; the landing cell, here the other W at
// (6,5), is not dispatched.
func TestNativeMap8_PassageWarpSpendsTheRound(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map8Hero("w"))
	f.placeNative(t, dungeon.NativeMap8ID, "w", 1, 2, 3, 10) // west of W at (3,3)
	f.rng.ints = []int{2, 3}
	if res, err := f.svc.Move(ctx, "w", dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 2 {
		t.Fatalf("W %+v err=%v rng=%d", res, err, f.rng.calls())
	}
	if exp := f.reload(t, "w"); exp.PosX != 6 || exp.PosY != 5 || exp.Round != 11 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("W state %+v", exp)
	}
}

// The limit rejects a wall hit before the warp draws.
func TestNativeMap8_LimitPrecedesTheWallWarp(t *testing.T) {
	f := newNativeFixture(t, map8Hero("limit"))
	f.placeNative(t, dungeon.NativeMap8ID, "limit", 1, 0, 3, 50)
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(context.Background(), "limit", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrMovementExhausted) || f.rng.calls() != 0 {
		t.Fatalf("limit err=%v rng=%d", err, f.rng.calls())
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || after.PosX != 0 || after.PosY != 3 || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}

// The edge, W, bosses and treasure all display as passage ($map_imgs{1} = '□').
func TestNativeMap8_ViewShowsOnlyPassage(t *testing.T) {
	f := newNativeFixture(t, map8Hero("view"))
	f.placeNative(t, dungeon.NativeMap8ID, "view", 1, 0, 0, 1)
	v, err := f.svc.ObserveExpedition(context.Background(), "view")
	if err != nil || v.Map == nil || v.Map.Formatted != "□□□\n□●□\n□□□" {
		t.Fatalf("view %+v err=%v", v.Map, err)
	}
}

// A, B, C, D and each treasure symbol fire once per run, across every cell
// with that symbol.
func TestNativeMap8_BossesAndTreasureFireOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name              string
		x, y, againX, agY int
		dir, againDir     dungeon.Direction
		want              dungeon.TileEventType
	}{
		{"A", 2, 0, 8, 0, dungeon.DirectionWest, dungeon.DirectionEast, dungeon.EventBoss},
		{"B", 10, 3, 1, 6, dungeon.DirectionNorth, dungeon.DirectionWest, dungeon.EventBoss},
		{"C", 10, 5, 2, 9, dungeon.DirectionNorth, dungeon.DirectionWest, dungeon.EventBoss},
		{"D", 3, 0, 9, 2, dungeon.DirectionEast, dungeon.DirectionNorth, dungeon.EventBoss},
		{"2", 1, 0, 1, 0, dungeon.DirectionWest, dungeon.DirectionWest, dungeon.EventTreasure},
		{"3", 10, 1, 1, 10, dungeon.DirectionNorth, dungeon.DirectionEast, dungeon.EventTreasure},
		{"4", 9, 8, 9, 8, dungeon.DirectionSouth, dungeon.DirectionSouth, dungeon.EventTreasure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map8Hero("once"))
			f.placeNative(t, dungeon.NativeMap8ID, "once", 1, tc.x, tc.y, 1)
			if res, err := f.svc.Move(ctx, "once", tc.dir); err != nil || res.EventType != tc.want {
				t.Fatalf("first %+v err=%v", res, err)
			}
			exp := f.reload(t, "once")
			exp.Encounter, exp.PosX, exp.PosY = nil, tc.againX, tc.agY
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			f.rng.floats = []float64{0.99} // the passage roll, if any, spawns nothing
			if res, err := f.svc.Move(ctx, "once", tc.againDir); err != nil || res.EventType != dungeon.EventMove || res.Monster != nil {
				t.Fatalf("repeat %+v err=%v", res, err)
			}
			if exp := f.reload(t, "once"); !slices.Equal(exp.ConsumedEvents, []string{tc.name}) {
				t.Fatalf("flags %v", exp.ConsumedEvents)
			}
		})
	}
}
