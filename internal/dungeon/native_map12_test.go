package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// map/12/1.cgi has map11's layout: bosses at (1, 3·k) for A..F, H flanking B..F.
func TestNativeMap12_Catalog(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap12ID)
	if d.RoundLimit != 30 || d.LeaderJobLevel != 25 || len(d.Variants) != 1 {
		t.Fatalf("map12 catalog %+v", d)
	}
	fl := d.Variants[0]
	if fl.Width != 3 || fl.Height != 22 || fl.StartX != 1 || fl.StartY != 21 || len(fl.Pools[0].Monsters) != 5 {
		t.Fatalf("geometry %+v", fl)
	}
	for sym, stage := range map[byte]string{'C': "ordinary-13-", 'D': "ordinary-18-", 'E': "ordinary-12-", 'F': "ordinary-10-"} {
		if !strings.HasPrefix(fl.Bosses[sym].ID, stage) {
			t.Errorf("%c boss %q, want stage %s", sym, fl.Bosses[sym].ID, stage)
		}
	}
	// A (get_boss_data2) and B (get_boss_data) have different groups.
	if fl.Bosses['A'].ID != "nm12-diabolos-a" || fl.Bosses['B'].ID != "nm12-diabolos-b" {
		t.Errorf("A %q B %q", fl.Bosses['A'].ID, fl.Bosses['B'].ID)
	}
}

// Each boss fires once with its own binding; re-entry draws nothing.
func TestNativeMap12_BossBindingsFireOnce(t *testing.T) {
	ctx := context.Background()
	fl := mapDef(t, dungeon.NativeMap12ID).Variants[0]
	for i, sym := range "ABCDEF" {
		t.Run(string(sym), func(t *testing.T) {
			y := 3 * (i + 1)
			f := newNativeFixture(t, map11Hero("boss", "job-01"))
			f.placeNative(t, dungeon.NativeMap12ID, "boss", 1, 1, y+1, 1)
			res, err := f.svc.Move(ctx, "boss", dungeon.DirectionNorth)
			if err != nil || res.EventType != dungeon.EventBoss || res.Monster == nil || res.Monster.ID != fl.Bosses[byte(sym)].ID {
				t.Fatalf("boss %+v err=%v", res.Monster, err)
			}
			exp := f.reload(t, "boss")
			if !slices.Equal(exp.ConsumedEvents, []string{string(sym)}) {
				t.Fatalf("flags %v", exp.ConsumedEvents)
			}
			exp.Encounter, exp.PosY = nil, y+1
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			f.rng.intCalls, f.rng.floatCalls = 0, 0
			if res, err := f.svc.Move(ctx, "boss", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
				t.Fatalf("repeat %+v err=%v rng=%d", res, err, f.rng.calls())
			}
		})
	}
}

func TestNativeMap12_RemedyTable(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		remedyTable(t, dungeon.NewMemoryExpeditionRepository(), dungeon.NativeMap12ID, "lead")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map12-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id, id+"-medic")
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		remedyTable(t, store, dungeon.NativeMap12ID, id)
	})
}

// Treasure 2/3/4 fire once per symbol.
func TestNativeMap12_TreasureOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		sym  string
		x, y int
		dir  dungeon.Direction
	}{{"2", 1, 0, dungeon.DirectionWest}, {"3", 1, 1, dungeon.DirectionNorth}, {"4", 1, 0, dungeon.DirectionEast}} {
		t.Run(tc.sym, func(t *testing.T) {
			f := newNativeFixture(t, map11Hero("chest", "job-01"))
			f.placeNative(t, dungeon.NativeMap12ID, "chest", 1, tc.x, tc.y, 1)
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
		})
	}
}

func TestNativeMap12_RoundLimit(t *testing.T) {
	f := newNativeFixture(t, map11Hero("limit", "job-01"))
	f.placeNative(t, dungeon.NativeMap12ID, "limit", 1, 1, 20, 30)
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(context.Background(), "limit", dungeon.DirectionNorth); !errors.Is(err, dungeon.ErrMovementExhausted) || f.rng.calls() != 0 {
		t.Fatalf("limit err=%v rng=%d", err, f.rng.calls())
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}
