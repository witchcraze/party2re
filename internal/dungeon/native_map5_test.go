package dungeon_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Variant 1 (map/5/1.cgi):
//
//	S0000IT140t
//	0111t111110
//	01000000010
//	01011101010
//	0I01L101000
//	01010001101
//	0T01I111100
//	0101I12100I
//	0001IIB1H13
func map5Hero(id, job string, hp int) corecharacter.Character {
	c := createTestChar(id, 30, hp, 900, 900)
	c.JobLevel, c.JobID, c.Stats.MaxMP = 11, job, 80
	return c
}

func (f *nativeFixture) placeMap5(t *testing.T, id string, x, y int) {
	t.Helper()
	f.placeNative(t, dungeon.NativeMap5ID, 60, id, 1, x, y, 2)
}

func (f *nativeFixture) startMap5Party(t *testing.T, x, y int) {
	t.Helper()
	f.rng.ints = []int{0}
	if _, err := f.svc.StartPartyExpedition(context.Background(), "lead", []string{"lead", "ally"}, dungeon.NativeMap5ID, "p"); err != nil {
		t.Fatal(err)
	}
	f.moveTo(t, "lead", x, y)
	f.rng.ints, f.rng.intCalls, f.rng.floatCalls = nil, 0, 0
}

func TestNativeMap5_CatalogTranscribesLayouts(t *testing.T) {
	var d dungeon.Dungeon
	for _, c := range dungeon.DefaultDungeonCatalog() {
		if c.ID == dungeon.NativeMap5ID {
			d = c
		}
	}
	if d.RoundLimit != 60 || d.LeaderJobLevel != 11 || len(d.Variants) != 4 {
		t.Fatalf("map5 catalog %+v", d)
	}
	starts := [][2]int{{0, 0}, {0, 0}, {2, 2}, {2, 2}}
	for i, f := range d.Variants {
		all := strings.Join(f.Grid, "")
		if f.Width != 11 || f.Height != 9 || f.StartX != starts[i][0] || f.StartY != starts[i][1] {
			t.Fatalf("variant %d geometry %+v", i+1, f)
		}
		for _, sym := range []string{"L", "B", "H"} {
			if strings.Count(all, sym) != 1 {
				t.Fatalf("variant %d has %d %s cells", i+1, strings.Count(all, sym), sym)
			}
		}
	}
}

// L is a once-only boss; its flag turns every I from wall to passage in view,
// while ★ stays (map/5/1.cgi:18–20,38–47). I is traversable either way.
func TestNativeMap5_LeverBossOnceAndReveal(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		leverAndReveal(t, dungeon.NewMemoryExpeditionRepository(), "lever")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		id := "lever-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, id)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		leverAndReveal(t, store, id)
	})
}

func leverAndReveal(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	f := newNativeFixtureOn(t, store, map5Hero(id, "job-01", 50000))
	f.placeMap5(t, id, 4, 5) // south of L, north of I (4,6)
	view := func() [][]string {
		v, err := f.svc.ObserveExpedition(ctx, id)
		if err != nil || v.Map == nil {
			t.Fatalf("observe %+v err=%v", v, err)
		}
		return v.Map.Tiles
	}
	if tiles := view(); tiles[0][1] != "★" || tiles[2][1] != "■" {
		t.Fatalf("before lever %v", tiles)
	}
	// The hidden passage is walkable before the lever and changes no state.
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionSouth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
		t.Fatalf("I before lever %+v err=%v rng=%d", res, err, f.rng.calls())
	}
	if _, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventBoss || res.CombatOutcome != corebattle.OutcomeWin {
		t.Fatalf("lever %+v err=%v", res, err)
	}
	if exp := f.reload(t, id); !slices.Equal(exp.ConsumedEvents, []string{"L"}) {
		t.Fatalf("lever flag %v", exp.ConsumedEvents)
	}
	if _, err := f.svc.Move(ctx, id, dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if tiles := view(); tiles[0][1] != "★" || tiles[2][1] != "□" {
		t.Fatalf("after lever %v", tiles)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	if res, err := f.svc.Move(ctx, id, dungeon.DirectionNorth); err != nil || res.EventType != dungeon.EventMove || f.rng.calls() != 0 {
		t.Fatalf("lever replay %+v err=%v rng=%d", res, err, f.rng.calls())
	}
}

// T poisons the party for the ensuing encounter on every entry (map/5/1.cgi:24).
func TestNativeMap5_PoisonGasPrecedesEncounter(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map5Hero("gas", "job-01", 100_000))
	f.placeMap5(t, "gas", 0, 6) // west of T at (1,6)
	for range 2 {
		res, err := f.svc.Move(ctx, "gas", dungeon.DirectionEast)
		if err != nil || res.EventType != dungeon.EventBattle || res.BattleResult == nil {
			t.Fatalf("gas %+v err=%v", res, err)
		}
		poisoned := slices.ContainsFunc(res.BattleResult.Logs, func(l corebattle.TurnLog) bool {
			return l.ActorID == "gas" && l.Kind == "status_damage"
		})
		if !poisoned {
			t.Fatal("severe poison did not reach the ensuing battle")
		}
		if _, err := f.svc.Move(ctx, "gas", dungeon.DirectionWest); err != nil {
			t.Fatal(err)
		}
	}
	if exp := f.reload(t, "gas"); len(exp.ConsumedEvents) != 0 {
		t.Fatalf("gas must not be once-only: %v", exp.ConsumedEvents)
	}
}

