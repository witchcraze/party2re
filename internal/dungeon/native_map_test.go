package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

// scriptedRNG replays fixed draws and counts every draw.
type scriptedRNG struct {
	ints       []int
	floats     []float64
	intCalls   int
	floatCalls int
}

func (r *scriptedRNG) Intn(n int) int {
	r.intCalls++
	if len(r.ints) == 0 || n <= 0 {
		return 0
	}
	v := r.ints[0]
	r.ints = r.ints[1:]
	return v % n
}

func (r *scriptedRNG) Float64() float64 {
	r.floatCalls++
	if len(r.floats) == 0 {
		return 0.99 // passage roll 1.98: no spawn
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

func (r *scriptedRNG) calls() int { return r.intCalls + r.floatCalls }

func nativeMap0Def(t *testing.T) dungeon.Dungeon {
	t.Helper()
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap0ID {
			return d
		}
	}
	t.Fatal("native map0 missing from default catalog")
	return dungeon.Dungeon{}
}

type nativeFixture struct {
	svc   *dungeon.Service
	store *dungeon.MemoryExpeditionRepository
	repo  *mockDungeonRepo
	chars *mockCharRepo
	rng   *scriptedRNG
}

func newNativeFixture(t *testing.T, hero corecharacter.Character) *nativeFixture {
	t.Helper()
	f := &nativeFixture{
		store: dungeon.NewMemoryExpeditionRepository(),
		repo:  newMockDungeonRepo(),
		chars: &mockCharRepo{chars: map[string]corecharacter.Character{hero.ID: hero}},
		rng:   &scriptedRNG{},
	}
	svc, err := dungeon.NewService(f.repo, f.chars, corebattle.Engine{},
		dungeon.WithActiveExpeditionStore(f.store), dungeon.WithRNG(f.rng),
		dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	return f
}

// place starts on variant 1 and then rewrites position and round; variant 1:
//
//	S0001
//	110H1
//	0B000
//	01110
//	00C10
func (f *nativeFixture) place(t *testing.T, id string, x, y, round int) dungeon.ActiveExpedition {
	t.Helper()
	ctx := context.Background()
	if _, err := f.svc.StartExpedition(ctx, id, dungeon.NativeMap0ID); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, id)
	exp.PosX, exp.PosY, exp.TurnsRemaining = x, y, 20-round
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	return exp
}

func (f *nativeFixture) reload(t *testing.T, id string) dungeon.ActiveExpedition {
	t.Helper()
	exp, err := f.svc.GetActiveExpedition(context.Background(), id)
	if err != nil || exp == nil {
		t.Fatalf("reload: %v %v", exp, err)
	}
	return *exp
}

func strongHero(id string) corecharacter.Character {
	return createTestChar(id, 30, 5000, 900, 900)
}

func TestNativeMap0_LayoutsAreIndependentFiveByFiveVariants(t *testing.T) {
	d := nativeMap0Def(t)
	if d.RoundLimit != 20 || len(d.Variants) != 6 || len(d.Floors) != 0 {
		t.Fatalf("limit=%d variants=%d floors=%d", d.RoundLimit, len(d.Variants), len(d.Floors))
	}
	for i, f := range d.Variants {
		counts := map[byte]int{}
		for _, row := range f.Grid {
			if len(row) != 5 {
				t.Fatalf("variant %d row %q is not 5 wide", i+1, row)
			}
			for j := range len(row) {
				counts[row[j]]++
			}
		}
		if len(f.Grid) != 5 || f.Width != 5 || f.Height != 5 || counts['S'] != 1 || counts['B'] != 1 || counts['C'] != 1 {
			t.Fatalf("variant %d counts %v", i+1, counts)
		}
		if hidden := counts['H'] > 0; hidden != (i < 4) {
			t.Fatalf("variant %d hidden passage presence %v", i+1, hidden)
		}
		if f.Grid[f.StartY][f.StartX] != 'S' || f.Boss == nil || len(f.Monsters) == 0 {
			t.Fatalf("variant %d start/content invalid", i+1)
		}
		// Boss and treasure are reachable through passable cells.
		seen := map[[2]int]bool{{f.StartX, f.StartY}: true}
		queue := [][2]int{{f.StartX, f.StartY}}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := [2]int{p[0] + d[0], p[1] + d[1]}
				if n[0] < 0 || n[1] < 0 || n[0] > 4 || n[1] > 4 || seen[n] || f.Grid[n[1]][n[0]] == '1' {
					continue
				}
				seen[n] = true
				queue = append(queue, n)
			}
		}
		for y, row := range f.Grid {
			for x := range len(row) {
				if (row[x] == 'B' || row[x] == 'C') && !seen[[2]int{x, y}] {
					t.Fatalf("variant %d event %c at %d,%d unreachable", i+1, row[x], x, y)
				}
			}
		}
	}
	// Listing never discloses variant grids or hidden passages.
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), d.Variants[0].Grid[1]) || strings.Contains(string(b), "variants") {
		t.Fatalf("native definition leaked layout: %s", b)
	}
}

