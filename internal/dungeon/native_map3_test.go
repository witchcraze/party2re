package dungeon_test

import (
	"context"
	"encoding/json"
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

func nativeMap3Def(t *testing.T) dungeon.Dungeon {
	t.Helper()
	for _, d := range dungeon.DefaultDungeonCatalog() {
		if d.ID == dungeon.NativeMap3ID {
			return d
		}
	}
	t.Fatal("native map3 missing from default catalog")
	return dungeon.Dungeon{}
}

func map3Hero(id string, hp, attack, defense int) corecharacter.Character {
	c := createTestChar(id, 30, hp, attack, defense)
	c.JobLevel = 7
	return c
}

// placeMap3 starts on the given variant, then rewrites position and round.
func (f *nativeFixture) placeMap3(t *testing.T, id string, variant, x, y, round int) {
	t.Helper()
	f.rng.ints = []int{variant - 1}
	if _, err := f.svc.StartExpedition(context.Background(), id, dungeon.NativeMap3ID); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, id)
	if exp.MapVariant != variant {
		t.Fatalf("variant %d, want %d", exp.MapVariant, variant)
	}
	exp.PosX, exp.PosY, exp.TurnsRemaining = x, y, 60-round
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
}

func cellOf(fl dungeon.Floor, sym byte) [][2]int {
	var out [][2]int
	for y, row := range fl.Grid {
		for x := range len(row) {
			if row[x] == sym {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// approach returns an open neighbour of p and the direction that enters p.
func approach(fl dungeon.Floor, p [2]int) ([2]int, dungeon.Direction) {
	for _, c := range []struct {
		dx, dy int
		dir    dungeon.Direction
	}{{-1, 0, dungeon.DirectionEast}, {1, 0, dungeon.DirectionWest}, {0, -1, dungeon.DirectionSouth}, {0, 1, dungeon.DirectionNorth}} {
		x, y := p[0]+c.dx, p[1]+c.dy
		if x >= 0 && y >= 0 && x < fl.Width && y < fl.Height && fl.Grid[y][x] != '1' {
			return [2]int{x, y}, c.dir
		}
	}
	return [2]int{-1, -1}, ""
}

func TestNativeMap3_LayoutsAndContent(t *testing.T) {
	d := nativeMap3Def(t)
	if d.RoundLimit != 60 || d.LeaderJobLevel != 7 || len(d.Variants) != 8 {
		t.Fatalf("limit=%d job=%d variants=%d", d.RoundLimit, d.LeaderJobLevel, len(d.Variants))
	}
	selfWarp, swap, eventLanding := false, false, false
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
		if len(fl.Grid) != 9 || counts['S'] != 1 || counts['M'] != 2 || counts['B'] != 2 || counts['C'] != 2 || counts['H'] != 0 || len(fl.Warps) != 6 {
			t.Fatalf("variant %d counts %v warps %d", i+1, counts, len(fl.Warps))
		}
		if len(fl.Pools) != 4 || fl.Grid[fl.StartY][fl.StartX] != 'S' {
			t.Fatalf("variant %d pools/start", i+1)
		}
		for sym, to := range fl.Warps {
			if counts[sym] != 1 || to[0] < 0 || to[1] < 0 || to[0] > 8 || to[1] > 8 || fl.Grid[to[1]][to[0]] == '1' {
				t.Fatalf("variant %d warp %c -> %v", i+1, sym, to)
			}
			from := cellOf(fl, sym)[0]
			land := fl.Grid[to[1]][to[0]]
			selfWarp = selfWarp || from == to
			if back, ok := fl.Warps[land]; ok && back == from && land != sym {
				swap = true
			}
			eventLanding = eventLanding || strings.IndexByte("MBC", land) >= 0
		}
		// Every event is reachable when entering a warp relocates the party.
		seen := map[[2]int]bool{{fl.StartX, fl.StartY}: true}
		entered := map[[2]int]bool{}
		queue := [][2]int{{fl.StartX, fl.StartY}}
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			for _, dd := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				n := [2]int{p[0] + dd[0], p[1] + dd[1]}
				if n[0] < 0 || n[1] < 0 || n[0] > 8 || n[1] > 8 || fl.Grid[n[1]][n[0]] == '1' {
					continue
				}
				entered[n] = true
				if to, ok := fl.Warps[fl.Grid[n[1]][n[0]]]; ok {
					n = to
				}
				if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		for _, sym := range []byte("MBCabcdef") {
			for _, p := range cellOf(fl, sym) {
				if !entered[p] {
					t.Fatalf("variant %d event %c at %v unreachable", i+1, sym, p)
				}
			}
		}
	}
	if !selfWarp || !swap || !eventLanding {
		t.Fatalf("layouts miss warp cases: self=%v swap=%v event=%v", selfWarp, swap, eventLanding)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), d.Variants[0].Grid[1]) || strings.Contains(string(b), "nm3-") {
		t.Fatalf("definition leaked layout or content: %s", b)
	}
}

// Every warp of every variant relocates exactly to its binding, spends one
// round, draws nothing and dispatches no event at the landing cell.
func TestNativeMap3_EveryWarpBinding(t *testing.T) {
	ctx := context.Background()
	for vi, fl := range nativeMap3Def(t).Variants {
		for sym, to := range fl.Warps {
			from, dir := approach(fl, cellOf(fl, sym)[0])
			f := newNativeFixture(t, map3Hero("warp", 5000, 900, 900))
			f.placeMap3(t, "warp", vi+1, from[0], from[1], 10)
			res, err := f.svc.Move(ctx, "warp", dir)
			if err != nil {
				t.Fatalf("variant %d warp %c: %v", vi+1, sym, err)
			}
			exp := f.reload(t, "warp")
			if exp.PosX != to[0] || exp.PosY != to[1] || exp.TurnsRemaining != 49 || exp.Encounter != nil || len(exp.ConsumedEvents) != 0 ||
				res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
				t.Fatalf("variant %d warp %c -> %v: got (%d,%d) turns=%d enc=%v consumed=%v rng=%d",
					vi+1, sym, to, exp.PosX, exp.PosY, exp.TurnsRemaining, exp.Encounter, exp.ConsumedEvents, f.rng.calls())
			}
		}
	}
}

// Variant 1 warp b lands on treasure (0,8); the chest appears only on a real entry.
func TestNativeMap3_WarpLandingLeavesEventUntriggered(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("land", 5000, 900, 900))
	f.placeMap3(t, "land", 1, 3, 3, 5) // west of b at (4,3)
	if _, err := f.svc.Move(ctx, "land", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "land"); exp.PosX != 0 || exp.PosY != 8 || exp.Encounter != nil {
		t.Fatalf("landing %+v", exp)
	}
	if _, err := f.svc.Move(ctx, "land", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Move(ctx, "land", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventTreasure || !slices.Equal(res.Expedition.ConsumedEvents, []string{"C:0:8"}) {
		t.Fatalf("re-entry %+v err=%v", res, err)
	}
}

func confusedTurns(res dungeon.ExpeditionStepResult, id string) int {
	n := 0
	for _, l := range res.BattleResult.Logs {
		if l.ActorID == id && l.Kind == "confused" {
			n++
		}
	}
	return n
}

// Mist confuses the party for the ensuing battle (drawn from the drawn pool),
// the status survives reload into the continuation, and it repeats per entry.
func TestNativeMap3_MistConfusesTheEnsuingBattle(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("mist", 1_000_000, 1, 1_000_000))
	f.placeMap3(t, "mist", 1, 0, 3, 8) // south of M at (0,2)
	f.rng.ints = []int{0, 26}          // pool a, Appears[26] = monster index 8
	res, err := f.svc.Move(ctx, "mist", dungeon.DirectionNorth)
	if err != nil {
		t.Fatal(err)
	}
	if res.Monster == nil || res.Monster.ID != "nm3-a-09" || res.BattleResult == nil || res.BattleResult.Outcome != corebattle.OutcomeDraw {
		t.Fatalf("mist encounter %+v", res)
	}
	if confusedTurns(res, "mist") == 0 {
		t.Fatal("confusion did not reach the ensuing battle")
	}
	exp := f.reload(t, "mist")
	if exp.Encounter == nil || exp.Encounter.Statuses["mist"] != string(corebattle.StatusConfusion) || exp.TurnsRemaining != 51 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("pending mist encounter %+v", exp)
	}
	res, err = f.svc.Fight(ctx, "mist")
	if err != nil || confusedTurns(res, "mist") == 0 || res.Expedition.TurnsRemaining != 51 {
		t.Fatalf("resumed battle lost confusion: %+v err=%v", res, err)
	}

	// A fresh party clears the mist and re-enters: it fires again.
	g := newNativeFixture(t, map3Hero("again", 5000, 900, 900))
	g.placeMap3(t, "again", 1, 0, 3, 8)
	for range 2 {
		if res, err := g.svc.Move(ctx, "again", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventBattle || confusedTurns(res, "again") == 0 {
			t.Fatalf("mist entry %+v err=%v", res, err)
		}
		if _, err := g.svc.Move(ctx, "again", dungeon.DirectionSouth); err != nil {
			t.Fatal(err)
		}
	}
	if exp := g.reload(t, "again"); len(exp.ConsumedEvents) != 0 {
		t.Fatalf("mist must not be once-only: %v", exp.ConsumedEvents)
	}
}

// An ordinary passage encounter carries no confusion.
func TestNativeMap3_PassageEncounterUsesPoolWithoutConfusion(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("walk", 5000, 900, 900))
	f.placeMap3(t, "walk", 1, 0, 0, 3)
	f.rng.floats, f.rng.ints = []float64{0.5}, []int{2, 10} // spawn; pool c is uniform
	res, err := f.svc.Move(ctx, "walk", dungeon.DirectionEast)
	if err != nil || res.Monster == nil || res.Monster.ID != "nm3-c-11" || confusedTurns(res, "walk") != 0 {
		t.Fatalf("passage %+v err=%v", res, err)
	}
}

