package battle

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestCallingSpecialMissingFallenAndAppearanceFallback(t *testing.T) {
	for _, job := range []int{61, 62, 63, 64, 66, 67, 71, 80, 87} {
		t.Run(fmt.Sprint(job), func(t *testing.T) {
			s, c, helper := callingSpecialFixture(t, job, 0)
			id := fmt.Sprintf("job-%d-special", job)
			// A living appearance substitute cannot bypass a fallen named helper.
			fallback := *s.actor(helper)
			fallback.ID, fallback.RosterIdentity = "fallback", ""
			prefix := map[int]int{62: 56, 67: 40, 71: 44, 80: 55}[job]
			if prefix == 0 {
				prefix = job
			}
			fallback.Icon = nativeFormIcon(fmt.Sprintf("job-%d", prefix), "m")
			s.Actors = append(s.Actors, fallback)
			c.PartyIDs = append(c.PartyIDs, fallback.ID)
			s.actor(helper).HP = 0
			before := s.callbackCopy()
			if r, err := s.ApplyRoster(&c, id, "enemy", &targetRNG{t: t}); err != nil || r.Applied || !reflect.DeepEqual(before, s) {
				t.Fatal(r, err)
			}
			s.actor(helper).RosterIdentity = "other"
			if got := s.FindCallingHelper(c.TargetContext, job); got != "fallback" {
				t.Fatal(got)
			}
			// Fallbacks keep their own stats while the summoner remains the actor.
			if job == 71 || job == 87 {
				rng := &targetRNG{t: t, draws: []float64{0, 0}}
				if _, err := s.ApplyRoster(&c, id, "enemy", rng); err != nil {
					t.Fatal(err)
				}
				rng.done()
				want := 437
				if job == 87 {
					want = 410
				}
				if s.actor("enemy").HP != want || c.ActorID != "player" {
					t.Fatal(s, c)
				}
			}
			// No appearance match is an independently accepted no-effect.
			s.actor("fallback").Icon = ""
			if r, err := s.ApplyRoster(&c, id, "enemy", &targetRNG{t: t}); err != nil || r.Applied {
				t.Fatal(r, err)
			}
		})
	}
}

