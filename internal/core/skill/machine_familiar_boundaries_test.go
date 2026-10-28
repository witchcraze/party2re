package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestMachineFamiliarDrainRequestedAmountAndGuards(t *testing.T) {
	for _, row := range []struct {
		mode       string
		mp, selfMP int
		draws      []float64
	}{
		{"ordinary", 75, 103, []float64{0}},
		{"floor", 20, 85, []float64{0}},
		{"curse", 100, 80, nil},
		{"sealed", 100, 80, []float64{.25}},
		{"reflected", 100, 79, []float64{0}},
	} {
		t.Run(row.mode, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			switch row.mode {
			case "floor":
				s.Actors[1].MP = 23
			case "curse":
				s.Actors[1].Abilities = []string{"curse"}
			case "sealed":
				s.Actors[0].Status = corebattle.StatusMagicSeal
			case "reflected":
				s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
			}
			executeMachineFamiliar(t, &s, c, d, "job-79-entry-0", []string{"enemy"}, row.draws...)
			if s.Actors[1].MP != row.mp || s.Actors[0].MP != row.selfMP || s.Actors[0].HP != 500 {
				t.Fatal(s)
			}
		})
	}
}

func TestMachineFamiliarDamageSealAndChanceBoundaries(t *testing.T) {
	for _, id := range []string{"job-79-entry-1", "job-81-entry-0", "job-82-entry-5"} {
		s, c, d := machineFamiliarFixture()
		s.Actors[0].Status = corebattle.StatusPhysicalSeal
		draws, want := []float64{.25}, 900
		if id == "job-82-entry-5" {
			draws, want = []float64{0}, 747
		}
		executeMachineFamiliar(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[1].HP != want {
			t.Fatal(id, s)
		}
	}
	for _, chance := range []float64{.499, .5, .501} {
		s, c, d := machineFamiliarFixture()
		draws := []float64{chance, 0, 0}
		want := 899 // physical20 is reduced by defense, then minimum damage.
		if chance < .5 {
			want = 787
			draws = draws[:2]
		}
		executeMachineFamiliar(t, &s, c, d, "job-81-entry-3", []string{"enemy"}, draws...)
		if s.Actors[1].HP != want {
			t.Fatal(chance, s)
		}
	}
}

func TestMachineFamiliarTensionUsesReturnedTarget(t *testing.T) {
	for _, mode := range []string{"living", "dead", "reflected-dead", "cover", "one-hit", "sealed", "fallen"} {
		t.Run(mode, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			draws, calls, tension := []float64{0}, 1, 1.7
			switch mode {
			case "dead":
				s.Actors[1].HP, calls, tension = 1, 2, 3
			case "reflected-dead":
				s.Actors[0].HP, s.Actors[1].Temporary, calls, tension = 1, corebattle.TemporaryReflectAttack, 2, 3
			case "cover":
				other := s.Actors[1]
				other.ID, other.HP, other.Temporary = "coverer", 1, corebattle.TemporaryCovering
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, other.ID)
				s.Actors[1].Temporary, calls, tension = corebattle.TemporaryCover, 2, 3
			case "one-hit":
				s.Actors[1].Temporary, draws, calls, tension = corebattle.TemporaryOneHitDefense, nil, 0, 1
			case "sealed":
				s.Actors[0].Status, draws, calls, tension = corebattle.StatusPhysicalSeal, []float64{.25}, 0, 1
			case "fallen":
				s.Actors[1].HP, draws, calls, tension = 0, nil, 0, 1
			}
			executeMachineFamiliar(t, &s, c, d, "job-79-entry-4", []string{"enemy"}, draws...)
			if c.TensionCalls != calls || s.Actors[0].Tension != tension {
				t.Fatal(s, c)
			}
			if mode == "cover" && (s.Actors[1].HP != 900 || s.Actors[3].HP != 0) || mode == "reflected-dead" && (s.Actors[0].HP != 0 || s.Actors[1].HP != 900) {
				t.Fatal(s)
			}
		})
	}
}

func TestMachineFamiliarSongOrderAndIndependentHeal(t *testing.T) {
	for _, chance := range []float64{.249, .25} {
		s, c, d := machineFamiliarFixture()
		s.Actors[2].HP, s.Actors[2].MaxHP = 100, 1000
		s.Actors[2].BaseStats = corebattle.BattleBaseStats{Attack: 50, Defense: 40}
		s.Actors[2].Attack, s.Actors[2].Defense = 50, 40
		s.Actors[0].Tension, s.Actors[0].Status = 3, corebattle.StatusMagicSeal
		draws := []float64{chance, .249, 0}
		if chance < .25 {
			draws = []float64{chance, 0, .25}
		}
		executeMachineFamiliar(t, &s, c, d, "job-81-entry-4", nil, draws...)
		a, ally := s.Actors[0], s.Actors[2]
		wantHP, allyHP := 500, 235
		if chance < .25 {
			wantHP, allyHP = 635, 100
		}
		if a.Attack != 160 || a.Defense != 96 || ally.Attack != 60 || ally.Defense != 48 || a.HP != wantHP || ally.HP != allyHP || a.Status != corebattle.StatusMagicSeal || c.StatUpCalls != 4 {
			t.Fatal(chance, s, c)
		}
	}
}

