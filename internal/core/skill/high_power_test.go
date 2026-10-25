package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func highPowerFixture() (corebattle.LifetimeState, *corebattle.HighPowerContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 50, Agility: 40}
	s.Actors[0].Defense, s.Actors[0].HP = 80, 500
	s.Actors[1].HP, s.Actors[1].Hit = 900, 95
	c := &corebattle.HighPowerContext{DamageContext: base.DamageContext, EquipmentReset: &corebattle.EquipmentResetContext{}}
	d := BattleDefinitions{}
	d.RegisterHighPower(c)
	return s, c, d
}

func executeHighPower(t *testing.T, s *corebattle.LifetimeState, c *corebattle.HighPowerContext, d BattleDefinitions, ids []string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(ids, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(ids, err)
	}
	if rng.used != len(draws) {
		t.Fatal(ids, rng.used, len(draws))
	}
}

// Independent intrinsic vectors: ff97fc5:_skill.cgi:3418–3568,3693–3718.
func TestHighPowerManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
		draws                    []float64
	}{
		{47, 0, 20, 5, 54, []float64{0}}, {47, 1, 50, 9, 72, []float64{0}},
		{47, 2, 80, 20, 81, []float64{0}}, {47, 3, 120, 36, 99, []float64{0}},
		{47, 4, 160, 40, 180, []float64{0, 0, 0, 0, 0, 0, 0, 0}}, {47, 5, 160, 47, 0, nil},
		{48, 0, 40, 56, 108, []float64{0, 0}}, {48, 1, 80, 66, 63, []float64{0, 0}},
		{48, 2, 120, 66, 270, []float64{0}}, {48, 3, 160, 44, 899, []float64{0}},
		{48, 4, 200, 46, 54, []float64{0}}, {48, 5, 200, 20, 0, nil},
		{49, 0, 100, 10, 0, nil}, {49, 1, 200, 20, 0, nil}, {49, 2, 300, 30, 72, []float64{0}},
		{52, 0, 20, 4, 54, []float64{0}}, {52, 1, 50, 8, 0, nil}, {52, 2, 80, 15, 0, nil},
		{52, 3, 130, 20, 0, nil}, {52, 4, 150, 30, 180, []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := highPowerFixture()
			if id == "job-48-entry-3" {
				s.Actors[1].MaxHP = 999
			}
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.pos == 5) {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if id == "job-49-entry-0" {
				args[0] = "self"
				s.Actors[0].Status = corebattle.StatusPoison
			}
			executeHighPower(t, &s, c, d, []string{id}, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || a.MP != 80 || a.CMP != 90 {
				t.Fatal(a, e)
			}
			switch id {
			case "job-47-entry-1":
				if a.HP != 495 || c.End.ImmediateRiskCalls != 1 {
					t.Fatal(a, c.End)
				}
			case "job-47-entry-2":
				if a.Attack != 90 {
					t.Fatal(a)
				}
			case "job-47-entry-4":
				if c.DamageCalls != 4 || len(c.Arguments) != 2 {
					t.Fatal(c)
				}
			case "job-47-entry-5":
				if a.Attack != 225 || a.Temporary != corebattle.TemporaryRegeneration || a.Hit != 95 {
					t.Fatal(a)
				}
			case "job-48-entry-0":
				if e.Status != corebattle.StatusDeadlyPoison || c.StatusCalls != 1 || c.AreaStatusCalls != 1 || !c.AdditionalEffect {
					t.Fatal(e, c)
				}
			case "job-48-entry-1":
				if e.Status != corebattle.StatusSleep || c.StatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-48-entry-2":
				if e.Hit != 88 || c.StatDownCalls != 1 || !c.AdditionalEffect {
					t.Fatal(e, c)
				}
			case "job-48-entry-5":
				if s.Actors[2].HP != 1 || a.Defense != 50 || a.Temporary != corebattle.TemporaryAbsorbMagic {
					t.Fatal(s)
				}
			case "job-49-entry-0":
				if a.Status != "" || c.CureCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-49-entry-1":
				if a.Defense != 110 || a.Temporary != corebattle.TemporaryReduceAttack {
					t.Fatal(a)
				}
			case "job-52-entry-1":
				if a.Attack != 200 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-52-entry-2":
				if e.Defense != 0 || e.Temporary != "" || e.Tension != 1 {
					t.Fatal(e)
				}
			case "job-52-entry-3":
				if a.Temporary != corebattle.TemporaryDouble || a.Tension != 3 || c.TensionCalls != 2 {
					t.Fatal(a, c)
				}
			case "job-52-entry-4":
				if a.Temporary != corebattle.TemporaryDouble || c.AreaDamageCalls != 1 {
					t.Fatal(a, c)
				}
			}
		})
	}
}
