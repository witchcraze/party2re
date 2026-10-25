package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func controlFixture() (corebattle.LifetimeState, *corebattle.ControlClimaxContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].BaseStats.Attack, s.Actors[0].BaseStats.Defense, s.Actors[0].BaseStats.Agility = 100, 50, 40
	s.Actors[0].Defense = 50
	s.Actors[0].HP = 500
	s.Actors[1].HP = 900
	s.Actors[1].Attack, s.Actors[1].MaxMP, s.Actors[1].MP = 100, 100, 100
	c := &corebattle.ControlClimaxContext{DamageContext: base.DamageContext}
	d := BattleDefinitions{}
	d.RegisterControlClimax(c)
	return s, c, d
}

func executeControl(t *testing.T, s *corebattle.LifetimeState, c *corebattle.ControlClimaxContext, d BattleDefinitions, id string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(id, err)
	}
	if rng.used != len(draws) {
		t.Fatal(id, rng.used, len(draws))
	}
}

// Intrinsic results at ff97fc5:_skill.cgi:3723–3754,3943–4024,5293–5345.
func TestControlClimaxManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp int
		draws            []float64
	}{
		{53, 0, 5, 3, []float64{0}}, {53, 1, 15, 4, []float64{0}}, {53, 2, 30, 16, []float64{0, 0}},
		{53, 3, 55, 15, nil}, {53, 4, 80, 8, []float64{0}}, {53, 5, 120, 7, []float64{0, .5, 0, 0}},
		{58, 0, 20, 26, []float64{.5}}, {58, 1, 50, 9, nil}, {58, 2, 80, 16, []float64{0, 0}},
		{58, 3, 110, 6, nil}, {58, 4, 140, 33, nil}, {58, 5, 160, 66, []float64{.5}}, {58, 6, 160, 38, []float64{0}},
		{70, 0, 50, 25, []float64{0}}, {70, 1, 100, 25, nil}, {70, 2, 150, 40, []float64{0}}, {70, 4, 200, 66, []float64{0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := controlFixture()
			c.MPDrain = &corebattle.ControlMPDrain{
				Validate: func(_ *corebattle.LifetimeState, _ *corebattle.EffectContext, e corebattle.TargetEffect) error {
					if e.TargetID != "enemy" || e.Category != corebattle.CategoryMagic || e.Payload.Number != .4 {
						t.Fatal(e)
					}
					return nil
				},
				Apply: func(s *corebattle.LifetimeState, c *corebattle.EffectContext, e corebattle.TargetEffect, _ random.Generator) (corebattle.EffectApplication, error) {
					s.Actors[1].MP = 60
					c.StatDownCalls++
					return corebattle.EffectApplication{}, nil
				},
			}
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 58 && row.pos == 6 || row.job == 70 && row.pos == 4) {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if row.job == 53 && row.pos == 3 {
				args[0] = "self"
			}
			executeControl(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if a.MP != 80 || a.CMP != 90 || c.ActorID != "self" {
				t.Fatal(a, c)
			}
			switch id {
			case "job-53-entry-0":
				if e.Status != corebattle.StatusDofuu {
					t.Fatal(e)
				}
			case "job-53-entry-1", "job-53-entry-2":
				if e.Status != corebattle.StatusDeadlyPoison {
					t.Fatal(e)
				}
			case "job-53-entry-3":
				if a.Temporary != corebattle.TemporaryReflectAttack {
					t.Fatal(a)
				}
			case "job-53-entry-4":
				if e.Status != corebattle.StatusPhysicalSeal {
					t.Fatal(e)
				}
			case "job-53-entry-5":
				if e.HP != 855 || c.DamageCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-58-entry-0":
				if e.HP != 270 {
					t.Fatal(e)
				}
			case "job-58-entry-1":
				if a.Attack != 200 || a.Agility != 80 || a.Status != corebattle.StatusConfusion || a.Temporary != corebattle.TemporaryAbsorbMagic {
					t.Fatal(a)
				}
			case "job-58-entry-2":
				if e.Status != corebattle.StatusMagicSeal {
					t.Fatal(e)
				}
			case "job-58-entry-3":
				if e.MP != 60 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-58-entry-4":
				if a.Temporary != corebattle.TemporaryDouble || a.Tension != 3 {
					t.Fatal(a)
				}
			case "job-58-entry-5":
				if e.HP != 225 {
					t.Fatal(e)
				}
			case "job-58-entry-6":
				if e.HP != 837 || c.DamageCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-70-entry-0":
				if a.HP != 770 {
					t.Fatal(a)
				}
			case "job-70-entry-1":
				if a.Attack != 140 || a.Defense != 70 {
					t.Fatal(a)
				}
			case "job-70-entry-2":
				if e.HP != 738 || c.AreaDamageCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-70-entry-4":
				if e.HP != 786 || c.DamageCalls != 2 || c.AreaDamageCalls != 1 {
					t.Fatal(e, c)
				}
			}
		})
	}
}
