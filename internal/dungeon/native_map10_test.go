package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Floors (map/10/1.cgi, _1.cgi, __1.cgi), x across, y down:
//
//	0F0  101  234
//	000  101  000
//	0A0  101  0C0
//	101  101  101
//	101  0B0  101
//	101  000  101
//	1S1  0F0  101
func map10Hero(id string) corecharacter.Character {
	c := createTestChar(id, 40, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID = 21, "job-01"
	return c
}

// placeMap10 starts on the first floor, then moves the run to variant at (x, y).
func (f *nativeFixture) placeMap10(t *testing.T, id string, variant, x, y, round int) {
	t.Helper()
	f.placeNative(t, dungeon.NativeMap10ID, id, 1, x, y, round)
	exp := f.reload(t, id)
	exp.MapVariant = variant
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
}

func TestNativeMap10_StartsOnTheFirstFloor(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap10ID)
	if d.RoundLimit != 30 || d.LeaderJobLevel != 21 || len(d.Variants) != 3 {
		t.Fatalf("map10 catalog %+v", d)
	}
	f := newNativeFixture(t, map10Hero("start"))
	f.rng.ints = []int{2}
	exp, err := f.svc.StartExpedition(context.Background(), "start", dungeon.NativeMap10ID)
	if err != nil || exp.MapVariant != 1 || exp.PosX != 1 || exp.PosY != 6 || exp.RoundLimit != 30 {
		t.Fatalf("start %+v err=%v", exp, err)
	}
}

// F climbs at the same coordinate; round, flags and the roster carry over.
func TestNativeMap10_RoutesCarryTheRun(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		routesCarry(t, dungeon.NewMemoryExpeditionRepository(), "lead")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map10-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id, id+"-ally")
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		routesCarry(t, store, id)
	})
}

func routesCarry(t *testing.T, store dungeon.ActiveExpeditionStore, lead string) {
	ctx := context.Background()
	ally := lead + "-ally"
	f := newNativeFixtureOn(t, store, map10Hero(lead))
	f.chars.chars[ally] = map10Hero(ally)
	f.rng.ints = []int{0}
	if _, err := f.svc.StartPartyExpedition(ctx, lead, []string{lead, ally}, dungeon.NativeMap10ID, "p"); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, lead)
	exp.PosX, exp.PosY, exp.Round, exp.ConsumedEvents = 1, 1, 7, []string{"A"}
	if err := store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if v, err := f.svc.ObserveExpedition(ctx, ally); err != nil || v.Floor != 1 || v.FloorName != "封刃の魔塔１階" {
		t.Fatalf("floor 1 view %+v err=%v", v, err)
	}
	for _, tc := range []struct {
		mover          string
		dir            dungeon.Direction
		variant, y     int
		round, floorNo int
		floorName      string
	}{
		{ally, dungeon.DirectionNorth, 2, 0, 8, 2, "封刃の魔塔２階"},
		{lead, dungeon.DirectionSouth, 3, 6, 9, 3, "封刃の魔塔３階"},
	} {
		if tc.variant == 3 {
			f.moveTo(t, lead, 1, 5)
		}
		before := f.reload(t, lead)
		f.rng.intCalls, f.rng.floatCalls = 0, 0
		if res, err := f.svc.Move(ctx, tc.mover, tc.dir); err != nil || res.EventType != dungeon.EventMove || f.rng.floatCalls != 0 {
			t.Fatalf("route to %d %+v err=%v", tc.variant, res, err)
		}
		exp := f.reload(t, lead)
		if exp.MapVariant != tc.variant || exp.PosX != 1 || exp.PosY != tc.y || exp.Round != tc.round || exp.RoundLimit != 30 ||
			exp.Encounter != nil || !slices.Equal(exp.ConsumedEvents, []string{"A"}) || len(exp.Members) != 2 {
			t.Fatalf("route to %d %+v", tc.variant, exp)
		}
		if v, err := f.svc.ObserveExpedition(ctx, ally); err != nil || v.Floor != tc.floorNo || v.FloorName != tc.floorName {
			t.Fatalf("view %+v err=%v", v, err)
		}
		rev := before.Revision
		if _, err := store.Step(ctx, lead, dungeon.StepParams{ExpectedExpeditionID: before.ID, ExpectedRevision: &rev,
			NewX: 1, NewY: 1, RoundDelta: 1, Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
			t.Fatalf("stale step err=%v", err)
		}
	}
}

