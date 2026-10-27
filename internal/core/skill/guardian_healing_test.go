package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func guardianFixture() (corebattle.LifetimeState, *corebattle.GuardianHealingContext, BattleDefinitions) {
	s, base, _ := hybridBreathFixture()
	s.Actors[2].MaxHP = 1000
	c := &corebattle.GuardianHealingContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterGuardianHealing(c)
	return s, c, d
}

func executeGuardian(t *testing.T, s *corebattle.LifetimeState, c *corebattle.GuardianHealingContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Complete intrinsic vectors: ff97fc5:_skill.cgi:1873–1986. Other positions
// keep their revival owner. Costs here are metadata, not naked-body charges.
func TestGuardianHealingManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, hp, damage, heals, cures int
		tmp                                        corebattle.TemporaryEffect
		draws                                      []float64
	}{
		{17, 0, 10, 0, 500, 0, 0, 0, "", nil},
		{17, 1, 20, 3, 527, 0, 1, 0, "", []float64{0}},
		{17, 2, 40, 6, 500, 0, 0, 0, corebattle.TemporaryReduceMagic, nil},
		{17, 3, 60, 5, 500, 0, 0, 2, "", nil},
		{17, 5, 130, 20, 500, 32, 0, 0, "", []float64{0}},
		{18, 0, 5, 2, 500, 0, 0, 1, "", nil},
		{18, 1, 15, 6, 500, 0, 0, 0, "", []float64{0}},
		{18, 2, 30, 5, 500, 0, 0, 2, "", nil},
		{18, 3, 50, 5, 500, 0, 0, 0, corebattle.TemporaryReflectMagic, nil},
		{18, 5, 90, 25, 716, 0, 2, 0, "", []float64{0, 0}},
		{18, 7, 120, 100, 770, 486, 2, 0, "", []float64{0, 0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := guardianFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.pos == 7) {
				t.Fatal(entry)
			}
			status := corebattle.StatusPoison
			if id == "job-17-entry-3" {
				status = corebattle.StatusParalyze
			}
			if id == "job-18-entry-2" {
				status = corebattle.StatusSleep
			}
			s.Actors[0].Status = status
			args := []string{"self", "tail"}
			if row.pos == 7 {
				args[0] = "enemy"
			}
			executeGuardian(t, &s, c, d, id, args, row.draws...)
			a, e, fallen := s.Actors[0], s.Actors[1], s.Actors[2]
			if a.HP != row.hp || 900-e.HP != row.damage || a.Temporary != row.tmp || c.HealCalls != row.heals || c.CureCalls != row.cures || a.MP != 80 || a.CMP != 90 {
				t.Fatal(a, e, c)
			}
			if row.cures > 0 && a.Status != "" {
				t.Fatal(a)
			}
			if id == "job-18-entry-1" && (e.Status != corebattle.StatusDanceSeal || c.StatusCalls != 1 || c.AreaStatusCalls != 1) {
				t.Fatal(e, c)
			}
			if row.pos == 7 {
				if fallen.HP != 271 || !s.SkillHistory["party"][id] {
					t.Fatal(fallen, s.SkillHistory)
				}
			} else if fallen.HP != 0 || fallen.Temporary != "" {
				t.Fatal(fallen)
			}
			shifted := id == "job-17-entry-0" || id == "job-17-entry-1" || id == "job-18-entry-0" || id == "job-18-entry-3" || row.pos == 7
			want := args
			if shifted {
				want = args[1:]
			}
			if fmt.Sprint(c.Arguments) != fmt.Sprint(want) {
				t.Fatal(c.Arguments, want)
			}
		})
	}
}
