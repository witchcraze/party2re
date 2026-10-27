package skill

import (
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestMartialEveryPositionSealBoundary(t *testing.T) {
	for _, job := range []int{4, 23, 25} {
		for _, entry := range corebattle.MartialEntries(job) {
			t.Run(entry.ID, func(t *testing.T) {
				s, c, d := martialFixture()
				s.Actors[0].Status, s.Actors[0].Hit = corebattle.StatusPhysicalSeal, 94
				draws := []float64{.9}
				switch entry.ID {
				case "job-4-entry-1", "job-25-entry-3", "job-25-entry-4", "job-25-entry-6", "job-25-entry-7":
					draws = nil
				case "job-4-entry-8", "job-23-entry-1", "job-23-entry-3":
					draws = []float64{.9, .9}
				case "job-4-entry-7":
					draws = []float64{0, .9, .9, .9}
				}
				executeMartial(t, &s, c, d, entry.ID, []string{"enemy", "enemy"}, draws...)
				a := s.Actors[0]
				if a.HP != 500 || a.Attack != 100 || a.Defense != 50 || a.Agility != 40 || a.Status != corebattle.StatusPhysicalSeal || s.Actors[1].HP != 1000 || c.DamageCalls != 0 || c.HealCalls != 0 || c.StatUpCalls != 0 {
					t.Fatal(entry, s, c)
				}
				// These assignments follow helper failure or have no seal guard.
				if (entry.ID == "job-4-entry-1" || entry.ID == "job-25-entry-1") && a.Hit != 95 {
					t.Fatal(a)
				}
				if entry.ID == "job-25-entry-2" && a.Temporary != corebattle.TemporaryDouble {
					t.Fatal(a)
				}
				if entry.ID == "job-4-entry-8" && (a.Extended != corebattle.ExtendedFlame || a.ExtendedTurns != 6) {
					t.Fatal(a)
				}
			})
		}
	}
}

func TestMartialFreshArgumentsAndFlurryRange(t *testing.T) {
	for _, args := range [][]string{{"enemy"}, {"enemy", "other", "tail"}} {
		s, c, d := martialFixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		draws := []float64{0, 0}
		if len(args) == 1 {
			draws = []float64{0, .999, 0}
		}
		frame := executeMartial(t, &s, c, d, "job-23-entry-3", args, draws...)
		if s.Actors[1].HP != 961 || s.Actors[3].HP != 961 || c.DamageCalls != 2 || len(c.Arguments) != max(0, len(args)-2) {
			t.Fatal(s, c, frame)
		}
	}
	for _, draw := range []float64{0, .999999} {
		s, c, d := martialFixture()
		count := 3 + int(draw*3)
		draws := append([]float64{draw}, make([]float64, count*2)...)
		frame := executeMartial(t, &s, c, d, "job-4-entry-7", []string{"untouched"}, draws...)
		if c.DamageCalls != count || s.Actors[1].HP != 1000-36*count || !reflect.DeepEqual(frame.Arguments, []string{"untouched"}) {
			t.Fatal(s, c, frame)
		}
	}
}

func TestMartialRepeatStopsOnReflectedDeathAndRereadsStats(t *testing.T) {
	for _, id := range []string{"job-4-entry-7", "job-23-entry-3", "job-25-entry-2"} {
		s, c, d := martialFixture()
		s.Actors[0].HP = 1
		s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
		c.Defeat = func(string) error { s.Actors[0].HP = 0; return nil }
		draws := []float64{0}
		if id == "job-4-entry-7" {
			draws = []float64{.999, 0, 0}
		}
		frame := executeMartial(t, &s, c, d, id, []string{"enemy", "tail"}, draws...)
		if s.Actors[0].HP != 0 || c.DamageCalls != 1 || s.Actors[1].HP != 1000 {
			t.Fatal(id, s, c)
		}
		if id == "job-23-entry-3" && !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
			t.Fatal(frame)
		}
		if id == "job-25-entry-2" && s.Actors[0].Temporary != corebattle.TemporaryDouble {
			t.Fatal(s.Actors[0])
		}
	}
	s, c, d := martialFixture()
	s.Actors[0].Extended = corebattle.ExtendedPurification
	c.Reset = func(string) error { s.Actors[0].Agility = 20; return nil }
	executeMartial(t, &s, c, d, "job-23-entry-3", []string{"enemy", "enemy"}, 0, 0)
	if s.Actors[1].HP != 942 || c.DamageCalls != 2 { // floor88×.45 + floor44×.45
		t.Fatal(s, c)
	}
}

