package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func chanceFixture() (corebattle.LifetimeState, *corebattle.TreatmentChanceContext, BattleDefinitions) {
	s, base, _ := treatmentFixture()
	s.Actors[1].Attack, s.Actors[1].Defense = 100, 80
	s.Actors[1].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80}
	c := &corebattle.TreatmentChanceContext{DamageContext: base.DamageContext, Status: base.Status}
	var d BattleDefinitions
	d.RegisterTreatmentChance(c)
	d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	return s, c, d
}

func executeChance(t *testing.T, s *corebattle.LifetimeState, c *corebattle.TreatmentChanceContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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
		t.Fatal(id, "draws", rng.used, len(draws))
	}
}

// Intrinsic oracles: ff97fc5:_skill.cgi:3186–3206,3249–3307,3317–3413.
// Expected results are source vectors, not a comparison to primitive execution.
func TestTreatmentChanceManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, selfHP, selfMP, enemyHP int
		target                                    string
		draws                                     []float64
	}{
		{43, 0, 5, 2, 500, 80, 900, "self", nil},
		{43, 1, 10, 3, 500, 80, 900, "self", nil},
		{43, 2, 20, 5, 500, 80, 900, "self", nil},
		{43, 3, 40, 7, 500, 80, 900, "self", nil},
		{43, 4, 70, 10, 500, 80, 900, "self", nil},
		{43, 5, 110, 35, 860, 80, 900, "self", []float64{0}},
		{45, 0, 10, 5, 500, 80, 900, "self", nil},
		{45, 1, 30, 8, 500, 80, 900, "enemy", []float64{0}},
		{45, 2, 50, 7, 500, 80, 900, "enemy", nil},
		{45, 3, 70, 3, 500, 97, 900, "enemy", []float64{0}},
		{45, 4, 90, 30, 500, 80, 900, "enemy", nil},
		{45, 5, 120, 7, 500, 80, 900, "self", nil},
		{45, 6, 150, 40, 500, 80, 630, "enemy", []float64{0}},
		{45, 7, 150, 20, 590, 96, 900, "enemy", []float64{0, 0, 0}},
		{46, 0, 10, 7, 500, 80, 787, "enemy", []float64{.21, .21, .21, 0}},
		{46, 1, 30, 14, 500, 80, 0, "enemy", []float64{0}},
		{46, 2, 60, 6, 444, 80, 851, "enemy", []float64{0, 0, .99, 0}},
		{46, 3, 80, 4, 500, 80, 0, "enemy", []float64{.5}},
		{46, 4, 140, 36, 500, 80, 801, "enemy", []float64{0, 0, .99, 0}},
		{46, 5, 140, 44, 250, 80, 900, "enemy", []float64{.5}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := chanceFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 45 && row.pos == 7 || row.job == 46 && row.pos == 5) {
				t.Fatal(entry)
			}
			s.Actors[0].Hit = 12
			switch id {
			case "job-43-entry-0":
				s.Actors[0].Status = corebattle.StatusPoison
			case "job-43-entry-1":
				s.Actors[0].Status = corebattle.StatusParalyze
			case "job-43-entry-2", "job-43-entry-4":
				s.Actors[0].Status = corebattle.StatusSleep
			}
			args := []string{row.target, "tail"}
			executeChance(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if a.HP != row.selfHP || a.MP != row.selfMP || e.HP != row.enemyHP || a.CMP != 90 {
				t.Fatal(a, e, c)
			}
			switch id {
			case "job-43-entry-0", "job-43-entry-1", "job-43-entry-2":
				if a.Status != "" || c.Status.PartyCureCalls != 1 || !c.AdditionalEffect {
					t.Fatal(a, c)
				}
			case "job-43-entry-3", "job-45-entry-5":
				if a.Temporary != corebattle.TemporaryRegeneration {
					t.Fatal(a)
				}
			case "job-43-entry-4":
				if a.Status != "" || c.Status.CureCalls != 1 {
					t.Fatal(a, c.Status)
				}
			case "job-45-entry-0":
				if a.Hit != 95 {
					t.Fatal(a)
				}
			case "job-45-entry-1":
				if e.Status != corebattle.StatusDofuu || e.Tension != 1 || c.Status.StatusCalls != 1 {
					t.Fatal(e, c.Status)
				}
			case "job-45-entry-2":
				if a.Defense != 120 || a.Temporary != corebattle.TemporaryReduceMagic || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-45-entry-3":
				if e.MP != 82 || c.MPHealCalls != 1 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-45-entry-4":
				if e.Attack != 60 || e.Defense != 48 || e.Icon != "assets/images/placeholders/battle/frog.svg" || c.StatDownCalls != 2 {
					t.Fatal(e, c)
				}
			case "job-45-entry-7":
				if c.TensionCalls != 2 || c.HealCalls != 1 || c.MPHealCalls != 1 || s.Actors[2].Tension != 1.7 {
					t.Fatal(c, s.Actors)
				}
			case "job-46-entry-2":
				if c.End.ImmediateRiskCalls != 1 {
					t.Fatal(c.End)
				}
			}
			consumes := row.job == 43 && row.pos >= 3 || row.job == 45 && row.pos != 2 && row.pos != 7 || row.job == 46 && (row.pos == 1 || row.pos == 2 || row.pos == 4)
			want := args
			if consumes {
				want = args[1:]
			}
			if fmt.Sprint(c.Arguments) != fmt.Sprint(want) {
				t.Fatal(c.Arguments, want)
			}
		})
	}
}
