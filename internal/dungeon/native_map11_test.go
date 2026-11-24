package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// map/11/1.cgi, x across, y down: 234 / 101 101 / 0A0 / 101 101 / HBH / … /
// HFH / 101 101 / 1S1. Bosses sit at (1, 3·k) for A..F; H flanks B..F.
func map11Hero(id, job string) corecharacter.Character {
	c := createTestChar(id, 40, 1_000_000, 1, 900) // survives and never wins
	c.JobLevel, c.JobID = 25, job                  // enough for map11 (23) and map12 (25)
	return c
}

func TestNativeMap11_Catalog(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap11ID)
	if d.RoundLimit != 30 || d.LeaderJobLevel != 23 || len(d.Variants) != 1 {
		t.Fatalf("map11 catalog %+v", d)
	}
	fl := d.Variants[0]
	if fl.Width != 3 || fl.Height != 22 || fl.StartX != 1 || fl.StartY != 21 || len(fl.Pools) != 1 || len(fl.Pools[0].Monsters) != 5 {
		t.Fatalf("geometry %+v", fl)
	}
	for sym, stage := range map[byte]string{'B': "ordinary-20-", 'C': "ordinary-17-", 'D': "ordinary-11-", 'E': "ordinary-09-", 'F': "ordinary-08-"} {
		if !strings.HasPrefix(fl.Bosses[sym][0].ID, stage) {
			t.Errorf("%c boss %q, want stage %s", sym, fl.Bosses[sym][0].ID, stage)
		}
	}
	if !slices.Contains(monsterIDs(fl.Bosses['A']), "nm11-lucifer") {
		t.Errorf("A bosses %v", monsterIDs(fl.Bosses['A']))
	}
}

// The base passage event draws the map's five enemies uniformly.
func TestNativeMap11_PassageSpawn(t *testing.T) {
	f := newNativeFixture(t, map11Hero("spawn", "job-01"))
	f.placeNative(t, dungeon.NativeMap11ID, "spawn", 1, 1, 2, 1)
	f.rng.floats, f.rng.ints = []float64{0.5}, []int{4}
	res, err := f.svc.Move(context.Background(), "spawn", dungeon.DirectionNorth)
	if err != nil || res.EventType != dungeon.EventBattle || len(res.Monsters) == 0 || !slices.Contains(monsterIDs(res.Monsters), "nm11-lantern-imp") {
		t.Fatalf("spawn %+v err=%v", res.Monsters, err)
	}
}

// Each boss fires once with its own binding; re-entry draws nothing.
func TestNativeMap11_BossBindingsFireOnce(t *testing.T) {
	ctx := context.Background()
	fl := mapDef(t, dungeon.NativeMap11ID).Variants[0]
	for i, sym := range "ABCDEF" {
		t.Run(string(sym), func(t *testing.T) {
			y := 3 * (i + 1)
			f := newNativeFixture(t, map11Hero("boss", "job-01"))
			f.placeNative(t, dungeon.NativeMap11ID, "boss", 1, 1, y+1, 1)
			res, err := f.svc.Move(ctx, "boss", dungeon.DirectionNorth)
			if err != nil || res.EventType != dungeon.EventBoss || len(res.Monsters) == 0 || !slices.Equal(monsterIDs(res.Monsters), monsterIDs(fl.Bosses[byte(sym)])) {
				t.Fatalf("boss %+v err=%v", res.Monsters, err)
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

// Only an acting job 43 deciphers H: the roster, fallen included, returns to
// full HP and MP, and one flag covers every H cell.
func TestNativeMap11_RemedyTable(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		remedyTable(t, dungeon.NewMemoryExpeditionRepository(), dungeon.NativeMap11ID, "lead")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "map11-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id, id+"-medic")
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		remedyTable(t, store, dungeon.NativeMap11ID, id)
	})
}

// remedyTable checks the H tables shared by map11 and map12 on mapID.
func remedyTable(t *testing.T, store dungeon.ActiveExpeditionStore, mapID, lead string) {
	ctx := context.Background()
	medic := lead + "-medic"
	f := newNativeFixtureOn(t, store, map11Hero(lead, "job-01"))
	f.chars.chars[medic] = map11Hero(medic, "job-43")
	f.rng.ints = []int{0}
	if _, err := f.svc.StartPartyExpedition(ctx, lead, []string{lead, medic}, mapID, "p"); err != nil {
		t.Fatal(err)
	}
	f.setVitality(t, lead, medic, 10, 0)

	// The leader's job does not qualify: nothing changes and H stays unspent.
	f.moveTo(t, lead, 1, 6)
	if _, err := f.svc.Move(ctx, lead, dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, lead); memberOf(t, exp, medic).CurrentHP != 10 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("leader job 1 deciphered H %+v", exp)
	}
	f.setVitality(t, lead, lead, 0, 0) // fallen members are restored too
	f.moveTo(t, lead, 1, 6)
	stale := f.reload(t, lead)
	if _, err := f.svc.Move(ctx, medic, dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, lead)
	l, m := memberOf(t, exp, lead), memberOf(t, exp, medic)
	if l.CurrentHP != l.MaxHP || l.CurrentMP != l.MaxMP || m.CurrentHP != m.MaxHP || m.CurrentMP != m.MaxMP || !slices.Equal(exp.ConsumedEvents, []string{"H"}) || exp.PosX != 2 {
		t.Fatalf("remedy %+v", exp)
	}
	rev := stale.Revision
	if _, err := store.Step(ctx, lead, dungeon.StepParams{ExpectedExpeditionID: stale.ID, ExpectedRevision: &rev,
		NewX: 2, NewY: 6, RoundDelta: 1, EventKey: "H", Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
		t.Fatalf("stale step err=%v", err)
	}

	// Another H cell is spent too.
	f.setVitality(t, lead, lead, 5, 0)
	f.moveTo(t, lead, 1, 9)
	if _, err := f.svc.Move(ctx, medic, dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, lead); memberOf(t, exp, lead).CurrentHP != 5 {
		t.Fatalf("second H restored %+v", exp.Members)
	}
}

// Treasure 2/3/4 fire once per symbol.
func TestNativeMap11_TreasureOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		sym  string
		x, y int
		dir  dungeon.Direction
	}{{"2", 1, 0, dungeon.DirectionWest}, {"3", 1, 1, dungeon.DirectionNorth}, {"4", 1, 0, dungeon.DirectionEast}} {
		t.Run(tc.sym, func(t *testing.T) {
			f := newNativeFixture(t, map11Hero("chest", "job-01"))
			f.placeNative(t, dungeon.NativeMap11ID, "chest", 1, tc.x, tc.y, 1)
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

func TestNativeMap11_RoundLimit(t *testing.T) {
	f := newNativeFixture(t, map11Hero("limit", "job-01"))
	f.placeNative(t, dungeon.NativeMap11ID, "limit", 1, 1, 20, 30)
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(context.Background(), "limit", dungeon.DirectionNorth); !errors.Is(err, dungeon.ErrMovementExhausted) || f.rng.calls() != 0 {
		t.Fatalf("limit err=%v rng=%d", err, f.rng.calls())
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}
