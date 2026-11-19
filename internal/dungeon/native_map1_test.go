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

func map1Hero(id, job string) corecharacter.Character {
	c := createTestChar(id, 30, 5000, 900, 900)
	c.JobLevel, c.JobID = 3, job
	return c
}

func TestNativeMap1_LayoutsSealTheBossBehindOneDoorApproach(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap1ID)
	if d.RoundLimit != 50 || d.LeaderJobLevel != 3 || len(d.Variants) != 7 {
		t.Fatalf("limit=%d job=%d variants=%d", d.RoundLimit, d.LeaderJobLevel, len(d.Variants))
	}
	// Legacy entry direction into the locked door: ++$px (west) in 1, 2, 5; --$py (south) otherwise.
	entry := []dungeon.Direction{"west", "west", "south", "south", "west", "south", "south"}
	for i, fl := range d.Variants {
		counts := map[byte]int{}
		for _, row := range fl.Grid {
			if len(row) != 9 {
				t.Fatalf("variant %d row %q", i+1, row)
			}
			for j := range len(row) {
				counts[row[j]]++
			}
		}
		if len(fl.Grid) != 9 || counts['S'] != 1 || counts['B'] != 1 || counts['D'] != 1 || counts['2'] != 1 ||
			counts['3'] < 1 || counts['K'] < 1 || counts['H'] < 1 || len(fl.Pools) != 1 {
			t.Fatalf("variant %d counts %v", i+1, counts)
		}
		closed, open := reachable(fl, false), reachable(fl, true)
		for _, sym := range []byte("K3H") {
			for _, p := range cellOf(fl, sym) {
				if !closed[p] {
					t.Fatalf("variant %d %c at %v needs the door", i+1, sym, p)
				}
			}
		}
		for _, sym := range []byte("B2") {
			p := cellOf(fl, sym)[0]
			if closed[p] || !open[p] {
				t.Fatalf("variant %d %c at %v not sealed behind the door", i+1, sym, p)
			}
		}
		door := cellOf(fl, 'D')[0]
		var approaches []dungeon.Direction
		for _, c := range []struct {
			dx, dy int
			dir    dungeon.Direction
		}{{1, 0, dungeon.DirectionWest}, {-1, 0, dungeon.DirectionEast}, {0, -1, dungeon.DirectionSouth}, {0, 1, dungeon.DirectionNorth}} {
			if closed[[2]int{door[0] + c.dx, door[1] + c.dy}] {
				approaches = append(approaches, c.dir)
			}
		}
		if !slices.Equal(approaches, []dungeon.Direction{entry[i]}) {
			t.Fatalf("variant %d door approaches %v, want %v", i+1, approaches, entry[i])
		}
	}
}

// Variant 1: K at (8,0); door (3,0) entered west from (4,0); boss (1,0); 2 at (0,0).
func TestNativeMap1_KeyOpensDoorWithoutBeingConsumed(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map1Hero("key", "job-01"))
	f.placeNative(t, dungeon.NativeMap1ID, "key", 1, 8, 1, 1)
	if res, err := f.svc.Move(ctx, "key", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("key %+v err=%v", res, err)
	}
	if exp := f.reload(t, "key"); !slices.Equal(exp.ConsumedEvents, []string{"K"}) || f.rng.calls() != 0 {
		t.Fatalf("key events %v rng=%d", exp.ConsumedEvents, f.rng.calls())
	}
	f.moveTo(t, "key", 4, 0)
	if _, err := f.svc.Move(ctx, "key", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "key")
	if exp.PosX != 3 || exp.PosY != 0 || !slices.Equal(exp.ConsumedEvents, []string{"K", "D"}) || exp.Round != 3 {
		t.Fatalf("opened %+v", exp)
	}
	// Re-entering the open door is a plain move: no event, spawn roll or new state.
	if _, err := f.svc.Move(ctx, "key", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	if _, err := f.svc.Move(ctx, "key", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "key"); exp.PosX != 3 || !slices.Equal(exp.ConsumedEvents, []string{"K", "D"}) || f.rng.calls() != 0 {
		t.Fatalf("repeat %+v rng=%d", exp, f.rng.calls())
	}
}

func TestNativeMap1_EveryKeyCellSharesOneKey(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map1Hero("keys", "job-01"))
	f.placeNative(t, dungeon.NativeMap1ID, "keys", 3, 1, 1, 1) // variant 3 keys at (1,0) and (6,8)
	if _, err := f.svc.Move(ctx, "keys", dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, "keys", 6, 7)
	if _, err := f.svc.Move(ctx, "keys", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "keys"); exp.PosX != 6 || exp.PosY != 8 || !slices.Equal(exp.ConsumedEvents, []string{"K"}) || f.rng.calls() != 0 {
		t.Fatalf("second key %+v rng=%d", exp, f.rng.calls())
	}
}