func TestMachineFamiliarHammerRestoresAttackAfterDeath(t *testing.T) {
	s, c, d := machineFamiliarFixture()
	s.Actors[1].Temporary, s.Actors[0].HP = corebattle.TemporaryReflectAttack, 1
	c.Defeat = func(id string) error { s.Actors[0].Attack = 777; return nil }
	executeMachineFamiliar(t, &s, c, d, "job-82-entry-1", []string{"enemy"}, .5, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].Attack != 100 || c.DamageCalls != 1 {
		t.Fatal(s, c)
	}
	// The intrinsic callback invokes the ordinary helper once, independently
	// of the outer ordinary attack's seal10 repeat.
	s, c, d = machineFamiliarFixture()
	s.Actors[0].Abilities = []string{"seal_shinsoku"}
	s.Actors[1].HP = 0
	executeMachineFamiliar(t, &s, c, d, "job-82-entry-1", []string{"enemy"}, .5)
	if s.Actors[0].Attack != 100 || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
}

func TestMachineFamiliarTeaResolvesThenHealsThenCures(t *testing.T) {
	s, c, d := machineFamiliarFixture()
	// Each helper resolves independently: fallen -> self -> covering fallen.
	s.Actors[0].Temporary, s.Actors[2].Temporary = corebattle.TemporaryCover, corebattle.TemporaryCovering
	s.Actors[0].Status, s.Actors[2].Status = corebattle.StatusPoison, corebattle.StatusSleep
	executeMachineFamiliar(t, &s, c, d, "job-82-entry-2", []string{"self"})
	if s.Actors[0].HP != 500 || s.Actors[0].Status != corebattle.StatusPoison || s.Actors[2].Status != corebattle.StatusSleep || c.CureCalls != 0 {
		t.Fatal(s, c)
	}
	s, c, d = machineFamiliarFixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeMachineFamiliar(t, &s, c, d, "job-82-entry-2", []string{"self"}, .25)
	if c.HealCalls != 0 || c.CureCalls != 0 || s.Actors[0].Status != corebattle.StatusMagicSeal {
		t.Fatal(s, c)
	}
	// Confusion makes all three helpers select independently. Cure takes the
	// status from the first resolved ally, not the subsequently healed caster.
	s, c, d = machineFamiliarFixture()
	s.Actors[0].Status = corebattle.StatusConfusion
	s.Actors[2].HP, s.Actors[2].Status = 100, corebattle.StatusSleep
	s.Actors[1].Status = corebattle.StatusSleep
	executeMachineFamiliar(t, &s, c, d, "job-82-entry-2", []string{"self"}, .9, 0, 0, .5)
	if s.Actors[0].HP != 635 || s.Actors[0].Status != corebattle.StatusConfusion || s.Actors[1].Status != "" || s.Actors[2].Status != corebattle.StatusSleep || c.CureCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestMachineFamiliarStatChoicesAndCap(t *testing.T) {
	for choice := 0; choice < 3; choice++ {
		for _, capped := range []bool{false, true} {
			t.Run(fmt.Sprintf("choice%d/capped%t", choice, capped), func(t *testing.T) {
				s, c, d := machineFamiliarFixture()
				a := &s.Actors[0]
				if capped {
					a.Attack, a.Defense, a.Agility = 250, 200, 100
				}
				before := []int{a.Attack, a.Defense, a.Agility}
				executeMachineFamiliar(t, &s, c, d, "job-82-entry-3", nil, (float64(choice)+.5)/3)
				want := append([]int(nil), before...)
				if !capped {
					want[choice] += []int{18, 14, 7}[choice]
				}
				if fmt.Sprint([]int{a.Attack, a.Defense, a.Agility}) != fmt.Sprint(want) {
					t.Fatal(s)
				}
			})
		}
	}
}

func TestMachineFamiliarArrowsCountAndEarlyReturns(t *testing.T) {
	for _, mode := range []string{"three", "four", "missing", "one-hit", "actor-death", "already-dead"} {
		t.Run(mode, func(t *testing.T) {
			s, c, d := machineFamiliarFixture()
			draws, calls, damage := []float64{0, 0, 0, 0, 0, 0, 0}, 3, 54
			switch mode {
			case "four":
				draws, calls, damage = []float64{.5, 0, 0, 0, 0, 0, 0, 0, 0}, 4, 72
			case "missing":
				s.Actors[1].HP, draws, calls, damage = 0, []float64{0, 0}, 0, 900
			case "one-hit":
				s.Actors[1].Temporary, draws, calls, damage = corebattle.TemporaryOneHitDefense, []float64{0, 0}, 1, 0
			case "actor-death":
				s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectAttack
				draws, calls, damage = []float64{0, 0, 0}, 1, 0
			case "already-dead":
				s.Actors[0].HP, draws, calls, damage = 0, []float64{0}, 0, 0
			}
			executeMachineFamiliar(t, &s, c, d, "job-82-entry-4", []string{"ignored", "tail"}, draws...)
			if c.DamageCalls != calls || s.Actors[1].HP != 900-damage || len(c.Arguments) != 2 {
				t.Fatal(s, c)
			}
		})
	}
}
