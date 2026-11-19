package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
)

func map4Hero(id string, maxHP int) corecharacter.Character {
	c := createTestChar(id, 30, maxHP, 900, 900)
	c.JobLevel, c.JobID = 9, "job-01"
	return c
}

func TestNativeMap4_CatalogTranscribesLayouts(t *testing.T) {
	d := mapDef(t, dungeon.NativeMap4ID)
	if d.RoundLimit != 70 || d.LeaderJobLevel != 9 || len(d.Variants) != 7 {
		t.Fatalf("map4 catalog %+v", d)
	}
	for i, f := range d.Variants {
		// The start's only exit is the message cell.
		if f.Width != 11 || f.Height != 10 || f.StartX != 0 || f.StartY != 0 || f.Grid[0][1] != 'M' || f.Grid[1][0] != '1' {
			t.Fatalf("variant %d layout %+v", i+1, f)
		}
	}
}

// event_0 replaces the base spawn: the mover loses int(MaxHP × 0.02 + 0.5)
// without an encounter or an RNG draw (map/4/1.cgi:24).
func TestNativeMap4_AttritionBoundaries(t *testing.T) {
	for _, tc := range []struct{ maxHP, loss int }{{24, 0}, {25, 1}, {74, 1}, {75, 2}} {
		t.Run(fmt.Sprint(tc.maxHP), func(t *testing.T) {
			f := newNativeFixture(t, map4Hero("walker", tc.maxHP))
			f.placeNative(t, dungeon.NativeMap4ID, "walker", 1, 2, 0, 2)
			res, err := f.svc.Move(context.Background(), "walker", dungeon.DirectionEast)
			if err != nil || res.EventType != dungeon.EventMove || res.Expedition.Encounter != nil || f.rng.calls() != 0 {
				t.Fatalf("passage %+v err=%v rng=%d", res, err, f.rng.calls())
			}
			exp := f.reload(t, "walker")
			if got := memberOf(t, exp, "walker").CurrentHP; got != tc.maxHP-tc.loss || exp.CurrentHP != got || res.MemberDamages["walker"] != tc.loss {
				t.Fatalf("HP %d run %d damages %v, want loss %d", got, exp.CurrentHP, res.MemberDamages, tc.loss)
			}
		})
	}
}

// Non-passage cells cost nothing; M is a once-only message.
func TestNativeMap4_MessageCostsNothing(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map4Hero("reader", 500))
	f.placeNative(t, dungeon.NativeMap4ID, "reader", 1, 2, 0, 2)
	for range 2 {
		if _, err := f.svc.Move(ctx, "reader", dungeon.DirectionWest); err != nil {
			t.Fatal(err)
		}
		f.moveTo(t, "reader", 2, 0)
	}
	exp := f.reload(t, "reader")
	if memberOf(t, exp, "reader").CurrentHP != 500 || !slices.Equal(exp.ConsumedEvents, []string{"M"}) || f.rng.calls() != 0 {
		t.Fatalf("message %+v", exp)
	}
}

// Only the mover pays; a fallen mover stops while the living ally continues,
// and an all-fallen party keeps the run with only Escape.
func TestNativeMap4_ActorOnlyDeathAndContinuation(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map4Hero("lead", 100))
	f.chars.chars["ally"] = map4Hero("ally", 100)
	f.startNativeParty(t, dungeon.NativeMap4ID, 1, 2, 0)
	f.setVitality(t, "lead", "lead", 1, 0)
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "lead")
	if memberOf(t, exp, "lead").CurrentHP != 0 || memberOf(t, exp, "ally").CurrentHP != 100 || exp.Status != dungeon.StatusExploring {
		t.Fatalf("after fall %+v", exp.Members)
	}
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionEast); !errors.Is(err, dungeon.ErrActorFallen) {
		t.Fatalf("fallen mover err=%v", err)
	}
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if exp = f.reload(t, "lead"); memberOf(t, exp, "ally").CurrentHP != 98 || memberOf(t, exp, "lead").CurrentHP != 0 {
		t.Fatalf("ally move %+v", exp.Members)
	}
	f.setVitality(t, "lead", "ally", 1, 0)
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp = f.reload(t, "lead"); exp.Status != dungeon.StatusExploring {
		t.Fatalf("all fallen ended the run %+v", exp)
	} else if c := exp.Controls("lead"); c.CanMove || c.CanFight || !c.CanEscape {
		t.Fatalf("all-fallen controls %+v", c)
	}
}

