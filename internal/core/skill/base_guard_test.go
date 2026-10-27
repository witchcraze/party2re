package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func baseGuardFixture() (corebattle.LifetimeState, *corebattle.BaseGuardContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP, s.Actors[0].Defense, s.Actors[0].Agility = 100, 80, 60
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 60}
	s.Actors[1].Defense = 80
	s.Actors[1].BaseStats.Defense = 80
	s.Actors[2].HP, s.Actors[2].MaxHP, s.Actors[2].Defense = 100, 100, 80
	s.Actors[2].BaseStats.Defense = 80
	c := &corebattle.BaseGuardContext{DamageContext: base.DamageContext, Status: &corebattle.StatusContext{}}
	var d BattleDefinitions
	d.RegisterBaseGuard(c)
	return s, c, d
}

func executeBaseGuard(t *testing.T, s *corebattle.LifetimeState, c *corebattle.BaseGuardContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.CallbackContext {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(id, err, rng.used)
	}
	return f
}

// Independent vectors: ff97fc5:party2/lib/_skill.cgi:1002–1183.
func TestBaseGuardManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage, calls, attack, defense int
		extended                                         bool
		tmp                                              corebattle.TemporaryEffect
		draws                                            []float64
	}{
		{0, 0, 8, 0, 23, 1, 100, 80, false, "", []float64{.5, 0}},
		{0, 1, 9, 0, 23, 1, 100, 80, false, "", []float64{.5, 0}},
		{0, 2, 10, 0, 23, 1, 100, 80, false, "", []float64{.5, 0}},
		{0, 3, 20, 0, 0, 0, 100, 80, false, "", nil},
		{0, 4, 30, 0, 0, 0, 100, 80, false, corebattle.TemporaryDefense, nil},
		{1, 0, 0, 0, 28, 1, 100, 80, true, "", []float64{0}},
		{1, 1, 5, 5, 18, 1, 100, 80, false, "", []float64{0}},
		{1, 2, 8, 0, 0, 0, 100, 80, false, corebattle.TemporaryCovering, nil},
		{1, 3, 25, 8, 0, 0, 200, 80, false, "", nil},
		{1, 4, 50, 5, 81, 1, 100, 80, false, "", []float64{0}},
		{1, 5, 80, 10, 113, 1, 100, 80, false, "", []float64{0, 0}},
		{1, 6, 80, 20, 113, 1, 100, 80, true, "", []float64{0, 0}},
		{2, 0, 0, 0, 12, 1, 100, 80, true, "", []float64{0}},
		{2, 1, 5, 3, 18, 1, 100, 80, false, "", []float64{0}},
		{2, 2, 10, 4, 0, 0, 100, 80, false, "", []float64{0}},
		{2, 3, 20, 5, 0, 0, 100, 80, false, corebattle.TemporaryDeflect, nil},
		{2, 4, 30, 0, 0, 0, 100, 80, false, corebattle.TemporaryCovering, nil},
		{2, 5, 50, 6, 36, 1, 100, 80, false, "", []float64{0}},
		{2, 6, 80, 12, 74, 2, 100, 80, false, "", []float64{0, 0}},
		{2, 7, 100, 21, 23, 1, 100, 80, false, "", []float64{0}},
		{2, 8, 100, 49, 117, 1, 100, 80, true, "", []float64{0}},
		{3, 0, 0, 0, 28, 1, 100, 80, true, "", []float64{0}},
		{3, 1, 1, 0, 0, 0, 100, 80, false, corebattle.TemporaryCovering, nil},
		{3, 2, 5, 2, 0, 0, 100, 112, false, "", nil},
		{3, 3, 15, 5, 68, 1, 100, 80, false, corebattle.TemporaryDouble, []float64{0}},
		{3, 4, 25, 3, 0, 0, 100, 80, false, corebattle.TemporaryStrongDefense, nil},
		{3, 5, 40, 7, 0, 0, 100, 100, false, "", nil},
		{3, 7, 80, 18, 32, 1, 100, 80, false, "", []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := baseGuardFixture()
			before := 1000
			if id == "job-2-entry-2" {
				s.Actors[1].HP, before = 999, 999
			}
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != row.extended {
				t.Fatal(entry)
			}
			target := "enemy"
			if row.tmp == corebattle.TemporaryCovering {
				target = "fallen"
			}
			executeBaseGuard(t, &s, c, d, id, []string{target, "unused"}, row.draws...)
			remaining := 1
			switch id {
			case "job-0-entry-3", "job-0-entry-4", "job-1-entry-3", "job-1-entry-4", "job-2-entry-0", "job-2-entry-3", "job-2-entry-7", "job-3-entry-2", "job-3-entry-4", "job-3-entry-5", "job-3-entry-7":
				remaining = 2
			}
			if len(c.Arguments) != remaining || c.Arguments[len(c.Arguments)-1] != "unused" {
				t.Fatal(id, c.Arguments)
			}
			a := s.Actors[0]
			if before-s.Actors[1].HP != row.damage || c.DamageCalls != row.calls || a.Attack != row.attack || a.Defense != row.defense || a.Temporary != row.tmp || a.MP != 80 || a.CMP != 90 {
				t.Fatal(row, s, c)
			}
			if id == "job-0-entry-3" && (a.Tension != 1.7 || c.TensionCalls != 1) {
				t.Fatal(a, c)
			}
			if id == "job-1-entry-1" && (!c.AdditionalEffect || s.Actors[1].Defense != 68 || c.StatDownCalls != 1) {
				t.Fatal(s, c)
			}
			if id == "job-2-entry-2" && s.Actors[1].Status != corebattle.StatusDofuu {
				t.Fatal(s)
			}
			if row.tmp == corebattle.TemporaryCovering && s.Actors[2].Temporary != corebattle.TemporaryCover {
				t.Fatal(s)
			}
			if id == "job-3-entry-5" && (s.Actors[2].Defense != 100 || c.PartyStatUpCalls != 1) {
				t.Fatal(s, c)
			}
		})
	}
}
