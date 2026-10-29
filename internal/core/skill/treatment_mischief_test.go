package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func treatmentFixture() (corebattle.LifetimeState, *corebattle.TreatmentMischiefContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP, s.Actors[0].MaxMP, s.Actors[0].Defense = 500, 200, 80
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 40}
	s.Actors[1].HP, s.Actors[1].MaxHP, s.Actors[1].MP, s.Actors[1].MaxMP, s.Actors[1].Hit = 900, 900, 100, 100, 95
	s.Actors[2].MaxHP = 101
	c := &corebattle.TreatmentMischiefContext{DamageContext: base.DamageContext}
	c.Status.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
		Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
	}}
	var d BattleDefinitions
	d.RegisterTreatmentMischief(c)
	d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	return s, c, d
}

func executeTreatment(t *testing.T, s *corebattle.LifetimeState, c *corebattle.TreatmentMischiefContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Every complete intrinsic vector is independently derived from ff97fc5:
// _skill.cgi:3572–3686,3815–3934,6137–6143; registry presence is not the oracle.
func TestTreatmentMischiefManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, hp, damage int
		draws                        []float64
	}{
		{50, 0, 10, 5, 572, 0, []float64{0}},
		{50, 1, 25, 3, 500, 0, nil},
		{50, 2, 40, 14, 500, 900, []float64{0}},
		{50, 3, 55, 10, 500, 0, nil},
		{50, 4, 75, 24, 680, 0, []float64{0}},
		{51, 0, 10, 6, 500, 0, nil},
		{51, 1, 30, 11, 500, 0, nil},
		{51, 2, 50, 14, 590, 0, []float64{0, 0, 0}},
		{51, 3, 80, 16, 500, 0, []float64{0, 0}},
		{51, 4, 110, 34, 500, 153, []float64{0}},
		{51, 6, 160, 46, 500, 63, []float64{.5, 0}},
		{51, 7, 160, 80, 500, 126, []float64{0, 0}},
		{55, 0, 5, 0, 500, 0, []float64{.7}},
		{55, 1, 10, 4, 500, 0, []float64{0}},
		{55, 2, 20, 0, 500, 0, []float64{.7}},
		{55, 3, 35, 0, 500, 0, []float64{.5}},
		{55, 4, 50, 8, 500, 25, []float64{0}},
		{55, 5, 65, 0, 500, 0, nil},
		{56, 0, 6, 5, 500, 22, []float64{0}},
		{56, 1, 16, 1, 500, 0, nil},
		{56, 2, 36, 12, 500, 63, []float64{0}},
		{56, 4, 66, 34, 500, 144, []float64{0}},
		{57, 0, 10, 3, 527, 0, []float64{0}},
		{57, 1, 20, 6, 500, 54, []float64{0}},
		{57, 2, 40, 10, 581, 0, []float64{0}},
		{57, 3, 60, 20, 500, 49, []float64{0, 0}},
		{57, 4, 80, 20, 500, 0, []float64{0}},
		{57, 5, 100, 4, 500, 0, []float64{0}},
		{57, 6, 120, 33, 500, 0, nil},
		{78, 3, 55, 14, 680, 0, []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := treatmentFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (id == "job-51-entry-7") {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if row.job == 50 && row.pos != 2 || row.job == 57 && (row.pos == 0 || row.pos == 2) {
				args[0] = "self"
			}
			if id == "job-57-entry-4" {
				args[0] = "fallen"
			}
			executeTreatment(t, &s, c, d, id, args, row.draws...)
			a, e, f := s.Actors[0], s.Actors[1], s.Actors[2]
			if a.HP != row.hp || 900-e.HP != row.damage || a.CMP != 90 {
				t.Fatal(a, e, f)
			}
			wantMP := 80
			switch id {
			case "job-50-entry-1":
				if a.Hit != 95 {
					t.Fatal(a)
				}
			case "job-50-entry-2":
				if c.Status.DeathCalls != 1 {
					t.Fatal(c.Status)
				}
			case "job-50-entry-3":
				if a.Defense != 112 || a.Temporary != corebattle.TemporaryReduceBreath || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-51-entry-0":
				if e.Hit != 76 || c.PartyStatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-51-entry-1":
				if f.Hit != 95 {
					t.Fatal(f)
				}
			case "job-51-entry-2":
				if c.PartyHealCalls != 1 || c.HealCalls != 2 {
					t.Fatal(c)
				}
			case "job-51-entry-3":
				if e.Status != corebattle.StatusConfusion || c.Status.StatusCalls != 1 || c.Status.AreaStatusCalls != 0 {
					t.Fatal(e, c.Status)
				}
			case "job-51-entry-6":
				if !c.AdditionalEffect || c.Status.AreaDeathCalls != 1 || c.AreaDamageCalls != 1 {
					t.Fatal(c)
				}
			case "job-51-entry-7":
				wantMP = 86
			case "job-55-entry-0":
				if f.Hit != 49 {
					t.Fatal(f)
				}
			case "job-55-entry-2":
				if f.Status != corebattle.StatusMagicSeal || f.Tension != 1 {
					t.Fatal(f)
				}
			case "job-55-entry-1", "job-57-entry-5":
				wantMP = 94
				if e.MP != 85 || c.MPHealCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-55-entry-3", "job-55-entry-5":
				if e.Tension != 1.7 || c.TensionCalls != 1 || f.Tension != 1 {
					t.Fatal(e, f, c)
				}
			case "job-56-entry-1":
				if a.Temporary != corebattle.TemporaryAbsorbMagic {
					t.Fatal(a)
				}
			case "job-57-entry-1":
				if e.Hit != 89 || !c.AdditionalEffect {
					t.Fatal(e, c)
				}
			case "job-57-entry-3":
				if e.Status != corebattle.StatusSleep || !c.AdditionalEffect {
					t.Fatal(e, c)
				}
			case "job-57-entry-4":
				if f.HP != 50 {
					t.Fatal(f)
				}
			case "job-57-entry-6":
				if a.Attack != 240 || c.StatUpCalls != 2 {
					t.Fatal(a, c)
				}
			case "job-78-entry-3":
				if a.Defense != 64 {
					t.Fatal(a)
				}
			}
			if a.MP != wantMP {
				t.Fatal(a.MP, wantMP)
			}
			consumed := row.job == 50 || row.job == 57 && row.pos != 6 || id == "job-51-entry-7" || id == "job-55-entry-1" || id == "job-55-entry-5"
			wantArgs := args
			if consumed {
				wantArgs = args[1:]
			}
			if id != "job-57-entry-4" && fmt.Sprint(c.Arguments) != fmt.Sprint(wantArgs) {
				t.Fatal(c.Arguments, args)
			}
		})
	}
}