// t hits every member with int(80 × (0.9 + rand(0.3))) on every entry; the
// fallen stay at 0 (map/5/1.cgi:25, vs_dungeon.cgi _trap_d).
func TestNativeMap5_ArrowTrapHitsEveryMember(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map5Hero("lead", "job-01", 500))
	f.chars.chars["ally"] = map5Hero("ally", "job-01", 500)
	f.startMap5Party(t, 4, 0) // north of t at (4,1)
	for _, tc := range []struct {
		floats             []float64
		wantLead, wantAlly int
	}{{[]float64{0, 0.99}, 428, 405}, {[]float64{0.99, 0}, 333, 333}} {
		f.rng.floats = tc.floats
		res, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth)
		if err != nil {
			t.Fatal(err)
		}
		exp := f.reload(t, "lead")
		if memberOf(t, exp, "lead").CurrentHP != tc.wantLead || memberOf(t, exp, "ally").CurrentHP != tc.wantAlly || exp.CurrentHP != tc.wantLead {
			t.Fatalf("trap %+v damages %v", exp.Members, res.MemberDamages)
		}
		if len(exp.ConsumedEvents) != 0 || res.MemberDamages["lead"] <= 0 || res.MemberDamages["ally"] <= 0 {
			t.Fatalf("trap state %v damages %v", exp.ConsumedEvents, res.MemberDamages)
		}
		f.rng.floats = nil
		if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionNorth); err != nil {
			t.Fatal(err)
		}
	}
	f.setVitality(t, "lead", "ally", 0, 0)
	f.rng.floats = []float64{0, 0}
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "lead"); memberOf(t, exp, "ally").CurrentHP != 0 || memberOf(t, exp, "lead").CurrentHP != 333-72 {
		t.Fatalf("fallen trap %+v", exp.Members)
	}
}

// H needs the acting character's job 43; another member's job does nothing.
// Success restores every member, reviving the fallen, once (map/5/1.cgi:26–37).
func TestNativeMap5_RemedyNeedsActingJob43AndRevives(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map5Hero("lead", "job-01", 500))
	f.chars.chars["ally"] = map5Hero("ally", "job-43", 300)
	f.startMap5Party(t, 8, 7) // north of H at (8,8)
	f.setVitality(t, "lead", "lead", 10, 0)
	f.setVitality(t, "lead", "ally", 20, 5)

	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "lead")
	if memberOf(t, exp, "lead").CurrentHP != 10 || memberOf(t, exp, "ally").CurrentHP != 20 || len(exp.ConsumedEvents) != 0 {
		t.Fatalf("non-qualifying actor %+v %v", exp.Members, exp.ConsumedEvents)
	}

	f.setVitality(t, "lead", "lead", 0, 0)
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	res, err := f.svc.Move(ctx, "ally", dungeon.DirectionSouth)
	if err != nil || len(res.MemberDamages) != 0 {
		t.Fatalf("remedy %+v err=%v", res, err)
	}
	exp = f.reload(t, "lead")
	lead, ally := memberOf(t, exp, "lead"), memberOf(t, exp, "ally")
	if lead.CurrentHP != 500 || lead.CurrentMP != 80 || ally.CurrentHP != 300 || ally.CurrentMP != 80 || exp.CurrentHP != 500 ||
		!slices.Equal(exp.ConsumedEvents, []string{"H"}) {
		t.Fatalf("remedy %+v run HP %d flags %v", exp.Members, exp.CurrentHP, exp.ConsumedEvents)
	}

	f.setVitality(t, "lead", "ally", 1, 0)
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionNorth); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if ally := memberOf(t, f.reload(t, "lead"), "ally"); ally.CurrentHP != 1 {
		t.Fatalf("remedy repeated %+v", ally)
	}
}
