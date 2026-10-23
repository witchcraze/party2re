package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func dualAgilityFixture() (corebattle.LifetimeState, *corebattle.DualAgilityContext, BattleDefinitions) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 1000, MaxHP: 1000, MP: 80, CMP: 90, Attack: 100, Agility: 40, Gear: &corebattle.BattleGearStats{}}, Hit: 95, Tension: 1},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 1000, MaxHP: 1000, Gear: &corebattle.BattleGearStats{}}, Tension: 1},
		{Participant: corebattle.Participant{ID: "fallen", TeamID: "party", Gear: &corebattle.BattleGearStats{}}, Hit: 99, Tension: 1},
	}}
	c := &corebattle.DualAgilityContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self", "fallen"}, EnemyIDs: []string{"enemy"}}}, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	d := BattleDefinitions{}
	d.RegisterDualAgility(c)
	return s, c, d
}

// Intrinsic vectors from ff97fc5:_skill.cgi:5929–5983,6620–6667.
func TestDualAgilityManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage, calls int
	}{
		{77, 0, 10, 4, 66, 2}, {77, 1, 40, 8, 45, 1},
		{77, 2, 70, 42, 54, 2}, {77, 3, 90, 22, 90, 2}, {77, 4, 130, 37, 135, 3},
		{83, 0, 5, 3, 45, 1}, {83, 1, 15, 9, 0, 0}, {83, 2, 35, 11, 0, 0},
		{83, 3, 60, 24, 114, 3}, {83, 4, 85, 8, 0, 0}, {83, 5, 100, 21, 76, 1},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := dualAgilityFixture()
			// Instant death's immune branch requires no defeat descendant.
			if row.job == 83 && row.pos == 2 {
				s.Actors[1].HP = 1001
			}
			c.Status = &corebattle.StatusContext{EffectContext: c.EffectContext}
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "enemy", "enemy"}}
			before := s.Actors[1].HP
			ops, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			draws := make([]float64, row.calls)
			for i := range draws {
				draws[i] = 0
			}
			if row.pos == 0 { // Ordinary critical gate declines at .5.
				draws = nil
				for range row.calls {
					draws = append(draws, .5, 0)
				}
			}
			if row.job == 77 && row.pos == 2 {
				draws = append([]float64{0}, draws...)
			}
			if row.job == 77 && row.pos == 4 {
				draws = []float64{0, 0, 0, 0, 0, 0}
			}
			rng := &damageGemDraws{t: t, draws: draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
				t.Fatal(err)
			}
			if before-s.Actors[1].HP != row.damage || c.DamageCalls != row.calls || rng.used != len(draws) {
				t.Fatal(row, s.Actors, c.DamageCalls, rng.used)
			}
			if s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 || s.Actors[0].Attack != 100 || s.Actors[0].Agility != 40 {
				t.Fatal(s.Actors[0])
			}
			if row.job == 83 && row.pos == 1 && (s.Actors[0].Hit != 95 || s.Actors[2].Hit != 95) {
				t.Fatal(s.Actors)
			}
			if row.job == 83 && row.pos == 4 && s.Actors[0].Temporary != corebattle.TemporaryRage {
				t.Fatal(s.Actors[0])
			}
		})
	}
}