// A passage spawn meets an attack-sealed party (event_0); index 24 of the
// appearance list is the rarer metal enemy.
func TestNativeMap10_SpawnMeetsASealedParty(t *testing.T) {
	f := newNativeFixture(t, map10Hero("spawn"))
	f.placeMap10(t, "spawn", 1, 1, 6, 1)
	f.rng.floats, f.rng.ints = []float64{0.5}, []int{24}
	res, err := f.svc.Move(context.Background(), "spawn", dungeon.DirectionNorth)
	if err != nil || res.EventType != dungeon.EventBattle || len(res.Monsters) == 0 || !slices.Contains(monsterIDs(res.Monsters), "nm10-quicksilver-king") {
		t.Fatalf("spawn %+v err=%v", res.Monsters, err)
	}
	if exp := f.reload(t, "spawn"); exp.Encounter.Statuses["spawn"] != corebattle.StatusPhysicalSeal {
		t.Fatalf("not sealed %+v", exp.Encounter)
	}
}

// Each floor binds its own boss; each meets a sealed party, sets only its own
// flag and is silent on re-entry.
func TestNativeMap10_FloorBosses(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name          string
		variant, x, y int
		dir           dungeon.Direction
		want          string
	}{
		{"A", 1, 1, 3, dungeon.DirectionNorth, "nm10-dark-sorcerer"},
		{"B", 2, 1, 3, dungeon.DirectionSouth, "nm10-fire-bird"},
		{"C", 3, 1, 3, dungeon.DirectionNorth, "nm10-end-beast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map10Hero("boss"))
			f.placeMap10(t, "boss", tc.variant, tc.x, tc.y, 1)
			res, err := f.svc.Move(ctx, "boss", tc.dir)
			if err != nil || res.EventType != dungeon.EventBoss || len(res.Monsters) == 0 || !slices.Contains(monsterIDs(res.Monsters), tc.want) {
				t.Fatalf("boss %+v err=%v", res.Monsters, err)
			}
			exp := f.reload(t, "boss")
			if exp.Encounter.Statuses["boss"] != corebattle.StatusPhysicalSeal || !slices.Equal(exp.ConsumedEvents, []string{tc.name}) {
				t.Fatalf("boss state %+v %v", exp.Encounter, exp.ConsumedEvents)
			}
			exp.Encounter, exp.PosX, exp.PosY = nil, tc.x, tc.y
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			f.rng.intCalls, f.rng.floatCalls = 0, 0
			if res, err := f.svc.Move(ctx, "boss", tc.dir); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
				t.Fatalf("repeat %+v err=%v rng=%d", res, err, f.rng.calls())
			}
		})
	}
}

// The top floor's treasure fires once per symbol.
func TestNativeMap10_TopFloorTreasure(t *testing.T) {
	ctx := context.Background()
	for x, sym := range []string{"2", "3", "4"} {
		t.Run(sym, func(t *testing.T) {
			f := newNativeFixture(t, map10Hero("chest"))
			f.placeMap10(t, "chest", 3, x, 1, 1)
			if res, err := f.svc.Move(ctx, "chest", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventTreasure {
				t.Fatalf("first %+v err=%v", res, err)
			}
			exp := f.reload(t, "chest")
			exp.Encounter, exp.PosY = nil, 1
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			if res, err := f.svc.Move(ctx, "chest", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove {
				t.Fatalf("repeat %+v err=%v", res, err)
			}
			if exp := f.reload(t, "chest"); !slices.Equal(exp.ConsumedEvents, []string{sym}) {
				t.Fatalf("flags %v", exp.ConsumedEvents)
			}
		})
	}
}

// The limit rejects on an upper floor before any draw.
func TestNativeMap10_RoundLimit(t *testing.T) {
	f := newNativeFixture(t, map10Hero("limit"))
	f.placeMap10(t, "limit", 2, 1, 5, 30)
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(context.Background(), "limit", dungeon.DirectionSouth); !errors.Is(err, dungeon.ErrMovementExhausted) || f.rng.calls() != 0 {
		t.Fatalf("limit err=%v rng=%d", err, f.rng.calls())
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || after.MapVariant != 2 || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}