func TestNativeMap1_LockedDoorSpendsTheRoundAndReturns(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		variant, x, y int
		dir           dungeon.Direction
	}{{1, 4, 0, dungeon.DirectionWest}, {3, 8, 4, dungeon.DirectionSouth}} {
		t.Run(fmt.Sprint(tc.variant), func(t *testing.T) {
			f := newNativeFixture(t, map1Hero("locked", "job-01"))
			f.placeNative(t, dungeon.NativeMap1ID, "locked", tc.variant, tc.x, tc.y, 10)
			res, err := f.svc.Move(ctx, "locked", tc.dir)
			if err != nil || res.EventType != dungeon.EventMove {
				t.Fatalf("locked %+v err=%v", res, err)
			}
			exp := f.reload(t, "locked")
			if exp.PosX != tc.x || exp.PosY != tc.y || exp.Round != 11 || len(exp.ConsumedEvents) != 0 || f.rng.calls() != 0 {
				t.Fatalf("locked state %+v rng=%d", exp, f.rng.calls())
			}
		})
	}
	// An ordinary wall spends nothing.
	f := newNativeFixture(t, map1Hero("wall", "job-01"))
	f.placeNative(t, dungeon.NativeMap1ID, "wall", 1, 4, 0, 10)
	before := f.reload(t, "wall")
	if _, err := f.svc.Move(ctx, "wall", dungeon.DirectionNorth); !errors.Is(err, dungeon.ErrImpassableWall) {
		t.Fatalf("wall err=%v", err)
	}
	if after := f.reload(t, "wall"); after.Revision != before.Revision || after.Round != 10 {
		t.Fatalf("wall changed %+v", after)
	}
}

func TestNativeMap1_OnlyTheActingJob9OpensWithoutKey(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map1Hero("thief", "job-09"))
	f.placeNative(t, dungeon.NativeMap1ID, "thief", 1, 4, 0, 1)
	if _, err := f.svc.Move(ctx, "thief", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "thief"); exp.PosX != 3 || !slices.Equal(exp.ConsumedEvents, []string{"D"}) {
		t.Fatalf("job 9 %+v", exp)
	}

	g := newNativeFixture(t, map1Hero("lead", "job-01"))
	g.chars.chars["ally"] = map1Hero("ally", "job-09")
	g.startNativeParty(t, dungeon.NativeMap1ID, 1, 4, 0)
	if _, err := g.svc.Move(ctx, "lead", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := g.reload(t, "lead"); exp.PosX != 4 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("ally job 9 opened the door for the actor %+v", exp)
	}
}

func TestNativeMap1_LockedDoorAtRoundLimit(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map1Hero("limit", "job-01"))
	f.placeNative(t, dungeon.NativeMap1ID, "limit", 1, 4, 0, 49)
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionWest); err != nil {
		t.Fatalf("round 49 -> 50: %v", err)
	}
	before := f.reload(t, "limit")
	if before.Round != 50 || before.PosX != 4 || before.Status != dungeon.StatusExploring {
		t.Fatalf("at limit %+v", before)
	}
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionSouth); !errors.Is(err, dungeon.ErrMovementExhausted) {
		t.Fatalf("limit err=%v", err)
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || f.rng.calls() != 0 || !after.Controls("limit").CanEscape {
		t.Fatalf("rejected move changed state %+v", after)
	}
}

// Variant 3: 3 at (8,0) and (0,8), reached through hidden passages, share one flag; 2 at (8,8) below the boss.
func TestNativeMap1_TreasureSymbolsAreIndependentOnceTriggers(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map1Hero("chest", "job-01"))
	f.placeNative(t, dungeon.NativeMap1ID, "chest", 3, 8, 1, 1)
	if res, err := f.svc.Move(ctx, "chest", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("first 3 %+v err=%v", res, err)
	}
	clear := func(x, y int) {
		exp := f.reload(t, "chest")
		exp.Encounter = nil // stands in for opening every chest (#2119)
		exp.PosX, exp.PosY = x, y
		if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
			t.Fatal(err)
		}
	}
	clear(1, 8)
	if res, err := f.svc.Move(ctx, "chest", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventMove || res.Expedition.Encounter != nil {
		t.Fatalf("second 3 %+v err=%v", res, err)
	}
	clear(8, 7)
	if res, err := f.svc.Move(ctx, "chest", dungeon.DirectionSouth); err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("2 %+v err=%v", res, err)
	}
	if exp := f.reload(t, "chest"); !slices.Equal(exp.ConsumedEvents, []string{"3", "2"}) {
		t.Fatalf("treasure keys %v", exp.ConsumedEvents)
	}
}