func TestNativeMap0_StartSelectsSavedVariantAtRoundOne(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, strongHero("start"))
	f.rng.ints = []int{3}
	exp, err := f.svc.StartExpedition(ctx, "start", dungeon.NativeMap0ID)
	if err != nil {
		t.Fatal(err)
	}
	v4 := nativeMap0Def(t).Variants[3]
	got := f.reload(t, "start")
	if exp.MapVariant != 4 || got.MapVariant != 4 || got.PosX != v4.StartX || got.PosY != v4.StartY || len(got.ConsumedEvents) != 0 || got.Encounter != nil {
		t.Fatalf("start state %+v", got)
	}
	view, err := f.svc.ObserveExpedition(ctx, "start")
	if err != nil {
		t.Fatal(err)
	}
	if view.Round != 1 || view.RoundLimit != 20 || !view.Controls.CanMove || len(view.Directions) != 4 {
		t.Fatalf("observation %+v", view)
	}
	if again := f.reload(t, "start"); again.MapVariant != 4 || again.Revision != got.Revision {
		t.Fatalf("observation changed the run: %+v", again)
	}
}

func TestNativeMap0_RoundLimitAndWalls(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, strongHero("limit"))
	f.place(t, "limit", 1, 4, 19)

	// A wall/bound spends no round and draws nothing.
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionSouth); !errors.Is(err, dungeon.ErrImpassableWall) {
		t.Fatalf("bound move err=%v", err)
	}
	if after := f.reload(t, "limit"); after.TurnsRemaining != before.TurnsRemaining || after.Revision != before.Revision || f.rng.calls() != 0 {
		t.Fatalf("wall changed state %+v rng=%d", after, f.rng.calls())
	}

	// Round 19 → 20 dispatches the destination (the treasure trigger).
	res, err := f.svc.Move(ctx, "limit", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	if res.EventType != dungeon.EventTreasure || res.Expedition.TurnsRemaining != 0 || res.Expedition.Status != dungeon.StatusExploring {
		t.Fatalf("round 20 step %+v", res)
	}

	// Release the chest so only the limit applies, then every move rejects unchanged.
	exp := f.reload(t, "limit")
	exp.Encounter.Chests[0].Opened = true
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	for _, dir := range []dungeon.Direction{dungeon.DirectionNorth, dungeon.DirectionSouth, dungeon.DirectionEast, dungeon.DirectionWest} {
		if _, err := f.svc.Move(ctx, "limit", dir); !errors.Is(err, dungeon.ErrMovementExhausted) {
			t.Fatalf("%s at limit err=%v", dir, err)
		}
	}
	after := f.reload(t, "limit")
	if after.Revision != exp.Revision || after.PosX != 2 || after.PosY != 4 || len(after.ConsumedEvents) != 1 || after.Status != dungeon.StatusExploring || f.rng.calls() != 0 {
		t.Fatalf("limit rejection changed state %+v rng=%d", after, f.rng.calls())
	}
	if after.Controls("limit").CanMove || len(f.repo.histories["limit"]) != 0 {
		t.Fatal("exhaustion must neither allow movement nor settle")
	}
}

