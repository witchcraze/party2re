package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func duelFixture() (corebattle.LifetimeState, *corebattle.DuelCrownContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].BaseStats.Attack = 100
	s.Actors[1].Attack, s.Actors[1].BaseStats.Attack = 100, 100
	c := &corebattle.DuelCrownContext{DamageContext: base.DamageContext}
	d := BattleDefinitions{}
	d.RegisterDuelCrown(c)
	return s, c, d
}

// Intrinsic vectors: ff97fc5:_skill.cgi:5594–5720, not shared-helper certification.
func TestDuelCrownManifest(t *testing.T) {
	for pos, row := range []struct{ sp, mp int }{{10, 0}, {30, 2}, {50, 10}, {70, 40}, {120, 14}, {150, 45}} {
		id := fmt.Sprintf("job-74-entry-%d", pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := duelFixture()
			entry := d.Invocations.JobTables[74].Variants[0][pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			ops, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			draws := [][]float64{{.5}, nil, {.5, 0, .5, 0, .5, 0, .5, 0, .5}, {0}, {0}, {0, 0, 0, .5, .5, 0, 0, 0, .5, .5, 0, 0, 0, .5, .5}}[pos]
			rng := &damageGemDraws{t: t, draws: draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
				t.Fatal(err)
			}
			if rng.used != len(draws) || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
				t.Fatal(rng.used, s)
			}
			switch pos {
			case 0:
				if s.Actors[1].Tension != 1.7 || c.TensionCalls != 1 {
					t.Fatal(s, c)
				}
			case 1:
				if s.Actors[0].Temporary != corebattle.TemporaryReduceAttack {
					t.Fatal(s)
				}
			case 2:
				if s.Actors[0].HP != 1000 || s.Actors[1].HP != 902 || s.Actors[0].Attack != 100 || c.DamageCalls != 4 {
					t.Fatal(s, c)
				}
			case 3:
				if s.Actors[1].HP != 865 || c.AreaDamageCalls != 1 {
					t.Fatal(s, c)
				}
			case 4:
				if s.Actors[0].Temporary != corebattle.TemporaryRevival {
					t.Fatal(s)
				}
			case 5:
				if s.Actors[1].HP != 880 || c.DamageCalls != 3 {
					t.Fatal(s, c)
				}
			}
		})
	}
}
