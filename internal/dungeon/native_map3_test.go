package dungeon_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

func map3Hero(id string, hp, attack, defense int) corecharacter.Character {
	c := createTestChar(id, 30, hp, attack, defense)
	c.JobLevel = 7
	return c
}

func TestNativeMap3_LayoutsAndContent(t *testing.T) {
	// Raw map/3/1.cgi–8.cgi grids, in variant order.
	legacy := []string{
		"F0000000D 0101C1010 003101b00 011101110 0A00S0000 011101110 0TE101200 010101T10 B0000000G",
		"b0000000F 010101010 00A1013T0 011101110 0000S00D0 011101110 0T2101B00 0101E1010 C0000000G",
		"A00000003 01T101010 00b101F00 011101110 0000S00D0 011101110 00C101BT0 0101E1010 20000000G",
		"A0000000F 010101010 00b1013T0 011101110 0000S00D0 011101110 002101G00 0101E1T10 C0000000B",
		"T00000000 010101010 0031C1B00 011101110 00A0S0D00 011101110 00b1E12T0 010101010 F0000000G",
		"F0000000G 010101010 0Tb1C1200 011101110 00A0S0D00 011101110 0031E1B00 010101010 00000000T",
		"F00000002 010101T10 00b1C1G00 011101110 00A0S0D00 011101110 0031E1000 010101010 T0000000B",
		"b0000000G 01T101010 00F1C1200 011101110 00A0S0D00 011101110 0031E1B00 010101010 00000000T",
	}
	nativeCells := strings.NewReplacer("A", "a", "C", "c", "D", "d", "E", "e", "F", "f", "G", "g", "T", "M")
	d := mapDef(t, dungeon.NativeMap3ID)
	if d.RoundLimit != 60 || d.LeaderJobLevel != 7 || len(d.Variants) != len(legacy) {
		t.Fatalf("limit=%d job=%d variants=%d", d.RoundLimit, d.LeaderJobLevel, len(d.Variants))
	}
	for i, fl := range d.Variants {
		want := strings.Fields(nativeCells.Replace(legacy[i]))
		if !slices.Equal(fl.Grid, want) || fl.Width != 9 || fl.Height != 9 {
			t.Fatalf("variant %d grid %v (%dx%d), want %v (9x9)", i+1, fl.Grid, fl.Width, fl.Height, want)
		}
		if len(cellOf(fl, 'I')) != 0 || len(cellOf(fl, 'H')) != 0 {
			t.Fatalf("variant %d contains an unreachable legacy hidden-passage event", i+1)
		}
		if len(fl.Pools) != 4 || fl.Grid[fl.StartY][fl.StartX] != 'S' {
			t.Fatalf("variant %d pools/start", i+1)
		}
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), d.Variants[0].Grid[1]) || strings.Contains(string(b), "nm3-") {
		t.Fatalf("definition leaked layout or content: %s", b)
	}
}

func TestNativeMapNamesMatchAdventureCatalog(t *testing.T) {
	catalog, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	assertName := func(m dungeon.DungeonMonster, catalogNumber int, approvedName string) {
		t.Helper()
		entry, err := catalog.FindByID(fmt.Sprintf("monster-%03d", catalogNumber))
		if err != nil {
			t.Fatal(err)
		}
		want := entry.Name
		if approvedName != "" {
			want = approvedName
		}
		if m.Name != want {
			t.Errorf("%s name %q, want %q (%s)", m.ID, m.Name, want, entry.ID)
		}
	}
	assertName(*mapDef(t, dungeon.NativeMap0ID).Variants[0].Boss, 7, "")
	// Stage order, including the shared Bomb Rock in stage 8 and the
	// existing main boss of stage 7 (Firebird, between its two escorts).
	stages := []struct {
		monsters []int
		boss     int
	}{
		{[]int{48, 49, 50, 51, 52, 53, 54, 55, 56, 57}, 58},
		{[]int{61, 62, 63, 64, 65, 66, 67, 68, 69, 70, 71, 72}, 73},
		{[]int{74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84}, 86},
		{[]int{88, 89, 90, 91, 76, 92, 93, 94, 95, 96}, 97},
	}
	approvedNames := map[int]string{54: "浮遊眼", 73: "一つ目の巨人"}
	pools := mapDef(t, dungeon.NativeMap3ID).Variants[0].Pools
	if len(pools) != len(stages) {
		t.Fatalf("pools %d, want %d", len(pools), len(stages))
	}
	for i, stage := range stages {
		pool := pools[i]
		if len(pool.Monsters) != len(stage.monsters) || pool.Boss == nil {
			t.Fatalf("stage %d monsters/boss do not match", i+5)
		}
		for j, id := range stage.monsters {
			assertName(pool.Monsters[j], id, approvedNames[id])
		}
		assertName(*pool.Boss, stage.boss, approvedNames[stage.boss])
	}
}

