package battle

import (
	"fmt"
	"testing"
)

func callingSpecialFixture(t *testing.T, job, form int) (LifetimeState, RosterContext, string) {
	t.Helper()
	s, c := rosterFixture()
	id := addRosterHelper(t, &s, &c, job)
	admitRosterDamage(&s, &c)
	c.Death = nativeDefeatContext(c.TargetContext)
	s.actor("player").HP, s.actor("player").MaxHP = 100, 1000
	s.actor("player").MP = 10
	s.actor("player").Attack = 7
	s.actor("enemy").HP, s.actor("enemy").MaxHP = 500, 500
	s.actor("enemy").Attack, s.actor("enemy").MP, s.actor("enemy").MaxMP = 100, 100, 100
	h := s.actor(id)
	h.SummonForm, h.HP, h.MaxHP, h.Attack, h.Agility = form, 50, 500, 100, 100
	h.Icon = fmt.Sprintf("assets/images/placeholders/battle/helpers.svg#calling-%d-%d", job, form)
	h.BaseStats.Attack = 100
	return s, c, id
}

// Independent intrinsic vectors at ff97fc5:_skill.cgi:4290–4306,4384–4402,
// 4491–4505,4636–4656,4862–4883,4982–5001,5397–5411,6384–6419,6958–6982.
func TestCallingSpecialBranches(t *testing.T) {
	for _, row := range []struct {
		name                                        string
		job, form                                   int
		target                                      string
		draws                                       []float64
		enemyHP, selfHP, selfMP, helperHP, helperAT int
		status                                      string
	}{
		{"61-death-equality", 61, 0, "enemy", []float64{0, .22}, 0, 100, 10, 50, 100, ""},
		{"61-damage-poison", 61, 0, "enemy", []float64{.5, 0, 0, 0}, 392, 100, 10, 50, 100, StatusDeadlyPoison},
		{"61-breath-poison", 61, 0, "enemy", []float64{.5, .5, 0}, 500, 100, 10, 50, 100, StatusDeadlyPoison},
		{"62-hp-return", 62, 0, "enemy", []float64{0, 0, 0}, 410, 100, 10, 131, 100, ""},
		{"62-mp-return", 62, 0, "enemy", []float64{.5, 0, 0}, 500, 100, 38, 50, 100, ""},
		{"62-song", 62, 0, "enemy", []float64{.5, .5, 0}, 500, 100, 10, 50, 100, StatusConfusion},
		{"63-poison", 63, 0, "enemy", []float64{0, 0}, 500, 100, 10, 50, 100, StatusDeadlyPoison},
		{"63-paralyze", 63, 0, "enemy", []float64{.5, 0, 0}, 500, 100, 10, 50, 100, StatusParalyze},
		{"63-sleep-equality", 63, 0, "enemy", []float64{.5, .5, .5}, 500, 100, 10, 50, 100, ""},
		{"64-possession", 64, 0, "enemy", []float64{0, 0, .5, 0, 0}, 455, 100, 10, 50, 100, ""},
		{"64-seal", 64, 0, "enemy", []float64{.5, 0, 0}, 500, 100, 10, 50, 100, StatusPhysicalSeal},
		{"64-stun", 64, 0, "enemy", []float64{.5, .5, 0}, 500, 100, 10, 50, 100, StatusDofuu},
		{"66-area-death", 66, 0, "enemy", []float64{0, 0}, 0, 100, 10, 50, 100, ""},
		{"66-single-death", 66, 0, "enemy", []float64{.5, 0, 0}, 0, 100, 10, 50, 100, ""},
		{"66-random-seal", 66, 0, "enemy", []float64{.5, .5, 0, 0}, 500, 100, 10, 50, 100, StatusPhysicalSeal},
		{"66-random-stun", 66, 0, "enemy", []float64{.5, .5, .5, 0}, 500, 100, 10, 50, 100, StatusDofuu},
		{"67-form0", 67, 0, "enemy", []float64{0}, 500, 100, 10, 50, 100, StatusDeadlyPoison},
		{"67-form1", 67, 1, "enemy", []float64{0}, 302, 100, 10, 50, 100, ""},
		{"67-form2", 67, 2, "enemy", []float64{0, 0}, 365, 100, 10, 50, 100, StatusParalyze},
		{"71-kick", 71, 0, "enemy", []float64{0, 0}, 437, 100, 10, 50, 100, ""},
		{"71-heal", 71, 0, "player", []float64{1.0 / 3, 0, 0}, 500, 235, 10, 50, 100, ""},
		{"71-dance", 71, 0, "enemy", []float64{1.0 / 3, .5, 0}, 500, 100, 10, 50, 100, StatusDofuu},
		{"80-form0-drain", 80, 0, "enemy", []float64{0, 0, 0}, 437, 100, 21, 50, 100, ""},
		{"80-form2-area-boundary", 80, 2, "enemy", []float64{1.0 / 7, 0}, 437, 100, 10, 50, 100, ""},
		{"80-form1-recover", 80, 1, "enemy", []float64{0, 0, 0, 0, 0}, 500, 370, 100, 320, 100, ""},
		{"80-form3-random-boundary", 80, 3, "enemy", []float64{1.0 / 6}, 500, 100, 10, 50, 100, ""},
		{"87-bite", 87, 2, "enemy", []float64{0, 0}, 410, 100, 10, 50, 100, ""},
		{"87-squeeze", 87, 2, "enemy", []float64{1.0 / 3, 0, 0}, 446, 100, 10, 50, 100, ""},
		{"87-buff-stun", 87, 2, "enemy", []float64{1.0 / 3, .5, 0}, 500, 100, 10, 50, 200, StatusDofuu},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, helper := callingSpecialFixture(t, row.job, row.form)
			id := fmt.Sprintf("job-%d-special", row.job)
			if err := s.ValidateRoster(&c, id, row.target, &targetRNG{t: t}); err != nil {
				t.Fatal(err)
			}
			rng := &targetRNG{t: t, draws: row.draws}
			r, err := s.ApplyRoster(&c, id, row.target, rng)
			if err != nil {
				t.Fatal(err)
			}
			rng.done()
			a, e, h := s.actor("player"), s.actor("enemy"), s.actor(helper)
			if e.HP != row.enemyHP || a.HP != row.selfHP || a.MP != row.selfMP || h.HP != row.helperHP || h.Attack != row.helperAT || e.Status != row.status {
				t.Fatal(a, e, h, c)
			}
			if row.name == "80-form3-random-boundary" && r.Delegation.Kind != "random" {
				t.Fatal(r)
			}
			if row.name == "62-mp-return" && e.MP != 70 {
				t.Fatal(e)
			}
			if c.ActorID != "player" {
				t.Fatal(c)
			}
		})
	}
}
