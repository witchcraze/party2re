package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func ritualDanceFixture() (corebattle.LifetimeState, *corebattle.RitualDanceContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP, s.Actors[0].MaxMP = 500, 1000
	s.Actors[1].HP, s.Actors[1].Hit = 900, 95
	c := &corebattle.RitualDanceContext{DamageContext: base.DamageContext}
	d := BattleDefinitions{}
	d.RegisterRitualDance(c, len(c.PartyIDs))
	return s, c, d
}

func executeRitualDance(t *testing.T, s *corebattle.LifetimeState, c *corebattle.RitualDanceContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Intrinsic vectors: ff97fc5:_skill.cgi:5730–5787,5802–5917.
func TestRitualDanceManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp int
		target           string
		draws            []float64
	}{
		{75, 0, 20, 16, "enemy", []float64{0, 0}},
		{75, 1, 40, 13, "enemy", nil},
		{75, 2, 60, 5, "self", nil},
		{75, 3, 80, 9, "self", []float64{.4}},
		{75, 4, 110, 1, "self", []float64{0, 0}},
		{75, 6, 140, 44, "enemy", []float64{0, 0, 0}},
		{76, 0, 10, 7, "enemy", nil},
		{76, 1, 20, 9, "enemy", []float64{0}},
		{76, 2, 50, 24, "enemy", []float64{0, 0, 0}},
		{76, 4, 110, 37, "self", nil},
		{76, 5, 130, 24, "", []float64{0, 0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := ritualDanceFixture()
			s.Actors[0].Hit = 40
			s.Actors[1].Temporary = corebattle.TemporaryDefense
			if row.pos == 3 || row.job == 76 && row.pos == 4 {
				s.Actors[0].Status = corebattle.StatusPoison
			}
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			executeRitualDance(t, &s, c, d, id, []string{row.target, "tail"}, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if a.CMP != 90 || c.ActorID != "self" || a.MP != 80 && id != "job-75-entry-4" {
				t.Fatal(a, c)
			}
			switch id {
			case "job-75-entry-0":
				if e.Status != corebattle.StatusMagicSeal || c.StatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-75-entry-1":
				if e.HP != 720 || e.Temporary != "" {
					t.Fatal(e)
				}
			case "job-75-entry-2":
				if a.Hit != 95 {
					t.Fatal(a)
				}
			case "job-75-entry-3":
				if a.Status != "" || e.Status != corebattle.StatusPoison {
					t.Fatal(a, e)
				}
			case "job-75-entry-4":
				if a.HP != 230 || a.MP != 182 || c.MPHealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-75-entry-6":
				if e.Status != corebattle.StatusKinju || e.Temporary != corebattle.TemporaryDouble || e.Tension != 1 {
					t.Fatal(e)
				}
			case "job-76-entry-0":
				if e.Hit != 86 || c.PartyStatDownCalls != 1 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-76-entry-1":
				if e.Status != corebattle.StatusSleep || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-76-entry-2":
				if e.HP != 833 || a.HP != 572 || c.DamageCalls != 1 || c.HealCalls != 1 {
					t.Fatal(a, e, c)
				}
			case "job-76-entry-4":
				if a.Status != "" || c.CureCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-76-entry-5":
				if e.HP != 855 || c.DamageCalls != 1 || a.Tension != 1 {
					t.Fatal(a, e, c)
				}
			}
		})
	}
}
