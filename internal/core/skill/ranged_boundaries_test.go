package skill

import (
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRangedCollapseThresholdAndDirectPoison(t *testing.T) {
	for _, hp := range []int{999, 1000} {
		for _, df := range []int{999, 1000} {
			for _, draw := range []float64{0, .5, 2.0 / 3, math.Nextafter(1, 0)} {
				s, c, d := rangedFixture()
				s.Actors[1].MaxHP, s.Actors[1].BaseStats.Defense = hp, df
				s.Actors[1].Tension = 5
				draws := []float64{}
				if hp <= 999 && df <= 999 {
					draws = []float64{draw}
				}
				executeRanged(t, &s, c, d, "job-42-entry-5", []string{"enemy"}, draws...)
				success := hp == 999 && df == 999 && draw < 2.0/3
				wantHP, wantState := 900, ""
				if success {
					wantHP, wantState = 225, corebattle.StatusDeadlyPoison
				}
				if s.Actors[1].HP != wantHP || s.Actors[1].Status != wantState || s.Actors[1].Tension != 5 {
					t.Fatal(hp, df, draw, s.Actors[1])
				}
			}
		}
	}
	// Poison is a direct tail even when the stat-down helper has no effect.
	for _, blocked := range []bool{false, true} {
		s, c, d := rangedFixture()
		s.Actors[1].ItemDefinitionIDs = []string{"item-159", "item-161"}
		s.Actors[1].Tension = 8
		if blocked {
			s.Actors[1].Temporary = corebattle.TemporaryNullMagic
		}
		executeRanged(t, &s, c, d, "job-42-entry-5", []string{"enemy"}, 0)
		if s.Actors[1].HP != 900 || s.Actors[1].Status != corebattle.StatusDeadlyPoison || c.Status.StatusCalls != 0 {
			t.Fatal(blocked, s, c)
		}
	}
}

func TestRangedMPTheftCapAndNoReturn(t *testing.T) {
	for _, mp := range []int{1000, 1006, 1007, 2000} {
		for _, draw := range []float64{0, .5, math.Nextafter(1, 0)} {
			s, c, d := rangedFixture()
			s.Actors[0].MaxMP = 3000
			s.Actors[1].MP, s.Actors[1].MaxMP = mp, mp
			amount := int(float64(mp) * .15)
			draws := []float64{0}
			if amount > 150 {
				amount, draws = int(100+draw*50), []float64{draw, 0}
			}
			executeRanged(t, &s, c, d, "job-11-entry-4", []string{"enemy"}, draws...)
			if s.Actors[1].MP != mp-amount || s.Actors[0].MP != 80+int(float64(amount)*.9)+1 || c.MPHealCalls != 1 {
				t.Fatal(mp, draw, s, c)
			}
		}
	}
	for _, guard := range []string{"floor", "dead", "curse", "null", "base-defense"} {
		s, c, d := rangedFixture()
		switch guard {
		case "floor":
			s.Actors[1].MP = 20
		case "dead":
			s.Actors[1].HP = 0
		case "curse":
			s.Actors[1].Abilities = []string{"curse"}
		case "null":
			s.Actors[1].Temporary = corebattle.TemporaryNullAttack
		case "base-defense":
			s.Actors[1].BaseStats.Defense = 1000
		}
		executeRanged(t, &s, c, d, "job-11-entry-4", []string{"enemy"})
		if s.Actors[0].MP != 80 || c.MPHealCalls != 0 {
			t.Fatal(guard, s, c)
		}
	}
}

func TestRangedStatusRatesAndDeathBoundary(t *testing.T) {
	for _, row := range []struct {
		id, symbol string
		chance     float64
		death      bool
	}{
		{"job-11-entry-1", corebattle.StatusDofuu, 80, false},
		{"job-11-entry-6", corebattle.StatusSleep, 55, false},
		{"job-42-entry-0", corebattle.StatusConfusion, 80, false},
		{"job-42-entry-1", corebattle.StatusMagicSeal, 90, false},
		{"job-42-entry-3", corebattle.StatusDeadlyPoison, 70, false},
		{"job-42-entry-4", corebattle.StatusParalyze, 35, false},
		{"job-42-entry-6", "", 30, true},
		{"job-54-entry-0", corebattle.StatusDofuu, 80, false},
		{"job-54-entry-3", corebattle.StatusDofuu, 30, false},
	} {
		for _, draw := range []float64{0, .5, row.chance / 100, math.Nextafter(1, 0)} {
			s, c, d := rangedFixture()
			draws := []float64{draw}
			if row.id == "job-11-entry-6" || row.id == "job-42-entry-3" || row.id == "job-42-entry-4" || row.death || row.id == "job-54-entry-3" {
				draws = []float64{0, draw}
			}
			executeRanged(t, &s, c, d, row.id, []string{"enemy"}, draws...)
			success := draw*100 < row.chance || row.death && draw*100 == row.chance
			if row.death {
				if (s.Actors[1].HP == 0) != success {
					t.Fatal(row, draw, s)
				}
			} else if (s.Actors[1].Status == row.symbol) != success {
				t.Fatal(row, draw, s)
			}
		}
	}
}

func TestRangedDirectDefenseAndRepeatedDeath(t *testing.T) {
	for _, id := range []string{"job-11-entry-7", "job-54-entry-5"} {
		for _, countDraw := range []float64{0, .5, math.Nextafter(1, 0)} {
			s, c, d := rangedFixture()
			count, damage := 3+int(countDraw*2), 38
			if id == "job-54-entry-5" {
				damage = 63
			}
			draws := append([]float64{countDraw}, make([]float64, count*2)...)
			executeRanged(t, &s, c, d, id, []string{"ignored"}, draws...)
			if s.Actors[1].HP != 900-count*damage || c.DamageCalls != count {
				t.Fatal(id, countDraw, s, c)
			}
		}
		// Reflection kills the caster on the first hit; later hits cannot execute.
		s, c, d := rangedFixture()
		s.Actors[0].HP, s.Actors[0].Defense = 1, 0
		s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
		executeRanged(t, &s, c, d, id, nil, 0, 0, 0)
		if s.Actors[0].HP != 0 || s.Actors[1].HP != 900 || c.DamageCalls != 1 {
			t.Fatal(id, s, c)
		}
	}
	for _, df := range []int{999, 1000} {
		s, c, d := rangedFixture()
		s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = 50, df
		executeRanged(t, &s, c, d, "job-54-entry-1", []string{"enemy"}, 0)
		want := 846
		if df == 1000 {
			want = 887 // (60*.5-50*.3)*.9 =>13.
		}
		if s.Actors[1].HP != want {
			t.Fatal(df, s)
		}
	}
}