func TestNativeMap0_PassageEncounterBoundary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		half  float64 // Float64 sample; r = 2 × sample
		spawn bool
	}{
		{"zero", 0, true},
		{"just below one", 0.4999999, true},
		{"exactly one", 0.5, true},
		{"just above one", 0.5000001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, strongHero("pass"))
			f.place(t, "pass", 0, 0, 1)
			f.rng.floats = []float64{tc.half}
			res, err := f.svc.Move(context.Background(), "pass", dungeon.DirectionEast)
			if err != nil {
				t.Fatal(err)
			}
			if spawned := res.EventType == dungeon.EventBattle; spawned != tc.spawn {
				t.Fatalf("spawned=%v result=%+v", spawned, res)
			}
			if !tc.spawn && f.rng.calls() != 1 {
				t.Fatalf("non-spawn draws=%d", f.rng.calls())
			}
			if got := f.reload(t, "pass"); got.PosX != 1 || got.TurnsRemaining != 18 {
				t.Fatalf("passage step %+v", got)
			}
		})
	}
}

func TestNativeMap0_NonPassageEventsDrawNoPassageRNG(t *testing.T) {
	for _, tc := range []struct {
		name     string
		x, y     int
		dir      dungeon.Direction
		consumed []string
	}{
		{"start", 1, 0, dungeon.DirectionWest, nil},
		{"hidden", 3, 0, dungeon.DirectionSouth, nil},
		{"consumed treasure", 1, 4, dungeon.DirectionEast, []string{"C:2:4"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, strongHero("quiet"))
			exp := f.place(t, "quiet", tc.x, tc.y, 5)
			exp.ConsumedEvents = tc.consumed
			if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
				t.Fatal(err)
			}
			res, err := f.svc.Move(context.Background(), "quiet", tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			if res.EventType != dungeon.EventMove || res.Expedition.Encounter != nil || f.rng.calls() != 0 || res.Expedition.TurnsRemaining != 14 {
				t.Fatalf("result %+v rng=%d", res, f.rng.calls())
			}
		})
	}
}