// Every warp of every variant relocates exactly to its binding, spends one
// round, draws nothing and dispatches no event at the landing cell.
func TestNativeMap3_EveryWarpBinding(t *testing.T) {
	ctx := context.Background()
	// Coordinates are (x, y), ordered by legacy event A, C, D, E, F, G.
	want := [8][6][2]int{
		{{5, 4}, {4, 5}, {2, 6}, {8, 0}, {6, 6}, {2, 2}},
		{{8, 8}, {6, 2}, {3, 4}, {4, 3}, {2, 6}, {2, 2}},
		{{6, 6}, {8, 0}, {3, 4}, {4, 3}, {0, 8}, {2, 2}},
		{{6, 6}, {6, 2}, {3, 4}, {4, 3}, {2, 6}, {0, 0}},
		{{6, 4}, {4, 2}, {2, 4}, {4, 6}, {6, 2}, {2, 2}},
		{{6, 4}, {4, 6}, {2, 4}, {4, 2}, {6, 6}, {2, 6}},
		{{6, 4}, {4, 6}, {2, 4}, {4, 2}, {6, 6}, {0, 8}},
		{{6, 4}, {4, 6}, {2, 4}, {4, 2}, {8, 8}, {2, 6}},
	}
	for vi, fl := range mapDef(t, dungeon.NativeMap3ID).Variants {
		if len(fl.Warps) != len(want[vi]) {
			t.Fatalf("variant %d warps: %v", vi+1, fl.Warps)
		}
		for i, sym := range []byte("acdefg") {
			to := want[vi][i]
			cells := cellOf(fl, sym)
			if got, ok := fl.Warps[sym]; !ok || got != to || len(cells) != 1 {
				t.Fatalf("variant %d warp %c: cells %v, destination %v, want %v", vi+1, sym, cells, got, to)
			}
			from, dir := approach(fl, cells[0])
			f := newNativeFixture(t, map3Hero("warp", 5000, 900, 900))
			f.placeNative(t, dungeon.NativeMap3ID, "warp", vi+1, from[0], from[1], 10)
			res, err := f.svc.Move(ctx, "warp", dir)
			if err != nil {
				t.Fatalf("variant %d warp %c: %v", vi+1, sym, err)
			}
			exp := f.reload(t, "warp")
			if exp.PosX != to[0] || exp.PosY != to[1] || exp.Round != 11 || exp.Encounter != nil || len(exp.ConsumedEvents) != 0 ||
				res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
				t.Fatalf("variant %d warp %c -> %v: got (%d,%d) turns=%d enc=%v consumed=%v rng=%d",
					vi+1, sym, to, exp.PosX, exp.PosY, exp.Round, exp.Encounter, exp.ConsumedEvents, f.rng.calls())
			}
		}
	}
}

