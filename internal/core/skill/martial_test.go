package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func martialFixture() (corebattle.LifetimeState, *corebattle.MartialContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP = 500
	s.Actors[0].Defense = 50
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 50, Agility: 40}
	c := &corebattle.MartialContext{DamageContext: base.DamageContext, Status: &corebattle.StatusContext{}}
	var d BattleDefinitions
	d.RegisterMartial(c)
	return s, c, d
}

func executeMartial(t *testing.T, s *corebattle.LifetimeState, c *corebattle.MartialContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.CallbackContext {
	t.Helper()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &frame, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(id, err, rng.used)
	}
	return frame
}

// Independent intrinsic vectors: ff97fc5:_skill.cgi:1190–1226,2237–2265,2296–2334.
func TestMartialManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage, calls, hp, attack, defense, agility int
		temporary                                                     corebattle.TemporaryEffect
		extended                                                      bool
		draws                                                         []float64
	}{
		{4, 0, 0, 0, 33, 1, 500, 100, 50, 40, "", true, []float64{0}},
		{4, 1, 1, 0, 0, 0, 500, 100, 50, 40, "", false, nil},
		{4, 2, 5, 3, 0, 0, 500, 100, 50, 56, "", false, nil},
		{4, 3, 14, 4, 54, 1, 500, 100, 50, 40, "", false, []float64{0}},
		{4, 4, 25, 3, 0, 0, 500, 100, 50, 40, "", false, []float64{0}},
		{4, 5, 45, 11, 0, 0, 500, 100, 50, 40, "", false, nil},
		{4, 6, 70, 15, 67, 1, 500, 100, 50, 40, "", false, []float64{0}},
		{4, 7, 100, 20, 108, 3, 500, 100, 50, 40, "", false, []float64{0, 0, 0, 0, 0, 0, 0}},
		{4, 8, 100, 10, 0, 0, 500, 140, 70, 40, "", true, nil},
		{23, 0, 10, 5, 32, 1, 500, 100, 50, 40, "", false, []float64{0}},
		{23, 1, 30, 25, 0, 0, 500, 140, 70, 40, "", false, nil},
		{23, 2, 50, 18, 40, 1, 536, 100, 50, 40, "", false, []float64{0, 0}},
		{23, 3, 70, 14, 78, 2, 500, 100, 50, 40, "", false, []float64{0, 0}},
		{23, 4, 130, 35, 126, 1, 500, 100, 50, 40, "", false, []float64{0}},
		{25, 0, 5, 8, 36, 1, 500, 100, 50, 40, "", false, []float64{0}},
		{25, 1, 15, 3, 0, 0, 590, 100, 50, 40, "", false, []float64{0}},
		{25, 2, 30, 15, 90, 1, 500, 100, 50, 40, corebattle.TemporaryDouble, false, []float64{0}},
		{25, 3, 50, 5, 0, 0, 500, 100, 50, 40, corebattle.TemporaryReflectAttack, false, nil},
		{25, 4, 70, 3, 0, 0, 500, 100, 50, 40, corebattle.TemporaryStrongDefense, false, nil},
		{25, 5, 90, 12, 108, 1, 500, 100, 50, 40, "", false, []float64{0}},
		{25, 6, 110, 0, 0, 0, 500, 100, 50, 40, corebattle.TemporaryCovering, false, nil},
		{25, 7, 130, 7, 0, 0, 500, 100, 50, 40, corebattle.TemporaryRevival, false, nil},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := martialFixture()
			s.Actors[0].Hit = 94
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != row.extended {
				t.Fatal(entry)
			}
			before := s.Actors[1].HP
			if row.job == 4 && row.pos == 4 {
				s.Actors[1].HP, before = 999, 999
			}
			if row.job == 4 && row.pos == 5 { // HP>999 is the source death-immunity branch.
				s.Actors[1].HP = 1001
				before = 1001
			}
			executeMartial(t, &s, c, d, id, []string{"enemy", "enemy"}, row.draws...)
			a := s.Actors[0]
			if before-s.Actors[1].HP != row.damage || c.DamageCalls != row.calls || a.HP != row.hp || a.Attack != row.attack || a.Defense != row.defense || a.Agility != row.agility || a.Temporary != row.temporary || a.MP != 80 || a.CMP != 90 {
				t.Fatal(row, s, c)
			}
			if (row.job == 4 && row.pos == 1 || row.job == 25 && row.pos == 1) && a.Hit != 95 {
				t.Fatal(a)
			}
			if row.job == 4 && row.pos == 4 && s.Actors[1].Status != corebattle.StatusDofuu {
				t.Fatal(s.Actors[1])
			}
			if row.job == 4 && row.pos == 8 && (a.Extended != corebattle.ExtendedFlame || a.ExtendedTurns != 6) {
				t.Fatal(a)
			}
			if row.job == 25 && row.pos == 6 && s.Actors[2].Temporary != corebattle.TemporaryCover {
				t.Fatal(s.Actors[2])
			}
		})
	}
}