func TestNativeMap0_TreasureBlocksMovementUntilEveryChestIsOpened(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, strongHero("chest"))
	start := f.place(t, "chest", 1, 4, 3)

	res, err := f.svc.Move(ctx, "chest", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "chest")
	if res.EventType != dungeon.EventTreasure || exp.Encounter == nil || len(exp.Encounter.Chests) != 1 ||
		exp.AccumulatedGold != start.AccumulatedGold || exp.AccumulatedMedals != 0 || len(exp.AccumulatedItems) != 0 ||
		len(exp.ConsumedEvents) != 1 || exp.ConsumedEvents[0] != "C:2:4" || exp.TurnsRemaining != 16 {
		t.Fatalf("treasure trigger %+v", exp)
	}

	assertBlocked := func(label string) {
		t.Helper()
		before := f.reload(t, "chest")
		f.rng.intCalls, f.rng.floatCalls = 0, 0
		view, err := f.svc.ObserveExpedition(ctx, "chest")
		if err != nil {
			t.Fatal(err)
		}
		if view.Controls.CanMove || view.Controls.CanFight || !view.Controls.CanEscape || len(view.Directions) != 0 {
			t.Fatalf("%s: controls %+v directions %v", label, view.Controls, view.Directions)
		}
		for _, dir := range []dungeon.Direction{dungeon.DirectionNorth, dungeon.DirectionSouth, dungeon.DirectionEast, dungeon.DirectionWest} {
			if _, err := f.svc.Move(ctx, "chest", dir); !errors.Is(err, dungeon.ErrCombatActive) {
				t.Fatalf("%s: %s err=%v", label, dir, err)
			}
		}
		if _, err := f.svc.Fight(ctx, "chest"); !errors.Is(err, dungeon.ErrNoActiveCombat) {
			t.Fatalf("%s: treasure resolved as combat: %v", label, err)
		}
		after := f.reload(t, "chest")
		if after.Revision != before.Revision || after.PosX != before.PosX || after.TurnsRemaining != before.TurnsRemaining ||
			len(after.ConsumedEvents) != len(before.ConsumedEvents) || f.rng.calls() != 0 {
			t.Fatalf("%s: rejection changed state %+v rng=%d", label, after, f.rng.calls())
		}
	}
	assertBlocked("single chest")

	// Partial opening of several chests still blocks; the last one releases.
	exp = f.reload(t, "chest")
	exp.Encounter.Chests = []dungeon.TreasureChest{{Opened: true}, {}}
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	assertBlocked("partially opened")
	exp = f.reload(t, "chest")
	exp.Encounter.Chests[1].Opened = true
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	res, err = f.svc.Move(ctx, "chest", dungeon.DirectionWest)
	if err != nil {
		t.Fatal(err)
	}
	if res.Expedition.Encounter != nil || res.Expedition.PosX != 1 || res.Expedition.TurnsRemaining != 15 {
		t.Fatalf("released move %+v", res.Expedition)
	}

	// Revisiting never regenerates the treasure.
	f.rng.floats = []float64{0.99}
	if res, err = f.svc.Move(ctx, "chest", dungeon.DirectionEast); err != nil || res.Expedition.Encounter != nil || len(res.Expedition.ConsumedEvents) != 1 {
		t.Fatalf("revisit %+v err=%v", res.Expedition, err)
	}

	// Escape stays available behind unopened chests and keeps the stock.
	g := newNativeFixture(t, strongHero("flee"))
	g.place(t, "flee", 1, 4, 3)
	if _, err := g.svc.Move(ctx, "flee", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if out, err := g.svc.Escape(ctx, "flee"); err != nil || out.Expedition.Status != dungeon.StatusEscaped {
		t.Fatalf("escape behind chest %+v err=%v", out, err)
	}
}

func TestNativeMap0_TreasureCountFollowsLivingRosterAndJob78(t *testing.T) {
	ctx := context.Background()
	lead := strongHero("hunter")
	lead.JobID = "job-78"
	f := newNativeFixture(t, lead)
	f.chars.chars["mate"] = strongHero("mate")
	f.chars.chars["fallen"] = strongHero("fallen")
	if _, err := f.svc.StartPartyExpedition(ctx, "hunter", []string{"hunter", "mate", "fallen"}, dungeon.NativeMap0ID, "p"); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "hunter")
	exp.PosX, exp.PosY, exp.MapVariant = 1, 4, 1
	exp.Members[2].CurrentHP = 0
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints = []int{1} // job 78: 1 + rand(2) = 2 extra
	res, err := f.svc.Move(ctx, "hunter", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Expedition.Encounter.Chests); got != 4 {
		t.Fatalf("chests=%d, want 2 living + 2", got)
	}
	if got := len(f.reload(t, "hunter").Encounter.Chests); got != 4 {
		t.Fatalf("reloaded chests=%d", got)
	}
}

func TestNativeMap0_BossTriggersOnceWithoutClearing(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, strongHero("boss"))
	f.place(t, "boss", 0, 2, 4)
	boss := nativeMap0Def(t).Variants[0].Boss

	res, err := f.svc.Move(ctx, "boss", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "boss")
	if res.EventType != dungeon.EventBoss || exp.Status != dungeon.StatusExploring || exp.Encounter != nil ||
		exp.AccumulatedExp != boss.ExpReward || exp.AccumulatedMedals != 0 || len(exp.ConsumedEvents) != 1 || exp.ConsumedEvents[0] != "B:1:2" {
		t.Fatalf("boss victory %+v / %+v", res, exp)
	}
	if len(f.repo.histories["boss"]) != 0 {
		t.Fatal("boss victory must not finalize the expedition")
	}
	if _, err := f.svc.Move(ctx, "boss", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "boss", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventMove || res.Expedition.Encounter != nil {
		t.Fatalf("boss revisit %+v err=%v", res, err)
	}
}