// H: rand(2) < 1 heals HP base 40, else MP base 40 +1, on a uniform roster
// member; one flag covers every H cell (variant 4 has two).
func TestNativeMap4_RecoveryBranchesAndOnce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name           string
		branch         float64
		wantHP, wantMP int
	}{{"hp", 0.4, 136, 10}, {"mp", 0.5, 100, 47}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map4Hero("healer", 500))
			f.placeNative(t, dungeon.NativeMap4ID, "healer", 4, 4, 4, 2)
			f.setVitality(t, "healer", "healer", 100, 10)
			exp := f.reload(t, "healer")
			exp.Members[0].MaxMP = 200
			if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
				t.Fatal(err)
			}
			// Branch, roster pick, then the amount roll at its 0.9 floor.
			f.rng.floats = []float64{tc.branch, 0, 0}
			if _, err := f.svc.Move(ctx, "healer", dungeon.DirectionEast); err != nil {
				t.Fatal(err)
			}
			m := memberOf(t, f.reload(t, "healer"), "healer")
			if m.CurrentHP != tc.wantHP || m.CurrentMP != tc.wantMP {
				t.Fatalf("HP %d MP %d, want %d/%d", m.CurrentHP, m.CurrentMP, tc.wantHP, tc.wantMP)
			}
			// The second H cell (7,7) does nothing and draws nothing.
			f.moveTo(t, "healer", 7, 6)
			if _, err := f.svc.Move(ctx, "healer", dungeon.DirectionSouth); err != nil {
				t.Fatal(err)
			}
			if again := memberOf(t, f.reload(t, "healer"), "healer"); again.CurrentHP != m.CurrentHP || again.CurrentMP != m.CurrentMP || f.rng.calls() != 0 {
				t.Fatalf("second H applied: %+v rng=%d", again, f.rng.calls())
			}
		})
	}
}

// A fallen pick heals the actor instead (_skill.cgi:9635–9638).
func TestNativeMap4_RecoveryRedirectsFallenPickToActor(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map4Hero("lead", 500))
	f.chars.chars["ally"] = map4Hero("ally", 500)
	f.startNativeParty(t, dungeon.NativeMap4ID, 4, 4, 4)
	f.setVitality(t, "lead", "lead", 100, 0)
	f.setVitality(t, "lead", "ally", 0, 0)
	f.rng.floats = []float64{0, 0.75, 0} // HP branch, pick the fallen ally
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "lead")
	if memberOf(t, exp, "lead").CurrentHP != 136 || memberOf(t, exp, "ally").CurrentHP != 0 || exp.CurrentHP != 136 {
		t.Fatalf("redirect %+v", exp.Members)
	}
}

// B (base event) and b (map event) are independent once-only bosses.
func TestNativeMap4_BossesAreIndependentAndOnce(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map4Hero("hunter", 50000))
	f.placeNative(t, dungeon.NativeMap4ID, "hunter", 1, 7, 0, 2)
	for _, dir := range []dungeon.Direction{dungeon.DirectionEast, dungeon.DirectionEast} {
		res, err := f.svc.Move(ctx, "hunter", dir)
		if err != nil || res.EventType != dungeon.EventBoss {
			t.Fatalf("boss %+v err=%v", res, err)
		}
	}
	f.moveTo(t, "hunter", 8, 0)
	if res, err := f.svc.Move(ctx, "hunter", dungeon.DirectionEast); err != nil || res.EventType == dungeon.EventBoss {
		t.Fatalf("B revisit %+v err=%v", res, err)
	}
}
