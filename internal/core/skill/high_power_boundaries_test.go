package skill

import (
	"fmt"
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestHighPowerDamageReturnGuards(t *testing.T) {
	for _, id := range []string{"job-47-entry-0", "job-47-entry-1", "job-47-entry-2", "job-49-entry-2", "job-52-entry-0"} {
		for _, guard := range []string{"one-hit", "miss", "sealed", "fallen"} {
			t.Run(id+"/"+guard, func(t *testing.T) {
				s, c, d := highPowerFixture()
				var draws []float64
				switch guard {
				case "one-hit":
					s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
					c.Defeat = nil
				case "miss":
					c.GuaranteedHit = false
					s.Actors[0].Hit = 0
					draws = []float64{.9}
				case "sealed":
					s.Actors[0].Status = corebattle.StatusPhysicalSeal
					draws = []float64{.9}
				case "fallen":
					s.Actors[1].HP = 0
					c.Defeat = nil
				}
				before := s.Actors[1].HP
				executeHighPower(t, &s, c, d, []string{id}, []string{"enemy"}, draws...)
				if s.Actors[1].HP != before || s.Actors[0].Attack != 100 || s.Actors[0].HP != 500 || c.End.ImmediateRiskCalls != 0 {
					t.Fatal(s, c)
				}
			})
		}
	}
}

func TestHighPowerWoundsAndWeaponCaps(t *testing.T) {
	for _, hp := range []int{0, 1, 610, 611, 999, 1000} {
		s, c, d := highPowerFixture()
		s.Actors[1].HP = hp
		var draws []float64
		want := hp
		if hp > 0 {
			draws = []float64{0}
			want = max(0, hp-int(float64(min(400, 1010-hp))*.9))
		}
		executeHighPower(t, &s, c, d, []string{"job-47-entry-3"}, []string{"enemy", "tail"}, draws...)
		if s.Actors[1].HP != want || len(c.Arguments) != 1 {
			t.Fatal(hp, s, c)
		}
	}
	for _, row := range []struct {
		hp, base, attack, want int
		over                   bool
	}{
		{0, 100, 100, 100, false}, {1, 100, 100, 100, false}, {999, 100, 900, 999, false}, {1000, 100, 900, 999, false},
		{1000, 400, 1100, 1200, true}, {1000, 700, 1900, 1998, true}, {1000, 100, 1200, 999, true},
	} {
		s, c, d := highPowerFixture()
		a := &s.Actors[0]
		a.HP, a.BaseStats.Attack, a.Attack, a.OverLevel = row.hp, row.base, row.attack, row.over
		a.Hit, a.Temporary = 1, corebattle.TemporaryDefense
		executeHighPower(t, &s, c, d, []string{"job-47-entry-5"}, nil)
		if s.Actors[0].Attack != row.want || s.Actors[0].Hit != 95 || s.Actors[0].Temporary != corebattle.TemporaryRegeneration {
			t.Fatal(row, s)
		}
	}
}

func TestHighPowerLifeCollapseBoundaries(t *testing.T) {
	for _, hp := range []int{0, 1, 999, 1000} {
		for _, maxHP := range []int{999, 1000} {
			for _, defense := range []int{999, 1000} {
				for _, draw := range []float64{math.Nextafter(1.0/3, 0), 1.0 / 3} {
					s, c, d := highPowerFixture()
					a := &s.Actors[1]
					a.HP, a.MaxHP, a.BaseStats.Defense = hp, maxHP, defense
					want := hp
					var draws []float64
					if hp > 0 && maxHP == 999 && defense == 999 {
						draws = []float64{draw}
						if draw < 1.0/3 {
							want = 1
						}
					}
					executeHighPower(t, &s, c, d, []string{"job-48-entry-3"}, []string{"enemy"}, draws...)
					if s.Actors[1].HP != want {
						t.Fatal(hp, maxHP, defense, draw, s)
					}
				}
			}
		}
	}
}

func TestHighPowerWoundsAboveMaximum(t *testing.T) {
	for _, hp := range []int{1010, 1011} {
		s, c, d := highPowerFixture()
		s.Actors[1].HP = hp
		var draws []float64
		want := hp
		if hp == 1011 {
			draws = []float64{0, 0}
			want--
		}
		executeHighPower(t, &s, c, d, []string{"job-47-entry-3"}, []string{"enemy"}, draws...)
		if s.Actors[1].HP != want {
			t.Fatal(hp, s)
		}
	}
}

func TestHighPowerStatusAndStatGuards(t *testing.T) {
	for pos, chance := range []float64{.4, .3} {
		s, c, d := highPowerFixture()
		s.Actors[1].HP = 1000
		id := fmt.Sprintf("job-48-entry-%d", pos)
		executeHighPower(t, &s, c, d, []string{id}, nil, 0, chance)
		if s.Actors[1].HP >= 1000 || s.Actors[1].Status != "" || c.StatusCalls != 1 {
			t.Fatal(s, c)
		}
		// A kill skips the status tail for that target.
		s, c, d = highPowerFixture()
		s.Actors[1].HP = 1
		executeHighPower(t, &s, c, d, []string{id}, nil, 0)
		if s.Actors[1].HP != 0 || c.StatusCalls != 0 {
			t.Fatal(s, c)
		}
	}
	for _, guard := range []string{"one-hit", "dead", "hit-floor", "immune"} {
		s, c, d := highPowerFixture()
		var draws []float64
		switch guard {
		case "one-hit":
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
			c.Defeat = nil
		case "dead":
			s.Actors[1].HP = 1
			draws = []float64{0}
		case "hit-floor":
			s.Actors[1].Hit = 50
			draws = []float64{0}
		case "immune":
			s.Actors[1].ItemDefinitionIDs = []string{"item-197"}
			draws = []float64{0}
		}
		before := s.Actors[1].Hit
		executeHighPower(t, &s, c, d, []string{"job-48-entry-2"}, []string{"enemy"}, draws...)
		if s.Actors[1].Hit != before {
			t.Fatal(guard, s)
		}
	}
	for _, id := range []string{"job-49-entry-1", "job-52-entry-1"} {
		s, c, d := highPowerFixture()
		s.Actors[0].Status = corebattle.StatusSabaku
		executeHighPower(t, &s, c, d, []string{id}, nil)
		if s.Actors[0].Attack != 100 || s.Actors[0].Defense != 80 || s.Actors[0].Status != corebattle.StatusSabaku {
			t.Fatal(s)
		}
		if id == "job-49-entry-1" && s.Actors[0].Temporary != corebattle.TemporaryReduceAttack {
			t.Fatal(s)
		}
	}
	s, c, d := highPowerFixture()
	executeHighPower(t, &s, c, d, []string{"job-49-entry-0"}, []string{"self"})
	if c.CureCalls != 0 {
		t.Fatal(c)
	}
	for _, df := range []int{0, 1, 999, 1000} {
		s, c, d = highPowerFixture()
		e := &s.Actors[1]
		e.BaseStats.Defense, e.Defense, e.Tension, e.Temporary = df, 2000, 5, corebattle.TemporaryDouble
		executeHighPower(t, &s, c, d, []string{"job-52-entry-2"}, []string{"enemy"})
		want := df
		if df <= 999 {
			want -= int(float64(df) * .1)
		}
		if s.Actors[1].Defense != want || s.Actors[1].Tension != 1 || s.Actors[1].Temporary != "" {
			t.Fatal(df, s)
		}
	}
	s, c, d = highPowerFixture()
	s.Actors[0].Status = corebattle.StatusSabaku
	executeHighPower(t, &s, c, d, []string{"job-52-entry-3"}, nil)
	if s.Actors[0].Tension != 1 || s.Actors[0].Temporary != corebattle.TemporaryDouble || c.TensionCalls != 2 {
		t.Fatal(s, c)
	}
}

func TestHighPowerFourHitsStopOnDeathAndPostHitVulnerability(t *testing.T) {
	for _, id := range []string{"job-47-entry-4", "job-52-entry-4"} {
		s, c, d := highPowerFixture()
		s.Actors[0].HP = 1
		s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
		c.Defeat = func(id string) error {
			if id != "self" {
				t.Fatal(id)
			}
			return nil
		}
		draws := []float64{0}
		if id == "job-47-entry-4" {
			draws = []float64{0, 0}
		}
		executeHighPower(t, &s, c, d, []string{id}, nil, draws...)
		if s.Actors[0].HP != 0 || c.DamageCalls != 1 {
			t.Fatal(s, c)
		}
		if id == "job-52-entry-4" && s.Actors[0].Temporary != corebattle.TemporaryDouble {
			t.Fatal(s)
		}
	}
}

func TestHighPowerAreaNoEffectAndIndependentSealChecks(t *testing.T) {
	for _, id := range []string{"job-48-entry-4", "job-52-entry-4"} {
		s, c, d := highPowerFixture()
		s.Actors = append(s.Actors[:1], s.Actors[2:]...)
		c.EnemyIDs = nil
		c.Defeat = nil
		executeHighPower(t, &s, c, d, []string{id}, []string{"ignored"})
		if c.AreaDamageCalls != 1 || c.DamageCalls != 0 || len(c.Arguments) != 1 {
			t.Fatal(s, c)
		}
		if id == "job-52-entry-4" && s.Actors[0].Temporary != corebattle.TemporaryDouble {
			t.Fatal(s)
		}
	}
	s, c, d := highPowerFixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeHighPower(t, &s, c, d, []string{"job-48-entry-0"}, nil, .9, 0, 0)
	if s.Actors[1].HP != 900 || s.Actors[1].Status != corebattle.StatusDeadlyPoison || c.AreaDamageCalls != 0 || c.AreaStatusCalls != 1 || s.Actors[0].Status != "" {
		t.Fatal(s, c)
	}
}
