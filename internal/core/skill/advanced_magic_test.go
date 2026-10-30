package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func advancedMagicFixture() (corebattle.LifetimeState, *corebattle.AdvancedMagicContext, BattleDefinitions) {
	s, base, _ := hybridBreathFixture()
	s.Actors[0].MaxMP = 200
	s.Actors[1].MP, s.Actors[1].MaxMP = 100, 100
	c := &corebattle.AdvancedMagicContext{DamageContext: base.DamageContext}
	c.Status.Defeat = &corebattle.DefeatContext{
		EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext},
		Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		},
	}
	var d BattleDefinitions
	d.RegisterAdvancedMagic(c)
	return s, c, d
}

func executeAdvancedMagic(t *testing.T, s *corebattle.LifetimeState, c *corebattle.AdvancedMagicContext, d BattleDefinitions, id string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	c.Defeat = func(target string) error {
		c.Status.Defeat.TargetContext = c.TargetContext
		_, err := s.ApplyDefeat(c.Status.Defeat, target, rng)
		return err
	}
	c.Reset = func(target string) error {
		c.Status.Defeat.TargetContext = c.TargetContext
		return s.ApplyEquipmentReset(&c.Status.Defeat.EquipmentResetContext, target, rng)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(id, err)
	}
	if rng.used != len(draws) {
		t.Fatal(id, "draws", rng.used, len(draws))
	}
}

// Source-bound intrinsic vectors: ff97fc5:_skill.cgi:1767–1864,1993–2029.
// Every position asserts an observable result, metadata, arguments and counters.
func TestAdvancedMagicManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, hp, selfMP, enemyHP int
		status                                string
		tmp                                   corebattle.TemporaryEffect
		draws                                 []float64
	}{
		{15, 0, 5, 6, 500, 80, 873, corebattle.StatusDeadlyPoison, "", []float64{0, 0}},
		{15, 1, 10, 5, 500, 80, 869, "", "", []float64{0}},
		{15, 2, 20, 8, 500, 80, 900, corebattle.StatusSleep, "", []float64{0}},
		{15, 3, 40, 7, 500, 80, 900, "", corebattle.TemporaryReflectMagic, nil},
		{15, 4, 60, 3, 500, 99, 900, "", "", []float64{0}},
		{15, 5, 80, 16, 540, 80, 855, "", "", []float64{0, 0, 0}},
		{15, 6, 130, 40, 500, 80, 666, "", "", []float64{0}},
		{15, 7, 130, 64, 500, 80, 450, "", "", []float64{0}},
		{16, 0, 5, 5, 554, 80, 900, "", "", []float64{0}},
		{16, 2, 15, 6, 500, 80, 900, corebattle.StatusMagicSeal, "", []float64{0}},
		{16, 3, 30, 18, 662, 80, 900, "", "", []float64{0}},
		{16, 4, 60, 20, 500, 80, 900, "", corebattle.TemporaryRevival, nil},
		{16, 5, 80, 5, 500, 80, 900, "", corebattle.TemporaryReduceMagic, nil},
		{16, 7, 140, 35, 500, 80, 738, "", "", []float64{0}},
		{16, 8, 140, 46, 1000, 80, 900, "", corebattle.TemporaryNullAttack, []float64{0, 0}},
		{19, 0, 4, 7, 500, 80, 900, "", "", nil},
		{19, 1, 8, 5, 500, 80, 900, "", corebattle.TemporaryReflectMagic, nil},
		{19, 2, 16, 10, 500, 80, 900, corebattle.StatusConfusion, "", []float64{0}},
		{19, 3, 25, 14, 500, 80, 0, "", "", []float64{0}},
		{19, 4, 40, 10, 500, 80, 900, corebattle.StatusMagicSeal, "", []float64{0}},
		{19, 5, 60, 18, 500, 80, 788, "", "", []float64{0}},
		{19, 6, 70, 2, 500, 103, 900, "", "", []float64{0}},
		{19, 7, 110, 32, 500, 80, 0, "", "", []float64{0}},
		{19, 8, 110, 88, 500, 80, 900, "", "", []float64{0, 0, .99, 0, .99, 0, .99, 0, .99}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := advancedMagicFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 15 && row.pos == 7 || row.job == 16 && row.pos == 8 || row.job == 19 && row.pos == 8) {
				t.Fatal(entry)
			}
			target := "enemy"
			if row.tmp != "" || row.job == 16 && (row.pos == 0 || row.pos == 3) {
				target = "self"
			}
			args := []string{target, "tail"}
			executeAdvancedMagic(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if a.HP != row.hp || a.MP != row.selfMP || a.CMP != 90 || a.Temporary != row.tmp || e.HP != row.enemyHP || e.Status != row.status || s.Actors[2].HP != 0 {
				t.Fatal(a, e, c)
			}
			want := args[1:]
			if row.job == 19 && (row.pos == 0 || row.pos == 2 || row.pos == 4 || row.pos == 5 || row.pos >= 7) {
				want = args
			}
			if !reflect.DeepEqual(c.Arguments, want) {
				t.Fatal(c.Arguments, want)
			}
			if row.job == 19 && row.pos == 0 && (e.Defense != 60 || c.StatDownCalls != 1 || c.PartyStatDownCalls != 1) {
				t.Fatal(e, c)
			}
			if id == "job-15-entry-0" && (!c.AdditionalEffect || c.DamageCalls != 1 || c.Status.StatusCalls != 1) || id == "job-16-entry-8" && (c.HealCalls != 2 || c.PartyHealCalls != 1) || id == "job-19-entry-8" && c.Status.DeathCalls != 4 {
				t.Fatal(c)
			}
		})
	}
}

// Keep the real source subset and the two separately bound positions distinct.
func TestAdvancedMagicRegistrationPreservesOtherOwners(t *testing.T) {
	for _, first := range []bool{false, true} {
		s, c, _ := advancedMagicFixture()
		var d BattleDefinitions
		bindOthers := func() {
			d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
			d.RegisterOrdinaryTools(&corebattle.OrdinaryToolContext{DamageContext: c.DamageContext})
		}
		if first {
			bindOthers()
		}
		d.RegisterAdvancedMagic(c)
		if !first {
			bindOthers()
		}
		for job, count := range map[int]int{15: 8, 16: 9, 19: 9} {
			if len(d.Invocations.JobTables[job].Variants[0]) != count {
				t.Fatal(job)
			}
		}
		for _, id := range []string{"job-16-entry-1", "job-16-entry-6"} {
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen"}}
			if _, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t}); err != nil {
				t.Fatal(id, err)
			}
		}
	}
}
