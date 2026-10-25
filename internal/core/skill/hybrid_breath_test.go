package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func hybridBreathFixture() (corebattle.LifetimeState, *corebattle.HybridBreathContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP, s.Actors[0].Defense = 500, 80
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 40}
	s.Actors[1].BaseStats.Defense, s.Actors[1].Defense = 80, 80
	s.Actors[1].HP = 900
	c := &corebattle.HybridBreathContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterHybridBreath(c)
	return s, c, d
}

func executeHybridBreath(t *testing.T, s *corebattle.LifetimeState, c *corebattle.HybridBreathContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Complete intrinsic vectors: ff97fc5:_skill.cgi:3074–3125,3222–3238.
func TestHybridBreathManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
	}{
		{39, 0, 3, 5, 22}, {39, 1, 7, 7, 0}, {39, 2, 11, 3, 0},
		{39, 3, 16, 7, 0}, {39, 4, 28, 10, 0}, {39, 6, 99, 40, 162}, {39, 7, 99, 10, 64},
		{40, 0, 25, 8, 63}, {40, 1, 50, 11, 54}, {40, 2, 99, 1, 144},
		{41, 0, 10, 1, 13}, {41, 1, 30, 6, 49}, {41, 2, 60, 14, 103},
		{41, 3, 90, 9, 0}, {41, 4, 120, 34, 175}, {41, 5, 120, 45, 162},
		{44, 0, 20, 4, 41}, {44, 1, 40, 5, 0}, {44, 2, 60, 8, 50},
		{44, 3, 80, 14, 153}, {44, 4, 100, 7, 0}, {44, 5, 120, 15, 54},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := hybridBreathFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 39 && row.pos == 7 || row.job == 41 && row.pos == 5) {
				t.Fatal(entry)
			}
			args, draws := []string{"enemy", "tail"}, []float64{0}
			if id == "job-39-entry-2" || id == "job-44-entry-4" {
				args[0] = "self"
			}
			if id == "job-39-entry-1" || id == "job-39-entry-3" || id == "job-44-entry-1" {
				draws = nil
			}
			executeHybridBreath(t, &s, c, d, id, args, draws...)
			a, e := s.Actors[0], s.Actors[1]
			wantMP := 80
			if id == "job-40-entry-2" {
				wantMP = 1
			}
			if 900-e.HP != row.damage || a.MP != wantMP || a.CMP != 90 {
				t.Fatal(a, e)
			}
			switch id {
			case "job-39-entry-1":
				// The fallen ally's ordinary buff redirects back to the caster.
				if a.Defense != 120 || c.PartyStatUpCalls != 1 || c.StatUpCalls != 2 {
					t.Fatal(a, c)
				}
			case "job-39-entry-2":
				if a.HP != 527 || c.HealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-39-entry-3":
				if e.Defense != 60 || c.PartyStatDownCalls != 1 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-39-entry-4":
				if e.Status != corebattle.StatusConfusion || c.StatusCalls != 1 || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-41-entry-3":
				if e.Status != corebattle.StatusParalyze || c.StatusCalls != 1 || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-44-entry-1":
				if a.Defense != 120 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-44-entry-2":
				if a.HP != 496 || c.End.ImmediateRiskCalls != 1 {
					t.Fatal(a, c.End)
				}
			case "job-44-entry-4":
				if a.HP != 635 || c.HealCalls != 1 {
					t.Fatal(a, c)
				}
			}
			consumed := row.job == 44 && row.pos != 1 || row.job == 40 && row.pos == 0 || row.job == 39 && (row.pos == 2 || row.pos == 7) || row.job == 41 && row.pos == 5
			wantArgs := args
			if consumed {
				wantArgs = args[1:]
			}
			if fmt.Sprint(c.Arguments) != fmt.Sprint(wantArgs) {
				t.Fatal(c.Arguments, wantArgs)
			}
		})
	}
}