func TestNativeMap1_ObservationFollowsKeyAndDoorState(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map1Hero("look", "job-01"))
	f.placeNative(t, dungeon.NativeMap1ID, "look", 1, 8, 1, 1)
	tile := func(row, col int) string {
		t.Helper()
		before := f.reload(t, "look")
		view, err := f.svc.ObserveExpedition(ctx, "look")
		if err != nil {
			t.Fatal(err)
		}
		if after := f.reload(t, "look"); after.Revision != before.Revision || f.rng.calls() != 0 {
			t.Fatal("observation mutated the run or drew RNG")
		}
		return view.Map.Tiles[row][col]
	}
	if got := tile(0, 1); got != "★" {
		t.Fatalf("key before pickup %q", got)
	}
	if _, err := f.svc.Move(ctx, "look", dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, "look", 8, 1)
	if got := tile(0, 1); got != "□" {
		t.Fatalf("key after pickup %q", got)
	}
	f.moveTo(t, "look", 4, 0)
	if got := tile(1, 0); got != "扉" {
		t.Fatalf("closed door %q", got)
	}
	if _, err := f.svc.Move(ctx, "look", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, "look", 4, 0)
	if got := tile(1, 0); got != "□" {
		t.Fatalf("open door %q", got)
	}
	// Hidden passage (1,7) looks like a wall before and after crossing it.
	f.moveTo(t, "look", 1, 6)
	if got := tile(2, 1); got != "■" {
		t.Fatalf("hidden before %q", got)
	}
	for _, dir := range []dungeon.Direction{dungeon.DirectionSouth, dungeon.DirectionSouth} {
		if _, err := f.svc.Move(ctx, "look", dir); err != nil {
			t.Fatal(err)
		}
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	if got := tile(0, 1); got != "■" {
		t.Fatalf("hidden after %q", got)
	}
}

// Key and door state survive Valkey reload, and stale requests cannot replay them.
func TestNativeMap1_ValkeyKeyAndDoorState(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenValkey(t)
	charID := "map1-vk-" + time.Now().Format("150405.000000")
	defer cleanupDungeonValkey(t, client, charID)
	store, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	rng := &scriptedRNG{ints: []int{0}}
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{charID: map1Hero(charID, "job-01")}}
	svc, err := dungeon.NewService(newMockDungeonRepo(), chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store),
		dungeon.WithRNG(rng), dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartExpedition(ctx, charID, dungeon.NativeMap1ID); err != nil {
		t.Fatal(err)
	}
	place := func(x, y int) dungeon.ActiveExpedition {
		exp, err := store.GetActiveExpedition(ctx, charID)
		if err != nil || exp == nil {
			t.Fatalf("reload %+v err=%v", exp, err)
		}
		exp.PosX, exp.PosY = x, y
		if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatal(err)
		}
		return *exp
	}
	place(8, 1)
	if _, err := svc.Move(ctx, charID, dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	stale := place(4, 0)
	if _, err := svc.Move(ctx, charID, dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetActiveExpedition(ctx, charID)
	if err != nil || got.MapVariant != 1 || got.PosX != 3 || !slices.Equal(got.ConsumedEvents, []string{"K", "D"}) {
		t.Fatalf("reload %+v err=%v", got, err)
	}
	rev := stale.Revision
	if _, err := store.Step(ctx, charID, dungeon.StepParams{ExpectedExpeditionID: got.ID, ExpectedRevision: &rev,
		NewX: 3, NewY: 0, RoundDelta: 1, EventKey: "D", Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
		t.Fatalf("stale door err=%v", err)
	}
	rev = got.Revision
	out, err := store.Step(ctx, charID, dungeon.StepParams{ExpectedExpeditionID: got.ID, ExpectedRevision: &rev,
		NewX: 3, NewY: 0, RoundDelta: 1, EventKey: "K", Now: time.Now().UTC()})
	if err != nil || out.EventConsumed || !slices.Equal(out.Expedition.ConsumedEvents, []string{"K", "D"}) {
		t.Fatalf("key replay %+v err=%v", out, err)
	}
}