// Both bosses trigger once each, from the pool drawn by the triggering move.
func TestNativeMap3_TwoBossesEachTriggerOnce(t *testing.T) {
	ctx := context.Background()
	hero := map3Hero("boss", 1_000_000, 100_000, 100_000)
	hero.Stats.Agility = 10_000 // scripted 0.99 rolls must still land on agile bosses
	f := newNativeFixture(t, hero)
	f.placeMap3(t, "boss", 1, 8, 2, 4) // below B at (8,1)
	f.rng.ints = []int{3}
	res, err := f.svc.Move(ctx, "boss", dungeon.DirectionNorth)
	if err != nil || res.EventType != dungeon.EventBoss || res.Monster.ID != "nm3-d-boss" {
		t.Fatalf("first boss %+v err=%v", res, err)
	}
	if _, err := f.svc.Move(ctx, "boss", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "boss", dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("boss revisit %+v err=%v", res, err)
	}
	exp := f.reload(t, "boss")
	exp.PosX, exp.PosY = 5, 7 // west of B at (6,7)
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints = []int{0}
	if res, err = f.svc.Move(ctx, "boss", dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventBoss || res.Monster.ID != "nm3-a-boss" {
		t.Fatalf("second boss %+v err=%v", res, err)
	}
	if exp := f.reload(t, "boss"); !slices.Equal(exp.ConsumedEvents, []string{"B:8:1", "B:6:7"}) || exp.Status != dungeon.StatusExploring {
		t.Fatalf("boss state %+v", exp)
	}
}

func TestNativeMap3_TreasuresAreIndependentOnceTriggers(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("chest", 5000, 900, 900))
	f.placeMap3(t, "chest", 1, 5, 0, 4) // west of C at (6,0)
	res, err := f.svc.Move(ctx, "chest", dungeon.DirectionEast)
	if err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("chest %+v err=%v", res, err)
	}
	exp := f.reload(t, "chest")
	exp.Encounter = nil // stands in for opening every chest (#2119)
	exp.PosX, exp.PosY = 1, 8
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "chest", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("second chest %+v err=%v", res, err)
	}
	if exp := f.reload(t, "chest"); !slices.Equal(exp.ConsumedEvents, []string{"C:6:0", "C:0:8"}) {
		t.Fatalf("treasure keys %v", exp.ConsumedEvents)
	}
}