// Variant 1 warp F lands on treasure 2 at (6,6).
func TestNativeMap3_WarpLandingLeavesEventUntriggered(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("land", 5000, 900, 900))
	f.placeNative(t, dungeon.NativeMap3ID, "land", 1, 1, 0, 5)
	if _, err := f.svc.Move(ctx, "land", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "land"); exp.PosX != 6 || exp.PosY != 6 || exp.Encounter != nil {
		t.Fatalf("landing %+v", exp)
	}
	if _, err := f.svc.Move(ctx, "land", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Move(ctx, "land", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventTreasure || !slices.Equal(res.Expedition.ConsumedEvents, []string{"2"}) {
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
	f.placeNative(t, dungeon.NativeMap3ID, "mist", 1, 0, 6, 8) // west of T at (1,6)
	// pool a, Appears[26] = monster index 8
	f.rng.ints = []int{0, 26}
	res, err := f.svc.Move(ctx, "mist", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	if res.Monster == nil || res.Monster.Name != "ミミック" || res.BattleResult == nil || res.BattleResult.Outcome != corebattle.OutcomeDraw {
		t.Fatalf("mist encounter %+v", res)
	}
	if confusedTurns(res, "mist") == 0 {
		t.Fatal("confusion did not reach the ensuing battle")
	}
	exp := f.reload(t, "mist")
	if exp.Encounter == nil || exp.Encounter.Statuses["mist"] != string(corebattle.StatusConfusion) || exp.Round != 9 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("pending mist encounter %+v", exp)
	}
	res, err = f.svc.Fight(ctx, "mist")
	if err != nil || confusedTurns(res, "mist") == 0 || res.Expedition.Round != 9 {
		t.Fatalf("resumed battle lost confusion: %+v err=%v", res, err)
	}

	// A fresh party clears the mist and re-enters: it fires again.
	g := newNativeFixture(t, map3Hero("again", 5000, 900, 900))
	g.placeNative(t, dungeon.NativeMap3ID, "again", 1, 0, 6, 8)
	for range 2 {
		if res, err := g.svc.Move(ctx, "again", dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventBattle || confusedTurns(res, "again") == 0 {
			t.Fatalf("mist entry %+v err=%v", res, err)
		}
		if _, err := g.svc.Move(ctx, "again", dungeon.DirectionWest); err != nil {
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
	f.placeNative(t, dungeon.NativeMap3ID, "walk", 1, 1, 0, 3)
	f.rng.floats, f.rng.ints = []float64{0.5}, []int{2, 10} // spawn; pool c is uniform
	res, err := f.svc.Move(ctx, "walk", dungeon.DirectionEast)
	if err != nil || res.Monster == nil || res.Monster.Name != "炎の戦士" || confusedTurns(res, "walk") != 0 {
		t.Fatalf("passage %+v err=%v", res, err)
	}
}

// Both bosses trigger once each, from the pool drawn by the triggering move.
func TestNativeMap3_TwoBossesEachTriggerOnce(t *testing.T) {
	ctx := context.Background()
	hero := map3Hero("boss", 1_000_000, 100_000, 100_000)
	hero.Stats.Agility = 10_000 // scripted 0.99 rolls must still land on agile bosses
	f := newNativeFixture(t, hero)
	f.placeNative(t, dungeon.NativeMap3ID, "boss", 1, 1, 8, 4) // east of B at (0,8)
	f.rng.ints = []int{3}
	res, err := f.svc.Move(ctx, "boss", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventBoss || res.Monster.Name != "ベヒーモス" {
		t.Fatalf("first boss %+v err=%v", res, err)
	}
	if _, err := f.svc.Move(ctx, "boss", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "boss", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("boss revisit %+v err=%v", res, err)
	}
	exp := f.reload(t, "boss")
	exp.PosX, exp.PosY = 7, 2 // east of b at (6,2)
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints = []int{0}
	if res, err = f.svc.Move(ctx, "boss", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventBoss || res.Monster.Name != "闇の魔術士" {
		t.Fatalf("second boss %+v err=%v", res, err)
	}
	if _, err := f.svc.Move(ctx, "boss", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "boss", dungeon.DirectionWest); err != nil || res.EventType != dungeon.EventMove {
		t.Fatalf("second boss revisit %+v err=%v", res, err)
	}
	if exp := f.reload(t, "boss"); !slices.Equal(exp.ConsumedEvents, []string{"B:0:8", "b:6:2"}) || exp.Status != dungeon.StatusExploring {
		t.Fatalf("boss state %+v", exp)
	}
}

func TestNativeMap3_TreasuresAreIndependentOnceTriggers(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("chest", 5000, 900, 900))
	f.placeNative(t, dungeon.NativeMap3ID, "chest", 1, 7, 6, 4) // east of 2 at (6,6)
	res, err := f.svc.Move(ctx, "chest", dungeon.DirectionWest)
	if err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("chest %+v err=%v", res, err)
	}
	exp := f.reload(t, "chest")
	exp.Encounter = nil       // stands in for opening every chest (#2119)
	exp.PosX, exp.PosY = 1, 2 // west of 3 at (2,2)
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "chest", dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("second chest %+v err=%v", res, err)
	}
	if exp := f.reload(t, "chest"); !slices.Equal(exp.ConsumedEvents, []string{"2", "3"}) {
		t.Fatalf("treasure keys %v", exp.ConsumedEvents)
	}
	exp = f.reload(t, "chest")
	exp.Encounter = nil
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Move(ctx, "chest", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if res, err = f.svc.Move(ctx, "chest", dungeon.DirectionEast); err != nil || res.EventType != dungeon.EventMove || res.Expedition.Encounter != nil {
		t.Fatalf("treasure revisit %+v err=%v", res, err)
	}
}

func TestNativeMap3_RoundLimitRejectsWarpWithoutChange(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map3Hero("limit", 5000, 900, 900))
	f.placeNative(t, dungeon.NativeMap3ID, "limit", 1, 1, 0, 59)
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionWest); err != nil {
		t.Fatalf("round 59 -> 60 warp: %v", err)
	}
	exp := f.reload(t, "limit")
	if exp.Round != 60 || exp.PosX != 6 || exp.PosY != 6 || exp.Status != dungeon.StatusExploring {
		t.Fatalf("at limit %+v", exp)
	}
	exp.PosX, exp.PosY = 1, 0
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	before := f.reload(t, "limit")
	if _, err := f.svc.Move(ctx, "limit", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrMovementExhausted) {
		t.Fatalf("limit warp err=%v", err)
	}
	if after := f.reload(t, "limit"); after.Revision != before.Revision || after.PosX != 1 || f.rng.calls() != 0 {
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
	if got := ids(f.svc.ListDungeons(2)); !slices.Equal(got, []string{dungeon.NativeMap0ID}) {
		t.Fatalf("job level 2 listing %v", got)
	}
	if got := ids(f.svc.ListDungeons(4)); !slices.Equal(got, []string{dungeon.NativeMap0ID, dungeon.NativeMap1ID}) {
		t.Fatalf("job level 4 listing %v", got)
	}
	if got := ids(f.svc.ListDungeons(6)); !slices.Equal(got, []string{dungeon.NativeMap0ID, dungeon.NativeMap1ID, dungeon.NativeMap2ID}) {
		t.Fatalf("job level 6 listing %v", got)
	}
	if got := ids(f.svc.ListDungeons(7)); !slices.Equal(got, []string{dungeon.NativeMap0ID, dungeon.NativeMap1ID, dungeon.NativeMap2ID, dungeon.NativeMap3ID}) {
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
		exp.MapVariant, exp.Round, exp.RoundLimit = 5, 30, 60
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
				NewX: x, NewY: 7, RoundDelta: 1, Encounter: enc, Now: time.Now().UTC()})
			return err
		}
		if err := step(rev, 0); err != nil {
			t.Fatal(err)
		}
		if err := step(rev, 1); !errors.Is(err, dungeon.ErrExpeditionStateChanged) && !errors.Is(err, dungeon.ErrCombatActive) {
			t.Fatalf("stale step err=%v", err)
		}
		got, err = store.GetActiveExpedition(ctx, charID)
		if err != nil || got.PosX != 0 || got.Round != 31 || got.MapVariant != 5 || got.Encounter == nil ||
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
	hero := map3Hero("look", 5000, 900, 900)
	hero.JobID = "job-09"
	f := newNativeFixture(t, hero)
	f.placeNative(t, dungeon.NativeMap3ID, "look", 1, 7, 6, 3) // within scouting range of treasure 2, mist T and warp G
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
	if tiles := view.Map.Tiles; tiles[2][1] != "□" || tiles[3][1] != "□" || tiles[4][3] != "□" || tiles[2][2] != "●" {
		t.Fatalf("event cells must look like passage: %v", tiles)
	}
	if after := f.reload(t, "look"); after.Revision != before.Revision || f.rng.calls() != 0 {
		t.Fatal("observation mutated the run or drew RNG")
	}
}
