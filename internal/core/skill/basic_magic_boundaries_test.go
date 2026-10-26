package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBasicMagicEveryPositionSealGuard(t *testing.T) {
	_, _, catalog := basicMagicFixture()
	if len(catalog.Invocations.Entries) != 16 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := basicMagicFixture()
			s.Actors[0].Status = corebattle.StatusMagicSeal
			draws := []float64{.9}
			if id == "job-5-entry-4" || id == "job-5-entry-6" || id == "job-5-entry-9" {
				draws = append([]float64{.999999}, draws...)
			}
			executeBasicMagic(t, &s, c, d, id, []string{"self", "tail"}, draws...)
			if s.Actors[0].HP != 500 || s.Actors[0].MP != 80 || s.Actors[0].Status != corebattle.StatusMagicSeal || s.Actors[1].HP != 900 || c.DamageCalls != 0 || c.HealCalls != 0 || c.StatUpCalls != 0 || c.StatDownCalls != 0 || c.CureCalls != 0 || c.StatusCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestBasicMagicContinuousAreaPowerAndOrder(t *testing.T) {
	for _, row := range []struct {
		id     string
		draw   float64
		damage [2]int
	}{
		{"job-5-entry-4", 0, [2]int{9, 7}},
		{"job-5-entry-4", .5, [2]int{20, 17}},
		{"job-5-entry-4", .999999, [2]int{31, 26}},
		{"job-5-entry-6", 0, [2]int{22, 19}},
		{"job-5-entry-6", .999999, [2]int{58, 49}},
		{"job-5-entry-9", 0, [2]int{72, 61}},
		{"job-5-entry-9", .999999, [2]int{161, 137}},
	} {
		t.Run(fmt.Sprintf("%s/%f", row.id, row.draw), func(t *testing.T) {
			s, c, d := basicMagicFixture()
			other := s.Actors[1]
			other.ID = "other"
			s.Actors = append(s.Actors, other)
			c.EnemyIDs = append(c.EnemyIDs, other.ID)
			executeBasicMagic(t, &s, c, d, row.id, []string{"ignored", "tail"}, row.draw, 0, 0)
			if s.Actors[1].HP != 900-row.damage[0] || s.Actors[3].HP != 900-row.damage[1] || c.DamageCalls != 2 || c.AreaDamageCalls != 1 || len(c.Arguments) != 2 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestBasicMagicCurrentMPAndReflection(t *testing.T) {
	for _, id := range []string{"job-5-entry-0", "job-6-entry-0"} {
		for _, row := range []struct{ mp, damage int }{{0, 0}, {1, 1}, {1000, 162}} {
			for _, reflected := range []bool{false, true} {
				s, c, d := basicMagicFixture()
				s.Actors[0].MP = row.mp
				if reflected {
					s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
				}
				var draws []float64
				if row.mp > 0 {
					draws = []float64{0}
				}
				if row.mp == 1 {
					draws = append(draws, 0)
				}
				executeBasicMagic(t, &s, c, d, id, []string{"enemy", "tail"}, draws...)
				wantSelf, wantEnemy := 500, 900-row.damage
				if reflected {
					wantSelf, wantEnemy = 500-row.damage, 900
				}
				if s.Actors[0].MP != row.mp || s.Actors[0].HP != wantSelf || s.Actors[1].HP != wantEnemy || len(c.Arguments) != 1 {
					t.Fatal(id, row, reflected, s, c)
				}
			}
		}
	}
}

func TestBasicMagicHealingAndPoisonTargets(t *testing.T) {
	for _, id := range []string{"job-5-entry-5", "job-5-entry-8"} {
		for _, hp := range []int{0, 1, 1000} {
			s, c, d := basicMagicFixture()
			s.Actors[2].HP, s.Actors[2].MaxHP = hp, 1000
			executeBasicMagic(t, &s, c, d, id, []string{"fallen", "tail"}, .999999)
			wantSelf, wantAlly, amount := 500, hp, 107
			if id == "job-5-entry-8" {
				amount = 1198
			}
			if hp == 0 {
				wantSelf = min(1000, 500+amount)
			} else {
				wantAlly = min(1000, hp+amount)
			}
			if s.Actors[0].HP != wantSelf || s.Actors[2].HP != wantAlly || c.HealCalls != 1 {
				t.Fatal(id, hp, s, c)
			}
		}
	}
	for _, status := range []string{corebattle.StatusPoison, corebattle.StatusDeadlyPoison, corebattle.StatusVirulentPoison, corebattle.StatusSleep, ""} {
		for _, hp := range []int{0, 1} {
			s, c, d := basicMagicFixture()
			s.Actors[0].Status = corebattle.StatusPoison
			s.Actors[2].HP, s.Actors[2].Status = hp, status
			executeBasicMagic(t, &s, c, d, "job-5-entry-2", []string{"fallen"})
			wantSelf, wantAlly := corebattle.StatusPoison, status
			if hp == 0 {
				wantSelf = ""
			} else if status != corebattle.StatusSleep {
				wantAlly = ""
			}
			if s.Actors[0].Status != wantSelf || s.Actors[2].Status != wantAlly || c.CureCalls != 1 {
				t.Fatal(status, hp, s, c)
			}
		}
	}
}

func TestBasicMagicStatAndSleepBoundaries(t *testing.T) {
	for _, row := range []struct{ defense, want int }{{199, 200}, {200, 200}} {
		s, c, d := basicMagicFixture()
		s.Actors[0].Defense = row.defense
		executeBasicMagic(t, &s, c, d, "job-5-entry-1", []string{"self"})
		if s.Actors[0].Defense != row.want || c.StatUpCalls != 1 {
			t.Fatal(s, c)
		}
	}
	for _, row := range []struct{ defense, want int }{{16, 16}, {17, 16}} {
		s, c, d := basicMagicFixture()
		s.Actors[1].Defense = row.defense
		executeBasicMagic(t, &s, c, d, "job-6-entry-2", []string{"enemy"})
		if s.Actors[1].Defense != row.want {
			t.Fatal(s, c)
		}
	}
	for _, row := range []struct{ hit, want int }{{50, 50}, {51, 50}, {95, 76}} {
		s, c, d := basicMagicFixture()
		s.Actors[1].Hit = row.hit
		executeBasicMagic(t, &s, c, d, "job-6-entry-4", nil)
		if s.Actors[1].Hit != row.want || c.PartyStatDownCalls != 1 {
			t.Fatal(s, c)
		}
	}
	for _, row := range []struct {
		draw float64
		want string
	}{{.649999, corebattle.StatusSleep}, {.65, ""}, {.999999, ""}} {
		s, c, d := basicMagicFixture()
		executeBasicMagic(t, &s, c, d, "job-6-entry-6", []string{"enemy"}, row.draw)
		if s.Actors[1].Status != row.want || c.StatusCalls != 1 {
			t.Fatal(s, c)
		}
	}
}

func TestBasicMagicAreaStopsAfterReflectedCasterDeath(t *testing.T) {
	for _, id := range []string{"job-5-entry-4", "job-5-entry-6", "job-5-entry-9", "job-6-entry-7"} {
		s, c, d := basicMagicFixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, other.ID)
		s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectMagic
		draws := []float64{0}
		if id != "job-6-entry-7" {
			draws = append(draws, 0)
		}
		executeBasicMagic(t, &s, c, d, id, nil, draws...)
		if s.Actors[0].HP != 0 || s.Actors[1].HP != 900 || s.Actors[3].HP != 900 || c.DamageCalls != 1 {
			t.Fatal(id, s, c)
		}
	}
}
