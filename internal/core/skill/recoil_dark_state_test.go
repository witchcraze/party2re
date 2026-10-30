package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func recoilDarkFixture() (corebattle.LifetimeState, *corebattle.RecoilDarkStateContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	a, e := &s.Actors[0], &s.Actors[1]
	a.HP, a.Defense, a.MaxMP = 500, 81, 200
	a.BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 51, Agility: 40}
	e.HP, e.MP, e.MaxMP, e.Hit = 900, 100, 200, 95
	c := &corebattle.RecoilDarkStateContext{DamageContext: base.DamageContext}
	d := BattleDefinitions{}
	d.RegisterRecoilDarkState(c)
	return s, c, d
}

func executeRecoilDark(t *testing.T, s *corebattle.LifetimeState, c *corebattle.RecoilDarkStateContext, d BattleDefinitions, ids, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(ids, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(ids, err)
	}
	if rng.used != len(draws) {
		t.Fatal(ids, rng.used, len(draws))
	}
}

// Independent intrinsic vectors: ff97fc5:_skill.cgi:2074–2187,3018–3069.
func TestRecoilDarkManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage, hp int
		draws                        []float64
	}{
		{21, 0, 10, 4, 58, 495, []float64{0}},
		{21, 1, 20, 5, 0, 500, nil},
		{21, 2, 40, 12, 0, 500, []float64{0}},
		{21, 3, 60, 5, 90, 500, []float64{0}},
		{21, 4, 80, 6, 72, 496, []float64{0}},
		{21, 5, 110, 9, 270, 500, []float64{.25, 0, 0}},
		{21, 6, 110, 20, 0, 500, nil},
		{22, 0, 10, 5, 67, 449, []float64{0}},
		{22, 1, 20, 9, 0, 500, nil},
		{22, 2, 40, 16, 0, 500, []float64{0}},
		{22, 3, 70, 10, 54, 500, []float64{0}},
		{22, 4, 140, 40, 112, 399, []float64{0}},
		{38, 0, 10, 12, 229, 706, []float64{0, 0}},
		{38, 1, 20, 3, 0, 500, []float64{0}},
		{38, 2, 30, 6, 0, 500, nil},
		{38, 3, 60, 9, 0, 500, nil},
		{38, 4, 90, 9, 0, 500, []float64{0}},
		{38, 5, 130, 37, 270, 770, []float64{0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := recoilDarkFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (id == "job-21-entry-6") {
				t.Fatal(entry)
			}
			executeRecoilDark(t, &s, c, d, []string{id}, []string{"enemy", "tail"}, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			mp := 80
			if id == "job-38-entry-1" {
				mp = 99
				if e.MP != 80 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			}
			if e.HP != 900-row.damage || a.HP != row.hp || a.MP != mp || a.CMP != 90 {
				t.Fatal(a, e)
			}
			switch id {
			case "job-21-entry-1":
				if a.Temporary != corebattle.TemporaryDeflect {
					t.Fatal(a)
				}
			case "job-21-entry-3":
				if a.Temporary != corebattle.TemporaryDouble {
					t.Fatal(a)
				}
			case "job-21-entry-5":
				if c.DamageCalls != 0 || c.End.ImmediateRiskCalls != 0 || a.Tension != 1 {
					t.Fatal(c, a)
				}
			case "job-21-entry-6":
				if a.Status != corebattle.StatusConfusion || a.Tension != 3 || a.Extended != corebattle.ExtendedFrenzy || a.ExtendedTurns != 5 || c.TensionCalls != 2 {
					t.Fatal(a, c)
				}
			case "job-22-entry-1", "job-38-entry-3":
				if a.Defense != 41 || a.Attack != 200 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-22-entry-3":
				if e.Hit != 86 || !c.AdditionalEffect || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-38-entry-0", "job-38-entry-5":
				if c.DamageCalls != 1 || c.HealCalls != 1 {
					t.Fatal(c)
				}
			case "job-38-entry-2":
				if a.Temporary != corebattle.TemporaryNullMagic {
					t.Fatal(a)
				}
			case "job-21-entry-2", "job-22-entry-2", "job-38-entry-4":
				symbol := map[int]string{21: corebattle.StatusDofuu, 22: corebattle.StatusConfusion, 38: corebattle.StatusSleep}[row.job]
				if e.Status != symbol || c.StatusCalls != 1 || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			}
			shifted := row.damage > 0 && id != "job-21-entry-5" || id == "job-38-entry-1"
			arguments := 2
			if shifted {
				arguments = 1
			}
			if len(c.Arguments) != arguments {
				t.Fatal(c.Arguments)
			}
		})
	}
}

func TestRecoilDarkGuardBoundaries(t *testing.T) {
	for _, id := range []string{"job-21-entry-0", "job-21-entry-3", "job-21-entry-4", "job-22-entry-0", "job-22-entry-3", "job-22-entry-4", "job-38-entry-0", "job-38-entry-5"} {
		t.Run(id+"/zero-return", func(t *testing.T) {
			s, c, d := recoilDarkFixture()
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
			c.Defeat = nil
			executeRecoilDark(t, &s, c, d, []string{id}, []string{"enemy"})
			if s.Actors[0].HP != 500 || s.Actors[1].HP != 900 || s.Actors[1].Temporary != "" || c.HealCalls != 0 || c.StatDownCalls != 0 || c.End.ImmediateRiskCalls != 0 {
				t.Fatal(s, c)
			}
			if id == "job-21-entry-3" && s.Actors[0].Temporary != corebattle.TemporaryDouble {
				t.Fatal(s)
			}
		})
	}
	for _, row := range []struct {
		id     string
		chance float64
	}{{"job-21-entry-2", .6}, {"job-22-entry-2", .5}, {"job-38-entry-4", .35}} {
		s, c, d := recoilDarkFixture()
		executeRecoilDark(t, &s, c, d, []string{row.id}, nil, row.chance)
		if s.Actors[1].Status != "" || c.StatusCalls != 1 {
			t.Fatal(row.id, s, c)
		}
	}
	for _, id := range []string{"job-22-entry-1", "job-38-entry-3"} {
		for _, bound := range []bool{false, true} {
			s, c, d := recoilDarkFixture()
			s.Actors[0].Attack = 250
			if bound {
				s.Actors[0].Attack, s.Actors[0].Status = 100, corebattle.StatusSabaku
			}
			executeRecoilDark(t, &s, c, d, []string{id}, nil)
			if s.Actors[0].Defense != 41 || s.Actors[0].Attack != map[bool]int{true: 100, false: 250}[bound] {
				t.Fatal(id, s)
			}
		}
	}
	for _, cure := range []bool{false, true} {
		s, c, d := recoilDarkFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		draw := .25
		if cure {
			draw = 0
		}
		executeRecoilDark(t, &s, c, d, []string{"job-38-entry-2"}, nil, draw)
		if (s.Actors[0].Temporary == corebattle.TemporaryNullMagic) != cure || (s.Actors[0].Status == "") != cure {
			t.Fatal(s)
		}
	}
	s, c, d := recoilDarkFixture()
	s.Actors[0].HP = 0
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-1"}, nil)
	if s.Actors[0].Temporary != corebattle.TemporaryDeflect {
		t.Fatal(s)
	}
	s, c, d = recoilDarkFixture()
	s.Actors[0].Status, s.Actors[0].Tension = corebattle.StatusSabaku, 5
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-6"}, nil)
	if s.Actors[0].Tension != 5 || s.Actors[0].Status != corebattle.StatusConfusion || c.TensionCalls != 2 {
		t.Fatal(s, c)
	}
}