func TestMartialDrainReturnAndHealOrder(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		s, c, d := martialFixture()
		s.Actors[0].Tension = 3
		draws := []float64{0, 0}
		wantHP := 608 // damage floor(90×.45×3)=121; heal floor121×.9=108
		if blocked {
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
			draws, wantHP = nil, 500
		}
		executeMartial(t, &s, c, d, "job-23-entry-2", []string{"enemy"}, draws...)
		if s.Actors[0].HP != wantHP || c.HealCalls != len(draws)/2 {
			t.Fatal(blocked, s, c)
		}
	}
	// A reflected hit can kill the caster before healing; the heal then has no effect.
	s, c, d := martialFixture()
	s.Actors[0].HP = 1
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	s.Actors[2].HP, s.Actors[2].MaxHP = 100, 1000
	c.Defeat = func(string) error { s.Actors[0].HP = 0; return nil }
	executeMartial(t, &s, c, d, "job-23-entry-2", []string{"enemy"}, 0)
	if s.Actors[0].HP != 0 || s.Actors[2].HP != 100 || c.HealCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestMartialHitAndStatCapBoundaries(t *testing.T) {
	for _, hit := range []int{94, 95, 100} {
		for _, id := range []string{"job-4-entry-1", "job-25-entry-1"} {
			s, c, d := martialFixture()
			s.Actors[0].Hit = hit
			draws := []float64{}
			if id == "job-25-entry-1" {
				s.Actors[0].HP = 1000 // Full HP still invokes healing, then resets hit.
				draws = []float64{0}
			}
			executeMartial(t, &s, c, d, id, nil, draws...)
			if s.Actors[0].Hit != 95 {
				t.Fatal(s.Actors[0])
			}
		}
	}
	for _, id := range []string{"job-4-entry-2", "job-4-entry-8", "job-23-entry-1"} {
		s, c, d := martialFixture()
		s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 249, 125, 100
		executeMartial(t, &s, c, d, id, nil)
		wantAttack := 250
		if id == "job-4-entry-2" {
			wantAttack = 249
		}
		if s.Actors[0].Attack != wantAttack || s.Actors[0].Defense != 125 || s.Actors[0].Agility != 100 {
			t.Fatal(id, s)
		}
	}
}

func TestMartialStatusAndDeathBoundaries(t *testing.T) {
	for _, draw := range []float64{.74999, .75} {
		s, c, d := martialFixture()
		s.Actors[1].HP = 999
		executeMartial(t, &s, c, d, "job-4-entry-4", []string{"enemy"}, draw)
		if (s.Actors[1].Status == corebattle.StatusDofuu) != (draw < .75) || c.Status.StatusCalls != 1 {
			t.Fatal(s, c.Status)
		}
		if draw < .75 {
			decision, err := s.Actors[1].CheckIncapacitation(corebattle.IncapacitationContext{}, noArithmeticDraws{t})
			if err != nil || decision.NormalAction || s.Actors[1].Status != "" {
				t.Fatal(decision, err, s.Actors[1])
			}
		}
	}
	for _, draw := range []float64{.19, .19001} {
		s, c, d := martialFixture()
		s.Actors[1].HP = 999
		c.Status.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		}}
		executeMartial(t, &s, c, d, "job-4-entry-5", []string{"enemy"}, draw)
		if (s.Actors[1].HP == 0) != (draw == .19) || c.Status.DeathCalls != 1 {
			t.Fatal(s, c.Status)
		}
	}
}

func TestMartialAgilityEquipmentTailUsesSuppliedOwner(t *testing.T) {
	s, c, d := martialFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-201", "item-205"}
	level, calls := 99, 0
	c.RequesterLevel = &level
	c.Damage = func(target corebattle.TargetContext, effect corebattle.TargetEffect, rng random.Generator) error {
		calls++
		if target.ActorID != "self" || effect.Payload.Number != 28 || effect.Category != corebattle.CategoryUntyped || !reflect.DeepEqual(effect.Arguments, []string{"1"}) || s.Actors[0].Attack != 100 {
			t.Fatal(target, effect, s)
		}
		return nil
	}
	executeMartial(t, &s, c, d, "job-4-entry-2", nil)
	if calls != 1 || s.Actors[0].Agility != 56 || s.Actors[0].Attack != 116 || s.Actors[0].Temporary != corebattle.TemporaryDouble || c.TensionCalls != 1 {
		t.Fatal(s, c, calls)
	}
}
