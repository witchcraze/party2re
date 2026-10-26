package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func basicMagicFixture() (corebattle.LifetimeState, *corebattle.BasicMagicContext, BattleDefinitions) {
	s, base, _ := timeMagicFixture()
	s.Actors[1].Hit = 95
	c := &corebattle.BasicMagicContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterBasicMagic(c)
	return s, c, d
}

func executeBasicMagic(t *testing.T, s *corebattle.LifetimeState, c *corebattle.BasicMagicContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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
}

// Independent intrinsic vectors: ff97fc5:_skill.cgi, complete spans per row.
func TestBasicMagicManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, line int
		damage                 int
		draws                  []float64
	}{
		{5, 0, 0, 0, 1235, 12, []float64{0}},
		{5, 1, 1, 3, 1237, 0, nil},
		{5, 2, 3, 2, 1238, 0, nil},
		{5, 4, 12, 4, 1240, 9, []float64{0, 0}},
		{5, 5, 24, 10, 1241, 0, []float64{0}},
		{5, 6, 45, 10, 1242, 22, []float64{0, 0}},
		{5, 8, 100, 30, 1255, 0, []float64{0}},
		{5, 9, 100, 23, 1256, 72, []float64{0, 0}},
		{6, 0, 0, 0, 1264, 12, []float64{0}},
		{6, 2, 4, 4, 1267, 0, nil},
		{6, 4, 14, 7, 1269, 0, nil},
		{6, 5, 20, 8, 1270, 63, []float64{0}},
		{6, 6, 30, 8, 1271, 0, []float64{0}},
		{6, 7, 55, 11, 1272, 54, []float64{0}},
		{6, 8, 90, 30, 1273, 198, []float64{0}},
		{6, 9, 90, 40, 1274, 540, []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := basicMagicFixture()
			s.Actors[0].Status = corebattle.StatusPoison
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.pos == 0 || row.pos == 9) {
				t.Fatal(row.line, entry)
			}
			args := []string{"enemy", "tail"}
			if row.job == 5 && (row.pos == 1 || row.pos == 2 || row.pos == 5 || row.pos == 8) {
				args[0] = "self"
			}
			executeBasicMagic(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || a.MP != 80 || a.CMP != 90 {
				t.Fatal(row.line, s)
			}
			switch id {
			case "job-5-entry-1":
				if a.Defense != 112 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-5-entry-2":
				if a.Status != "" || c.CureCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-5-entry-5", "job-5-entry-8":
				want := 581
				if row.pos == 8 {
					want = 1000
				}
				if a.HP != want || c.HealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-6-entry-2":
				if e.Defense != 48 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-6-entry-4":
				if e.Hit != 76 || c.StatDownCalls != 1 || c.PartyStatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-6-entry-6":
				if e.Status != corebattle.StatusSleep || c.StatusCalls != 1 {
					t.Fatal(e, c)
				}
			}
			area := row.job == 5 && (row.pos == 4 || row.pos == 6 || row.pos == 9) || row.job == 6 && (row.pos == 4 || row.pos == 7)
			wantArgs := args[1:]
			if area {
				wantArgs = args
			}
			if !reflect.DeepEqual(c.Arguments, wantArgs) || row.damage > 0 && c.DamageCalls != 1 {
				t.Fatal(c.Arguments, wantArgs, c)
			}
		})
	}
}
