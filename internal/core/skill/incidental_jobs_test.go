package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func incidentalFixture() (corebattle.LifetimeState, *corebattle.IncidentalJobContext, BattleDefinitions) {
	s, base, _ := treatmentFixture()
	s.Actors[1].Defense, s.Actors[1].Agility = 80, 30
	s.Actors[1].BaseStats.Defense, s.Actors[1].BaseStats.Agility = 80, 30
	c := &corebattle.IncidentalJobContext{DamageContext: base.DamageContext, Status: base.Status}
	var d BattleDefinitions
	d.RegisterIncidentalJobs(c)
	return s, c, d
}

func executeIncidental(t *testing.T, s *corebattle.LifetimeState, c *corebattle.IncidentalJobContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.CallbackContext {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(id, err, rng.used, len(draws))
	}
	return f
}

// Independently derived from ff97fc5:party2/lib/_skill.cgi:1290–1536.
// Each position has its own intrinsic result, even when a helper is shared.
func TestIncidentalJobManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, hp, damage int
		target                       string
		draws                        []float64
	}{
		{7, 1, 2, 2, 500, 0, "self", nil}, {7, 3, 12, 6, 500, 0, "enemy", nil},
		{7, 5, 35, 4, 497, 36, "enemy", []float64{0}}, {7, 6, 45, 1, 500, 0, "self", []float64{0}},
		{7, 7, 65, 7, 500, 0, "enemy", []float64{0}},
		{8, 1, 1, 0, 950, 0, "ignored", []float64{0}}, {8, 2, 4, 0, 500, 0, "ignored", []float64{.4}},
		{8, 3, 8, 0, 500, 0, "ignored", []float64{.9}}, {8, 4, 12, 0, 500, 0, "ignored", []float64{0}},
		{8, 5, 18, 0, 500, 0, "ignored", []float64{.5}}, {8, 6, 24, 0, 500, 0, "fallen", nil},
		{8, 7, 36, 0, 500, 0, "ignored", []float64{.9, .5, .5}}, {8, 8, 50, 0, 500, 0, "ignored", nil},
		{9, 0, 0, 0, 500, 3, "enemy", []float64{0}}, {9, 1, 3, 3, 500, 0, "enemy", nil},
		{9, 2, 6, 3, 500, 0, "self", nil}, {9, 3, 12, 7, 500, 0, "enemy", []float64{0}},
		{9, 5, 35, 9, 500, 0, "enemy", []float64{0}}, {9, 7, 70, 8, 500, 32, "enemy", []float64{0, 0}},
		{9, 8, 90, 15, 500, 0, "enemy", nil}, {9, 9, 90, 20, 519, 22, "enemy", []float64{0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := incidentalFixture()
			if id == "job-8-entry-8" || id == "job-9-entry-8" {
				s.Actors[1].Tension = 5
			}
			if id == "job-9-entry-8" {
				s.Actors[1].Defense, s.Actors[1].Temporary = 200, corebattle.TemporaryDouble
			}
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 9 && (row.pos == 0 || row.pos == 9)) {
				t.Fatal(entry, row)
			}
			executeIncidental(t, &s, c, d, id, []string{row.target, "tail"}, row.draws...)
			if s.Actors[0].HP != row.hp || 900-s.Actors[1].HP != row.damage || s.Actors[0].CMP != 90 {
				t.Fatal(row, s.Actors, c)
			}
			shifted := row.job == 7 && (row.pos == 5 || row.pos == 6) || row.job == 8 && row.pos == 6 || row.job == 9 && (row.pos == 0 || row.pos == 1 || row.pos == 2 || row.pos >= 7)
			wantArgs := 2
			if shifted {
				wantArgs = 1
			}
			if len(c.Arguments) != wantArgs || c.Arguments[len(c.Arguments)-1] != "tail" {
				t.Fatal(c.Arguments)
			}
			wantMP := 80
			if id == "job-7-entry-6" {
				wantMP = 77
			}
			if s.Actors[0].MP != wantMP {
				t.Fatal(s.Actors[0])
			}
			switch id {
			case "job-7-entry-1":
				if s.Actors[0].Defense != 112 || c.StatUpCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-7-entry-3":
				if s.Actors[1].Hit != 76 || c.PartyStatDownCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-7-entry-5":
				if c.DamageCalls != 1 || c.End.ImmediateRiskCalls != 1 {
					t.Fatal(c)
				}
			case "job-7-entry-6":
				if c.MPHealCalls != 1 {
					t.Fatal(c)
				}
			case "job-7-entry-7", "job-9-entry-3":
				if s.Actors[1].Status != corebattle.StatusConfusion || c.Status.AreaStatusCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-8-entry-1":
				if s.Actors[0].Status != corebattle.StatusSleep || c.HealCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-8-entry-2", "job-9-entry-7":
				if s.Actors[1].Status != corebattle.StatusParalyze {
					t.Fatal(s)
				}
			case "job-8-entry-3":
				if s.Actors[2].Status != corebattle.StatusDofuu {
					t.Fatal(s)
				}
			case "job-8-entry-4":
				if s.Actors[0].Status != corebattle.StatusDeadlyPoison {
					t.Fatal(s)
				}
			case "job-8-entry-5":
				if s.Actors[1].Tension != 1.7 || c.TensionCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-8-entry-6":
				if s.Actors[2].Tension != 1.7 || c.TensionCalls != 1 {
					t.Fatal(s, c)
				}
			case "job-8-entry-7":
				if len(c.JokeOutcomes) != 1 || c.JokeOutcomes[0] != 6 || s.Actors[1].Status != corebattle.StatusDofuu || s.Actors[2].Status != corebattle.StatusDofuu {
					t.Fatal(s, c)
				}
			case "job-8-entry-8":
				if s.Actors[1].Tension != 1 {
					t.Fatal(s)
				}
			case "job-9-entry-1":
				if s.Actors[1].Agility != 18 {
					t.Fatal(s)
				}
			case "job-9-entry-2":
				if s.Actors[0].Agility != 56 {
					t.Fatal(s)
				}
			case "job-9-entry-5":
				if s.Actors[1].Status != corebattle.StatusSleep {
					t.Fatal(s)
				}
			case "job-9-entry-8":
				if s.Actors[1].Defense != 72 || s.Actors[1].Temporary != "" || s.Actors[1].Tension != 1 {
					t.Fatal(s)
				}
			case "job-9-entry-9":
				if s.Actors[1].Defense != 80 || s.Actors[0].Hit != 95 || c.HealCalls != 1 {
					t.Fatal(s, c)
				}
			}
		})
	}
}
