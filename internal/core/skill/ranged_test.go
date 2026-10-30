package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func rangedFixture() (corebattle.LifetimeState, *corebattle.RangedContext, BattleDefinitions) {
	s, base, _ := songDanceFixture()
	s.Actors[0].Hit = 12
	s.Actors[1].MaxHP, s.Actors[1].Hit = 900, 95
	c := &corebattle.RangedContext{DamageContext: base.DamageContext, Status: *base.Status}
	var d BattleDefinitions
	d.RegisterRanged(c)
	return s, c, d
}

func executeRanged(t *testing.T, s *corebattle.LifetimeState, c *corebattle.RangedContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.CallbackContext {
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

// Intrinsic vectors: ff97fc5:_skill.cgi:1603–1664,3132–3179,3778–3806.
func TestRangedManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
		extended                 bool
		draws                    []float64
	}{
		{11, 0, 0, 0, 25, true, []float64{0}},
		{11, 1, 5, 3, 0, false, []float64{0}},
		{11, 2, 10, 0, 0, false, nil},
		{11, 3, 20, 8, 25, false, []float64{0}},
		{11, 4, 40, 4, 0, false, []float64{0}},
		{11, 5, 60, 6, 54, false, []float64{0}},
		{11, 6, 90, 20, 49, false, []float64{0, 0}},
		{11, 7, 110, 24, 114, false, make([]float64, 7)},
		{11, 8, 110, 12, 0, true, nil},
		{42, 0, 15, 7, 0, false, []float64{0}},
		{42, 1, 30, 6, 0, false, []float64{0}},
		{42, 2, 55, 10, 0, false, nil},
		{42, 3, 80, 4, 49, false, []float64{0, 0}},
		{42, 4, 100, 8, 54, false, []float64{0, 0}},
		{42, 5, 120, 42, 675, false, []float64{0}},
		{42, 6, 150, 24, 900, false, []float64{0, 0}},
		{42, 7, 150, 42, 61, true, []float64{0}},
		{54, 0, 5, 4, 0, false, []float64{0}},
		{54, 1, 15, 7, 54, false, []float64{0}},
		{54, 2, 30, 0, 0, false, nil},
		{54, 3, 60, 12, 45, false, []float64{0, 0}},
		{54, 4, 90, 34, 162, false, []float64{0}},
		{54, 5, 130, 44, 189, false, make([]float64, 7)},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := rangedFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != row.extended {
				t.Fatal(entry)
			}
			f := executeRanged(t, &s, c, d, id, []string{"enemy", "tail"}, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || a.HP != 500 || a.CMP != 90 {
				t.Fatal(a, e, row.damage)
			}
			wantMP := 80
			switch id {
			case "job-11-entry-1", "job-54-entry-0", "job-54-entry-3":
				if e.Status != corebattle.StatusDofuu || c.Status.StatusCalls != 1 {
					t.Fatal(e, c.Status)
				}
			case "job-11-entry-2", "job-54-entry-2":
				if a.Hit != 95 {
					t.Fatal(a)
				}
			case "job-11-entry-4":
				wantMP = 94
				if e.MP != 85 || c.StatDownCalls != 1 || c.MPHealCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-11-entry-5":
				if e.Hit != 89 || c.StatDownCalls != 1 || !c.AdditionalEffect {
					t.Fatal(e, c)
				}
			case "job-11-entry-6":
				if e.Status != corebattle.StatusSleep || c.Status.StatusCalls != 1 {
					t.Fatal(e, c.Status)
				}
			case "job-11-entry-8":
				if a.Extended != corebattle.ExtendedBow || a.ExtendedTurns != 10 || a.Tension != 1.7 || c.TensionCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-42-entry-0":
				if e.Status != corebattle.StatusConfusion {
					t.Fatal(e)
				}
			case "job-42-entry-1":
				if e.Status != corebattle.StatusMagicSeal {
					t.Fatal(e)
				}
			case "job-42-entry-2":
				if a.Agility != 80 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-42-entry-3", "job-42-entry-5":
				if e.Status != corebattle.StatusDeadlyPoison {
					t.Fatal(e)
				}
			case "job-42-entry-4":
				if e.Status != corebattle.StatusParalyze {
					t.Fatal(e)
				}
			case "job-42-entry-6":
				if c.Status.DeathCalls != 1 || c.DamageCalls != 1 {
					t.Fatal(c)
				}
			}
			if a.MP != wantMP {
				t.Fatal(a)
			}
			unshifted := row.job == 11 && (row.pos == 2 || row.pos == 3 || row.pos == 7 || row.pos == 8) || row.job == 42 && row.pos == 2 || row.job == 54 && (row.pos == 2 || row.pos == 4 || row.pos == 5)
			wantArgs := []string{"tail"}
			if unshifted {
				wantArgs = []string{"enemy", "tail"}
			}
			if !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"}) || !reflect.DeepEqual(c.Arguments, wantArgs) {
				t.Fatal(f, c.Arguments)
			}
		})
	}
}
