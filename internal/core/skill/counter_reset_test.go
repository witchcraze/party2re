package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func counterResetFixture() (corebattle.LifetimeState, *corebattle.CounterResetContext, BattleDefinitions) {
	s, base, _ := hybridBreathFixture()
	s.Actors[1].Attack, s.Actors[1].Defense = 200, 160
	s.Actors[1].BaseStats = corebattle.BattleBaseStats{Attack: 50, Defense: 40, Agility: 20}
	c := &corebattle.CounterResetContext{DamageContext: base.DamageContext,
		StatusDefeat: &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: base.TargetContext}, Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		}},
	}
	var d BattleDefinitions
	d.RegisterCounterReset(c)
	return s, c, d
}

func executeCounterReset(t *testing.T, s *corebattle.LifetimeState, c *corebattle.CounterResetContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Complete intrinsic vectors: ff97fc5:_skill.cgi:2871–3013; helpers retain their
// own formulas, while these assertions certify each concrete binding and tail.
func TestCounterResetManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, enemyHP, selfHP int
		temporary                         corebattle.TemporaryEffect
		status                            string
		draws                             []float64
	}{
		{35, 0, 10, 4, 900, 500, corebattle.TemporaryDeflect, "", nil},
		{35, 2, 60, 14, 0, 500, "", "", []float64{0}},
		{35, 3, 90, 40, 738, 500, "", "", []float64{0}},
		{35, 4, 120, 25, 900, 770, "", "", []float64{0}},
		{35, 5, 150, 6, 900, 500, corebattle.TemporaryNullMagic, "", nil},
		{35, 6, 180, 70, 765, 500, "", corebattle.StatusParalyze, []float64{0, 0}},
		{36, 0, 10, 5, 900, 500, corebattle.TemporaryReflectDance, "", nil},
		{36, 1, 20, 5, 900, 500, corebattle.TemporaryReflectBreath, "", nil},
		{36, 2, 40, 5, 900, 500, corebattle.TemporaryReflectMagic, "", nil},
		{36, 3, 60, 5, 900, 500, corebattle.TemporaryReflectAttack, "", nil},
		{37, 0, 5, 10, 900, 500, "", corebattle.StatusMagicSeal, []float64{0}},
		{37, 1, 10, 1, 900, 500, corebattle.TemporaryAbsorbMagic, "", nil},
		{37, 2, 15, 6, 900, 500, "", corebattle.StatusDanceSeal, []float64{0}},
		{37, 3, 25, 6, 900, 500, corebattle.TemporaryReduceMagic, "", nil},
		{37, 4, 40, 5, 900, 500, corebattle.TemporaryReflectMagic, "", nil},
		{37, 5, 60, 7, 900, 500, "", corebattle.StatusPhysicalSeal, []float64{0}},
		{37, 6, 80, 25, 900, 770, "", "", []float64{0}},
		{37, 7, 100, 15, 900, 500, "", "", nil},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := counterResetFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if row.job == 37 && row.pos == 4 {
				args[0] = "self"
			}
			if row.job == 37 && row.pos == 7 {
				s.Actors[1].Temporary, s.Actors[1].Tension = corebattle.TemporaryReflectMagic, 4
			}
			executeCounterReset(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if a.MP != 80 || a.CMP != 90 || a.HP != row.selfHP || e.HP != row.enemyHP || a.Temporary != row.temporary || e.Status != row.status {
				t.Fatal(a, e)
			}
			wantArgs := args
			if id == "job-35-entry-2" || row.job == 37 && (row.pos == 4 || row.pos == 5 || row.pos == 7) {
				wantArgs = args[1:]
			}
			if !reflect.DeepEqual(c.Arguments, wantArgs) {
				t.Fatal(c.Arguments, wantArgs)
			}
			if row.job == 37 && row.pos == 7 && (e.Attack != 50 || e.Defense != 40 || e.Tension != 1 || e.Temporary != "" || c.StatDownCalls != 0) {
				t.Fatal(e, c)
			}
			if id == "job-35-entry-6" && (!c.AdditionalEffect || c.DamageCalls != 1 || c.AreaDamageCalls != 1 || c.StatusCalls != 1 || c.AreaStatusCalls != 1) {
				t.Fatal(c)
			}
			if id == "job-35-entry-2" && c.DeathCalls != 1 {
				t.Fatal(c)
			}
		})
	}
}
