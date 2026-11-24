package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Variant 1 (map/9/1.cgi), x across, y down:
//
//	0100014
//	0102010
//	0100010
//	011A110
//	0000000
//	1111101
//	0301000
//	0001000
//	0B010C0
//	1011101
//	000S000
func map9Hero(id string) corecharacter.Character {
	c := createTestChar(id, 40, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID = 19, "job-01"
	return c
}

func TestNativeMap9_Layouts(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap9ID)
	if d.RoundLimit != 60 || d.LeaderJobLevel != 19 || len(d.Variants) != 4 {
		t.Fatalf("map9 catalog %+v", d)
	}
	for i, fl := range d.Variants {
		if fl.Width != 7 || fl.Height != 11 || fl.StartX != 3 || fl.StartY != 10 || len(fl.Pools) != 1 || len(fl.Pools[0].Monsters) != 9 {
			t.Fatalf("variant %d geometry %+v", i+1, fl)
		}
		open := reachable(fl, true)
		for _, sym := range []byte("ABC234") {
			cells := cellOf(fl, sym)
			if len(cells) != 1 || !open[cells[0]] {
				t.Fatalf("variant %d: %c cells %v", i+1, sym, cells)
			}
		}
	}
}

// A passage spawn meets a sealed party, and the seal survives save/reload of
// the unfinished battle (event_0).
func TestNativeMap9_SpawnMeetsASealedParty(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		sealedSpawn(t, dungeon.NewMemoryExpeditionRepository(), "lead")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map9-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id, id+"-ally")
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		sealedSpawn(t, store, id)
	})
}

func sealedSpawn(t *testing.T, store dungeon.ActiveExpeditionStore, lead string) {
	ctx := context.Background()
	ally := lead + "-ally"
	f := newNativeFixtureOn(t, store, map9Hero(lead))
	f.chars.chars[ally] = map9Hero(ally)
	f.rng.ints = []int{0}
	if _, err := f.svc.StartPartyExpedition(ctx, lead, []string{lead, ally}, dungeon.NativeMap9ID, "p"); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	stale := f.reload(t, lead)
	f.rng.floats = []float64{0.5} // roll 1.0 <= 1 spawns
	f.rng.ints = []int{8}
	res, err := f.svc.Move(ctx, ally, dungeon.DirectionEast)
	if err != nil || res.EventType != dungeon.EventBattle || len(res.Monsters) == 0 || res.Monsters[0].Name != "幻騎士A" {
		t.Fatalf("spawn %+v err=%v", res.Monsters, err)
	}
	exp := f.reload(t, lead)
	for _, id := range []string{lead, ally} {
		if exp.Encounter == nil || exp.Encounter.Statuses[id] != corebattle.StatusMagicSeal {
			t.Fatalf("%s not sealed: %+v", id, exp.Encounter)
		}
	}
	if exp.PosX != 4 || exp.Round != stale.Round+1 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("spawn state %+v", exp)
	}
	rev := stale.Revision
	if _, err := store.Step(ctx, lead, dungeon.StepParams{ExpectedExpeditionID: stale.ID, ExpectedRevision: &rev,
		Combat: true, NewX: 2, NewY: 10, RoundDelta: 1, Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
		t.Fatalf("stale step err=%v", err)
	}
}