func TestNativeMap3_RoundLimitRejectsWarpWithoutChange(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("limit", 5000, 900, 900))
	f.placeMap3(t, "limit", 1, 3, 3, 59)
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionEast); err != nil {
		t.Fatalf("round 59 -> 60 warp: %v", err)
	}
	exp := f.reload(t, "limit")
	if exp.TurnsRemaining != 0 || exp.PosX != 0 || exp.PosY != 8 || exp.Status != dungeon.StatusExploring {
		t.Fatalf("at limit %+v", exp)
	}
	exp.PosX, exp.PosY = 3, 3
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionEast); !errors.Is(err, dungeon.ErrMovementExhausted) {
		t.Fatalf("limit warp err=%v", err)
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || after.PosX != 3 || f.rng.calls() != 0 {
		t.Fatalf("rejected warp changed state %+v rng=%d", after, f.rng.calls())
	}
}

func TestNativeMap3_LeaderJobLevelAdmission(t *testing.T) {
	ctx := context.Background()
	low := map3Hero("low", 5000, 900, 900)
	low.JobLevel = 6
	f := newNativeFixture(t, low)
	if _, err := f.svc.StartExpedition(ctx, "low", dungeon.NativeMap3ID); !errors.Is(err, dungeon.ErrJobLevelRequired) {
		t.Fatalf("job level 6 err=%v", err)
	}
	if run, _ := f.store.GetActiveExpedition(ctx, "low"); run != nil {
		t.Fatal("rejected start published a run")
	}
	if _, err := f.svc.StartExpedition(ctx, "low", dungeon.NativeMap0ID); err != nil {
		t.Fatalf("map0 is open to every job level: %v", err)
	}
	ids := func(ds []dungeon.Dungeon) (out []string) {
		for _, d := range ds {
			out = append(out, d.ID)
		}
		return out
	}
	if got := ids(f.svc.ListDungeons(6)); !slices.Equal(got, []string{dungeon.NativeMap0ID}) {
		t.Fatalf("job level 6 listing %v", got)
	}
	if got := ids(f.svc.ListDungeons(7)); !slices.Equal(got, []string{dungeon.NativeMap0ID, dungeon.NativeMap3ID}) {
		t.Fatalf("job level 7 listing %v", got)
	}

	// The leader is gated; a low-level joiner is not.
	g := newNativeFixture(t, map3Hero("lead", 5000, 900, 900))
	g.chars.chars["low"] = low
	if _, err := g.svc.StartPartyExpedition(ctx, "lead", []string{"lead", "low"}, dungeon.NativeMap3ID, "p"); err != nil {
		t.Fatalf("party with low joiner: %v", err)
	}
	h := newNativeFixture(t, low)
	h.chars.chars["lead"] = map3Hero("lead", 5000, 900, 900)
	if _, err := h.svc.StartPartyExpedition(ctx, "low", []string{"low", "lead"}, dungeon.NativeMap3ID, "p"); !errors.Is(err, dungeon.ErrJobLevelRequired) {
		t.Fatalf("low leader err=%v", err)
	}
}

