package skill

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func statusLoadRegistry(enemies, absorbers int) (corebattle.LifetimeState, corebattle.CallbackContext, BattleDefinitions, *corebattle.StatusContext) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 100, MaxHP: 100, CMP: 20, Gear: &corebattle.BattleGearStats{}}, Tension: 1}}}
	f := corebattle.CallbackContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, Custom: true}, Arguments: []string{"self", "tail"}}
	for i := range enemies {
		id := fmt.Sprintf("enemy-%d", i)
		a := corebattle.LifetimeActor{Participant: corebattle.Participant{ID: id, TeamID: "enemy", HP: 100, MaxHP: 100, MaxMP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1}
		if i < absorbers {
			a.Temporary = corebattle.TemporaryAbsorbMagic
		}
		s.Actors = append(s.Actors, a)
		f.EnemyIDs = append(f.EnemyIDs, id)
	}
	c := &corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: f.TargetContext}}
	var d BattleDefinitions
	d.RegisterStatuses(c, nil)
	d.RegisterDefenseGems()
	return s, f, d, c
}

func TestStatusProjectionRegisteredSequentialLoad(t *testing.T) {
	s, f, d, c := statusLoadRegistry(5, 0)
	ops, err := d.InvocationAction(&s, &f, []string{"gem_inflict_poison", "gem_inflict_sleep", "gem_guard_1"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: []float64{0, 0, 0, 0, 0, .99, .99, .99, .99, .99}}
	if err = s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != 10 || s.Actors[0].CMP != 20 {
		t.Fatal(err, rng.used, c)
	}
	for _, a := range s.Actors[1:] {
		if a.Status != corebattle.StatusDeadlyPoison {
			t.Fatal(a)
		}
	}
}

func TestStatusProjectionRegisteredCumulativeBudgetRejectsAtomically(t *testing.T) {
	for _, intrinsicTail := range []bool{false, true} {
		s, f, d, c := statusLoadRegistry(2, 2)
		if intrinsicTail {
			s.Actors[0].NPC = true
			c.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: f.TargetContext}}
		}
		before := s
		before.Actors = append([]corebattle.LifetimeActor(nil), s.Actors...)
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_inflict_confusion"}, {Kind: "entry", EntryID: "gem_inflict_confusion"}, {Kind: "entry", EntryID: "gem_guard_1"}}
		ids := []string{"gem_inflict_confusion", "gem_inflict_confusion", "gem_guard_1"}
		if intrinsicTail {
			ids = []string{"gem_inflict_confusion", "gem_instant_death_sky"}
			ops = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: ids[0]}, {Kind: "entry", EntryID: ids[1]}}
		}
		if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, s) || f.Damage != nil || f.Status != nil || !reflect.DeepEqual(f.Arguments, []string{"self", "tail"}) {
			t.Fatal("budget rejection changed caller")
		}
	}
}

func BenchmarkStatusProjectionRegisteredSequentialLoad(b *testing.B) {
	for _, absorbers := range []int{0, 2} {
		name := "normal5"
		enemies := 5
		if absorbers > 0 {
			name = "absorb2-reject"
			enemies = 2
		}
		b.Run(name, func(b *testing.B) {
			s, f, d, _ := statusLoadRegistry(enemies, absorbers)
			ids := []string{"gem_inflict_confusion", "gem_inflict_confusion", "gem_guard_1"}
			if absorbers == 0 {
				ids = []string{"gem_inflict_poison", "gem_inflict_sleep", "gem_guard_1"}
			}
			b.ReportAllocs()
			rng := random.NewDeterministic(1)
			for b.Loop() {
				_, err := d.InvocationAction(&s, &f, ids, rng)
				if absorbers > 0 {
					if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
						b.Fatal(err)
					}
				} else if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