func TestNativeMap0_BossDrawResumesWithoutSpendingRounds(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, createTestChar("stall", 30, 1_000_000, 1, 1_000_000))
	f.place(t, "stall", 0, 2, 4)
	res, err := f.svc.Move(ctx, "stall", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "stall")
	if res.BattleResult == nil || res.BattleResult.Outcome != corebattle.OutcomeDraw || exp.Encounter == nil || exp.Encounter.EventType != dungeon.EventBoss {
		t.Fatalf("expected a pending boss draw, got %+v", res)
	}
	if exp.PosX != 1 || exp.PosY != 2 || exp.TurnsRemaining != 15 || len(exp.ConsumedEvents) != 1 {
		t.Fatalf("boss trigger state %+v", exp)
	}
	if !exp.Controls("stall").CanFight || exp.Controls("stall").CanMove {
		t.Fatalf("boss continuation controls %+v", exp.Controls("stall"))
	}
	if _, err := f.svc.Fight(ctx, "stall"); err != nil {
		t.Fatal(err)
	}
	if after := f.reload(t, "stall"); after.TurnsRemaining != 15 || after.Status != dungeon.StatusExploring || len(after.ConsumedEvents) != 1 {
		t.Fatalf("resume changed rounds or cleared: %+v", after)
	}
}

func TestNativeMap0_HiddenPassageAlwaysLooksLikeAWall(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, strongHero("seek"))
	f.place(t, "seek", 3, 0, 2)
	observe := func() [][]string {
		t.Helper()
		before := f.reload(t, "seek")
		view, err := f.svc.ObserveExpedition(ctx, "seek")
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, leak := range []string{"variant", "consumed", "H:", "grid", "chests"} {
			if strings.Contains(string(b), leak) {
				t.Fatalf("observation leaked %q: %s", leak, b)
			}
		}
		if after := f.reload(t, "seek"); after.Revision != before.Revision {
			t.Fatal("observation mutated the run")
		}
		return view.Map.Tiles
	}
	// Radius 1 around (3,0): row 0 is out of bounds, (3,1) below is hidden.
	if tiles := observe(); tiles[2][1] != "■" || tiles[0][1] != "■" || tiles[1][1] != "●" {
		t.Fatalf("before traversal %v", tiles)
	}
	if _, err := f.svc.Move(ctx, "seek", dungeon.DirectionSouth); err != nil {
		t.Fatalf("hidden passage is traversable: %v", err)
	}
	if tiles := observe(); tiles[1][1] != "●" || tiles[2][1] != "□" {
		t.Fatalf("on hidden passage %v", tiles)
	}
	if _, err := f.svc.Move(ctx, "seek", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if tiles := observe(); tiles[0][1] != "■" {
		t.Fatalf("after traversal %v", tiles)
	}
}

func TestNativeMap0_EscapeAtLimitSettlesStockOnce(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, strongHero("exit"))
	exp := f.place(t, "exit", 2, 4, 20)
	exp.AccumulatedGold, exp.AccumulatedExp = 120, 40
	exp.Members[0].AccumulatedGold, exp.Members[0].AccumulatedExp = 120, 40
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if !exp.Controls("exit").CanEscape {
		t.Fatal("escape must remain available at the limit")
	}
	res, err := f.svc.Escape(ctx, "exit")
	if err != nil || res.Expedition.Status != dungeon.StatusEscaped {
		t.Fatalf("escape %+v err=%v", res, err)
	}
	if _, err := f.svc.Escape(ctx, "exit"); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Fatalf("repeated escape err=%v", err)
	}
	h := f.repo.histories["exit"]
	if len(h) != 1 || h[0].Outcome != dungeon.StatusEscaped || h[0].GoldReward != 120 || h[0].ExpReward != 40 {
		t.Fatalf("history %+v", h)
	}
}