func TestCallingSpecialDamageReturnAndOrderedStatus(t *testing.T) {
	for _, mode := range []string{"zero", "redirect", "one-hit"} {
		t.Run(mode, func(t *testing.T) {
			s, c, helper := callingSpecialFixture(t, 62, 0)
			draws := []float64{0}
			switch mode {
			case "zero":
				s.actor("player").ItemDefinitionIDs = []string{"item-166"}
			case "redirect":
				s.actor("enemy").Temporary = TemporaryReflectMagic
				draws = []float64{0, 0, 0}
			case "one-hit":
				s.actor("enemy").Temporary = TemporaryOneHitDefense
			}
			rng := &targetRNG{t: t, draws: draws}
			if _, err := s.ApplyRoster(&c, "job-62-special", "enemy", rng); err != nil {
				t.Fatal(err)
			}
			rng.done()
			if s.actor("enemy").HP != 500 {
				t.Fatal(s)
			}
			if mode == "redirect" {
				if s.actor("player").HP != 10 || s.actor(helper).HP != 131 {
					t.Fatal(s)
				}
			} else if c.HealCalls != 0 || s.actor(helper).HP != 50 {
				t.Fatal(s, c)
			}
		})
	}
	// Earlier damage crosses current-HP status immunity before the poison pass.
	s, c, _ := callingSpecialFixture(t, 61, 0)
	s.actor("enemy").HP, s.actor("enemy").MaxHP, s.actor("enemy").NPC = 1000, 1000, false
	rng := &targetRNG{t: t, draws: []float64{.5, 0, 0, 0}}
	if _, err := s.ApplyRoster(&c, "job-61-special", "enemy", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if s.actor("enemy").HP != 892 || s.actor("enemy").Status != StatusDeadlyPoison || !c.AdditionalEffect {
		t.Fatal(s, c)
	}
	// An earlier item195 hit makes a later status redirect to an unready helper
	// possible. Reject the complete intrinsic body before even the branch draw.
	s, c, helper := callingSpecialFixture(t, 61, 0)
	s.actor("enemy").ItemDefinitionIDs = []string{"item-195"}
	s.actor(helper).Gear = nil
	before := s.callbackCopy()
	if _, err := s.ApplyRoster(&c, "job-61-special", "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(err, s)
	}
}

func TestCallingDrainBarrageHitCountAndDeathStop(t *testing.T) {
	for i, draw := range []float64{0, 1.0 / 3, 2.0 / 3} {
		s, c, _ := callingSpecialFixture(t, 62, 0)
		draws := []float64{draw}
		for range i + 2 {
			draws = append(draws, 0, 0, 0, 0)
		}
		rng := &targetRNG{t: t, draws: draws}
		r, err := s.ApplyRoster(&c, "job-62-drain-barrage", "ignored", rng)
		if err != nil {
			t.Fatal(err)
		}
		rng.done()
		if s.actor("enemy").HP != 500-54*(i+2) || s.actor("player").HP != 100+24*(i+2) || c.DamageCalls != i+2 || c.HealCalls != i+2 || r.TargetConsumed {
			t.Fatal(s, c, r)
		}
	}
	// Reflection kills the caster on hit one. Its healing still runs; the next
	// iteration stops, without consuming the remaining hit/selection draws.
	s, c, _ := callingSpecialFixture(t, 62, 0)
	s.actor("player").HP = 1
	s.actor("enemy").Temporary = TemporaryReflectAttack
	rng := &targetRNG{t: t, draws: []float64{.999, 0, 0, .999, 0}}
	if _, err := s.ApplyRoster(&c, "job-62-drain-barrage", "", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if s.actor("player").HP != 0 || c.DamageCalls != 1 || c.HealCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestCallingGrowthAfterLethalHitAndStrictBoundary(t *testing.T) {
	for _, chance := range []float64{0, 1.0 / 3} {
		s, c, helper := callingSpecialFixture(t, 87, 0)
		s.actor("player").HP = 1
		s.actor("enemy").Temporary = TemporaryReflectAttack
		draws := []float64{0, 0, chance}
		if chance == 0 {
			draws = append(draws, 0, 0, 0, 0, 0)
		}
		rng := &targetRNG{t: t, draws: draws}
		if _, err := s.ApplyRoster(&c, "job-87-special", "enemy", rng); err != nil {
			t.Fatal(err)
		}
		rng.done()
		want := 0
		if chance == 0 {
			want = 1
		}
		if s.actor("player").HP != 0 || s.actor(helper).SummonForm != want {
			t.Fatal(s)
		}
	}
}

func TestCallingCurrentAppearanceAndFallenRecovery(t *testing.T) {
	// A transformed named helper keeps its identity, but loses its old breath
	// appearance. The new intrinsic body follows the default magic/status branch.
	s, c, helper := callingSpecialFixture(t, 67, 0)
	s.actor(helper).Icon = nativeFormIcon("job-40", "f")
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	if _, err := s.ApplyRoster(&c, "job-67-special", "enemy", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if s.actor("enemy").HP != 365 || s.actor("enemy").Status != StatusParalyze {
		t.Fatal(s)
	}
	// A copied male job55 appearance selects the archer branch regardless of
	// the named helper's saved summon ordinal.
	s, c, helper = callingSpecialFixture(t, 80, 3)
	s.actor(helper).Icon = nativeFormIcon("job-55", "m")
	rng = &targetRNG{t: t, draws: []float64{1.0 / 7, 0}}
	if _, err := s.ApplyRoster(&c, "job-80-special", "enemy", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if s.actor("enemy").HP != 437 {
		t.Fatal(s)
	}
	// Full party HP recovery then MP recovery precedes direct fallen revival;
	// source target fallback still visits the caster for the fallen party slot.
	s, c, _ = callingSpecialFixture(t, 80, 1)
	s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "fallen", TeamID: "party", MaxHP: 201, MP: 7, MaxMP: 100, Gear: &BattleGearStats{}, Status: StatusPoison}, Tension: 1})
	c.PartyIDs = append(c.PartyIDs, "fallen")
	rng = &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0, 0}}
	if _, err := s.ApplyRoster(&c, "job-80-special", "enemy", rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if s.actor("fallen").HP != 60 || s.actor("fallen").MP != 7 || s.actor("fallen").Status != StatusPoison || c.PartyHealCalls != 1 || c.PartyMPHealCalls != 1 {
		t.Fatal(s, c)
	}
}
