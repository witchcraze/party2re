package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func machineFamiliarFixture() (corebattle.LifetimeState, *corebattle.MachineFamiliarContext, BattleDefinitions) {
	s, base, _ := hybridBreathFixture()
	s.Actors[0].MaxMP = 200
	s.Actors[1].MP, s.Actors[1].MaxMP = 100, 100
	c := &corebattle.MachineFamiliarContext{DamageContext: base.DamageContext, RequesterGender: "f"}
	var d BattleDefinitions
	d.RegisterMachineFamiliar(c)
	return s, c, d
}

func executeMachineFamiliar(t *testing.T, s *corebattle.LifetimeState, c *corebattle.MachineFamiliarContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Full intrinsic vectors: ff97fc5:_skill.cgi:6246–6254,6297–6302,
// 6475,6515–6532,6555–6593. Shared primitive tests alone do not certify entries.
func TestMachineFamiliarManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
		draws                    []float64
	}{
		{79, 0, 11, 2, 0, []float64{0}},
		{79, 1, 33, 8, 18, []float64{0}},
		{79, 4, 99, 27, 198, []float64{0}},
		{81, 0, 10, 4, 32, []float64{0}},
		{81, 3, 95, 10, 113, []float64{0, 0}},
		{81, 4, 130, 43, 0, []float64{0, 0}},
		{82, 1, 25, 7, 32, []float64{.5, 0}},
		{82, 2, 50, 23, 0, []float64{0}},
		{82, 3, 65, 3, 0, []float64{0}},
		{82, 4, 80, 11, 54, []float64{0, 0, 0, 0, 0, 0, 0}},
		{82, 5, 111, 20, 153, []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if id == "job-82-entry-2" {
				args[0], s.Actors[0].Status = "self", corebattle.StatusPoison
			}
			executeMachineFamiliar(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			wantMP := 80
			if id == "job-79-entry-0" {
				wantMP = 103 // requested25 -> floor(25×.9)+1.
				if e.MP != 75 || c.StatDownCalls != 1 || c.MPHealCalls != 1 || a.HP != 500 {
					t.Fatal(s, c)
				}
			}
			if 900-e.HP != row.damage || a.MP != wantMP || a.CMP != 90 || a.Attack != 100 && id != "job-81-entry-4" && id != "job-82-entry-3" {
				t.Fatal(a, e)
			}
			switch id {
			case "job-79-entry-4":
				if a.Tension != 1.7 || c.TensionCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-81-entry-4":
				if a.Attack != 120 || a.Defense != 96 || a.HP != 635 || c.StatUpCalls != 2 || c.HealCalls != 1 || c.PartyStatUpCalls != 0 || s.Actors[2].Attack != 0 {
					t.Fatal(s, c)
				}
			case "job-82-entry-2":
				if a.HP != 635 || a.Status != "" || c.HealCalls != 1 || c.CureCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-82-entry-3":
				if a.Attack != 118 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-82-entry-4":
				if c.DamageCalls != 3 {
					t.Fatal(c)
				}
			}
			wantArgs := args[1:]
			if id == "job-81-entry-4" || id == "job-82-entry-3" || id == "job-82-entry-4" {
				wantArgs = args
			}
			if fmt.Sprint(c.Arguments) != fmt.Sprint(wantArgs) {
				t.Fatal(c.Arguments, wantArgs)
			}
		})
	}
}