// The confused continuation survives reload, and a stale competitor cannot overwrite it.
func TestNativeMap3_ConfusedContinuationStoreContract(t *testing.T) {
	run := func(t *testing.T, store dungeon.ActiveExpeditionStore, charID string) {
		ctx := context.Background()
		exp := sampleExpedition("map3-"+charID, charID, dungeon.NativeMap3ID)
		exp.MapVariant, exp.TurnsRemaining = 5, 30
		if err := store.SaveActiveExpedition(ctx, exp); err != nil {
			t.Fatal(err)
		}
		got, err := store.GetActiveExpedition(ctx, charID)
		if err != nil {
			t.Fatal(err)
		}
		rev := got.Revision
		enemy := corebattle.MustNewParticipant("nm3-b-02", 50, 1, 1)
		enemy.TeamID = "enemies"
		enc := &dungeon.ActiveEncounter{EventType: dungeon.EventBattle, X: 0, Y: 7, Enemies: []corebattle.Participant{enemy},
			Statuses: map[string]string{charID: string(corebattle.StatusConfusion)}}
		step := func(rev int, x int) error {
			_, err := store.Step(ctx, charID, dungeon.StepParams{ExpectedExpeditionID: exp.ID, ExpectedRevision: &rev, Combat: true,
				NewX: x, NewY: 7, TurnsDelta: -1, Encounter: enc, Now: time.Now().UTC()})
			return err
		}
		if err := step(rev, 0); err != nil {
			t.Fatal(err)
		}
		if err := step(rev, 1); !errors.Is(err, dungeon.ErrExpeditionStateChanged) && !errors.Is(err, dungeon.ErrCombatActive) {
			t.Fatalf("stale step err=%v", err)
		}
		got, err = store.GetActiveExpedition(ctx, charID)
		if err != nil || got.PosX != 0 || got.TurnsRemaining != 29 || got.MapVariant != 5 || got.Encounter == nil ||
			got.Encounter.Statuses[charID] != string(corebattle.StatusConfusion) {
			t.Fatalf("reload %+v err=%v", got, err)
		}
	}
	t.Run("memory", func(t *testing.T) { run(t, dungeon.NewMemoryExpeditionRepository(), "map3-mem") })
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		charID := "map3-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, charID)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		run(t, store, charID)
	})
}

// Observation shows warps, mist, bosses and chests as plain passage and mutates nothing.
func TestNativeMap3_ObservationMasksEvents(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("look", 5000, 900, 900))
	f.placeMap3(t, "look", 2, 4, 4, 3) // start, between warps c and d
	before := f.reload(t, "look")
	view, err := f.svc.ObserveExpedition(ctx, "look")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"nm3-", "warp", "consumed", "grid"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("observation leaked %q: %s", leak, b)
		}
	}
	if tiles := view.Map.Tiles; tiles[1][0] != "□" || tiles[1][2] != "□" || tiles[1][1] != "●" {
		t.Fatalf("warp cells must look like passage: %v", tiles)
	}
	if after := f.reload(t, "look"); after.Revision != before.Revision || f.rng.calls() != 0 {
		t.Fatal("observation mutated the run or drew RNG")
	}
}
