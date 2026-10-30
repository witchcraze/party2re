package skill

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRangedReturnedLivingTails(t *testing.T) {
	for _, id := range []string{"job-11-entry-5", "job-11-entry-6", "job-42-entry-3", "job-42-entry-4", "job-42-entry-6", "job-54-entry-3"} {
		for _, guard := range []string{"dead-after", "blocked", "absent", "reflected", "revived"} {
			t.Run(id+"/"+guard, func(t *testing.T) {
				s, c, d := rangedFixture()
				draws := []float64{0}
				switch guard {
				case "dead-after":
					s.Actors[1].HP = 1
				case "blocked":
					s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryOneHitDefense, nil
					draws = nil
				case "absent":
					s.Actors[1].HP, s.Actors[1].Gear = 0, nil
					draws = nil
				case "reflected":
					s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
					// The tail resolves again, then reflection returns the caster.
					draws = []float64{0, 0} // Damage, independent enemy selection.
					if id != "job-11-entry-5" {
						draws = append(draws, 0)
					}
				case "revived":
					s.Actors[1].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryRevival
					draws = []float64{0, 0} // Damage and revival.
					if id != "job-11-entry-5" {
						draws = append(draws, 0)
					}
				}
				executeRanged(t, &s, c, d, id, []string{"enemy"}, draws...)
				if guard == "dead-after" || guard == "blocked" || guard == "absent" {
					if c.StatDownCalls != 0 || c.Status.StatusCalls != 0 || c.Status.DeathCalls != 0 || !c.AdditionalEffect {
						t.Fatal(s, c)
					}
				} else {
					index := 1
					if guard == "reflected" {
						index = 0
						if s.Actors[1].HP != 900 {
							t.Fatal("tail used original target", s)
						}
					}
					if id == "job-11-entry-5" {
						if c.StatDownCalls != 1 || s.Actors[index].Hit >= 95 {
							t.Fatal(s, c)
						}
					} else if id == "job-42-entry-6" {
						if s.Actors[index].HP != 0 || c.Status.DeathCalls != 1 {
							t.Fatal(s, c)
						}
					} else if c.Status.StatusCalls != 1 || s.Actors[index].Status == "" {
						t.Fatal(s, c)
					}
				}
			})
		}
	}
}

func TestRangedBowTensionAndRequester(t *testing.T) {
	for _, row := range []struct {
		start, want float64
		draws       []float64
	}{{1, 1.7, nil}, {1.7, 3, nil}, {3, 5, nil}, {5, 8, []float64{0}}, {8, 8, nil}} {
		s, c, d := rangedFixture()
		level := 69
		c.RequesterLevel, s.Actors[0].Tension = &level, row.start
		executeRanged(t, &s, c, d, "job-11-entry-8", nil, row.draws...)
		if s.Actors[0].Tension != row.want || s.Actors[0].ExtendedTurns != 10 || c.TensionCalls != 1 {
			t.Fatal(row, s, c)
		}
	}
	for _, bound := range []bool{false, true} {
		s, c, d := rangedFixture()
		level := 24
		c.RequesterLevel, s.Actors[0].Tension = &level, 5
		if bound {
			s.Actors[0].Status = corebattle.StatusSabaku
		}
		executeRanged(t, &s, c, d, "job-11-entry-8", nil)
		if s.Actors[0].Tension != 5 || s.Actors[0].Extended != corebattle.ExtendedBow || c.TensionCalls != 1 {
			t.Fatal(bound, s, c)
		}
	}
}

func TestRangedEarlierHitExposesDeathDescendant(t *testing.T) {
	// Damage crosses player current-HP immunity and activates item195. The
	// subsequent death helper may now select the caster; require its reset facts.
	s, c, d := rangedFixture()
	s.Actors[1].HP, s.Actors[1].MaxHP = 1050, 1200
	s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
	s.Actors[2].HP, s.Actors[2].Gear = 200, nil
	c.GuaranteedHit = true
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-42-entry-0"}, noArithmeticDraws{t}); err != nil {
		t.Fatal("initial immunity must skip unreachable gear", err)
	}
	if _, err := d.InvocationAction(&s, &f, []string{"job-42-entry-6"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[1].HP != 1050 || c.Item195Activated || s.Actors[0].MP != 80 {
		t.Fatal(s, c)
	}
	s.Actors[2].Gear = &corebattle.BattleGearStats{}
	if _, err := d.InvocationAction(&s, &f, []string{"job-42-entry-6"}, noArithmeticDraws{t}); err != nil {
		t.Fatal("complete descendant should be admitted", err)
	}
}