// A passage roll above 1 spawns nothing and leaves no status behind.
func TestNativeMap9_QuietPassage(t *testing.T) {
	f := newNativeFixture(t, map9Hero("quiet"))
	f.placeNative(t, dungeon.NativeMap9ID, "quiet", 1, 3, 10, 1)
	if res, err := f.svc.Move(context.Background(), "quiet", dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 1 {
		t.Fatalf("quiet %+v err=%v rng=%d", res, err, f.rng.calls())
	}
	if exp := f.reload(t, "quiet"); exp.Encounter != nil {
		t.Fatalf("quiet encounter %+v", exp.Encounter)
	}
}

// A uses _data.cgi's boss; B and C draw a stage 8–12 boss. Each meets a
// sealed party, sets only its own flag, and is silent on re-entry.
func TestNativeMap9_BossesAreSealedAndOnce(t *testing.T) {
	ctx := context.Background()
	stageBoss := map[int]string{}
	for _, draw := range []int{0, 4} {
		f := newNativeFixture(t, map9Hero("probe"))
		f.placeNative(t, dungeon.NativeMap9ID, "probe", 1, 1, 7, 1)
		f.rng.ints = []int{draw}
		res, err := f.svc.Move(ctx, "probe", dungeon.DirectionSouth)
		if err != nil || len(res.Monsters) == 0 {
			t.Fatalf("draw %d %+v err=%v", draw, res, err)
		}
		stageBoss[draw] = res.Monsters[0].ID
	}
	if stageBoss[0] == stageBoss[4] {
		t.Fatalf("stages 8 and 12 drew the same boss %v", stageBoss)
	}
	for _, tc := range []struct {
		name string
		x, y int
		dir  dungeon.Direction
		draw int
		want string
	}{
		{"A", 3, 4, dungeon.DirectionNorth, 0, "nm9-knight-golem"},
		{"B", 1, 9, dungeon.DirectionNorth, 4, stageBoss[4]},
		{"C", 5, 9, dungeon.DirectionNorth, 0, stageBoss[0]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map9Hero("boss"))
			f.placeNative(t, dungeon.NativeMap9ID, "boss", 1, tc.x, tc.y, 1)
			f.rng.ints = []int{tc.draw}
			res, err := f.svc.Move(ctx, "boss", tc.dir)
			if err != nil || res.EventType != dungeon.EventBoss || len(res.Monsters) == 0 || !slices.Contains(monsterIDs(res.Monsters), tc.want) {
				t.Fatalf("boss %+v err=%v", res.Monsters, err)
			}
			exp := f.reload(t, "boss")
			if exp.Encounter.Statuses["boss"] != corebattle.StatusMagicSeal || !slices.Equal(exp.ConsumedEvents, []string{tc.name}) {
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

// Treasure fires once per symbol and is marked until opened.
func TestNativeMap9_TreasureOnceAndMarked(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		sym     string
		variant int
		x, y    int
		dir     dungeon.Direction
	}{{"2", 1, 2, 1, dungeon.DirectionEast}, {"3", 1, 0, 6, dungeon.DirectionEast}, {"4", 1, 6, 1, dungeon.DirectionNorth}} {
		t.Run(tc.sym, func(t *testing.T) {
			f := newNativeFixture(t, map9Hero("chest"))
			f.placeNative(t, dungeon.NativeMap9ID, "chest", tc.variant, tc.x, tc.y, 1)
			v, err := f.svc.ObserveExpedition(ctx, "chest")
			if err != nil || v.Map == nil || !strings.Contains(v.Map.Formatted, "宝") {
				t.Fatalf("mark %+v err=%v", v.Map, err)
			}
			if res, err := f.svc.Move(ctx, "chest", tc.dir); err != nil || res.EventType != dungeon.EventTreasure {
				t.Fatalf("first %+v err=%v", res, err)
			}
			exp := f.reload(t, "chest")
			exp.Encounter, exp.PosX, exp.PosY = nil, tc.x, tc.y
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			if res, err := f.svc.Move(ctx, "chest", tc.dir); err != nil || res.EventType != dungeon.EventMove {
				t.Fatalf("repeat %+v err=%v", res, err)
			}
			if exp := f.reload(t, "chest"); !slices.Equal(exp.ConsumedEvents, []string{tc.sym}) {
				t.Fatalf("flags %v", exp.ConsumedEvents)
			}
		})
	}
}

// The limit rejects before the seal or any draw.
func TestNativeMap9_RoundLimit(t *testing.T) {
	f := newNativeFixture(t, map9Hero("limit"))
	f.placeNative(t, dungeon.NativeMap9ID, "limit", 1, 3, 10, 60)
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(context.Background(), "limit", dungeon.DirectionEast); !errors.Is(err, dungeon.ErrMovementExhausted) || f.rng.calls() != 0 {
		t.Fatalf("limit err=%v rng=%d", err, f.rng.calls())
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}
