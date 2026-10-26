package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTimeMixedMagicEveryPositionMagicSealBoundary(t *testing.T) {
	_, _, catalog := timeMagicFixture()
	for id := range catalog.Invocations.Entries {
		t.Run(id, func(t *testing.T) {
			s, c, d := timeMagicFixture()
			s.Actors[0].Status = corebattle.StatusMagicSeal
			draws := []float64{.99}
			switch id {
			case "job-29-entry-2":
				draws = []float64{0, .99, .99}
			case "job-29-entry-6":
				draws = []float64{0, .99, .99, .99, .99}
			case "job-30-entry-7":
				draws = []float64{.99, .99}
			}
			executeTimeMagic(t, &s, c, d, id, []string{"enemy"}, draws...)
			wantHP := 500
			if id == "job-31-entry-0" {
				wantHP = 0
			}
			if s.Actors[0].HP != wantHP || s.Actors[1].HP != 900 || s.Actors[0].Status != corebattle.StatusMagicSeal || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.StatDownCalls != 0 || c.HealCalls != 0 {
				t.Fatal(s, c)
			}
			if (s.Actors[0].Temporary == corebattle.TemporaryReduceMagic) != (id == "job-31-entry-3") {
				t.Fatal(s)
			}
		})
	}
}

func TestTimeMixedMagicRouletteRawRosterAndBoundaries(t *testing.T) {
	for _, row := range []struct {
		hp, defense int
		draw        float64
		want        string
	}{
		{999, 999, .5, "enemy"}, {1000, 999, .5, ""},
		{999, 1000, .5, ""}, {999, 999, .99, "self"},
		{999, 999, 0, "self"},
	} {
		t.Run(fmt.Sprint(row), func(t *testing.T) {
			s, c, d := timeMagicFixture()
			s.Actors[1].HP, s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = row.hp, row.defense, 2000
			// Raw roulette bypasses normal targeting, item195, reflection and NPC immunity.
			s.Actors[0].Status, c.Item195Activated = corebattle.StatusConfusion, true
			s.Actors[1].NPC, s.Actors[1].Temporary = true, corebattle.TemporaryReflectMagic
			defeated := ""
			c.Defeat = func(id string) error { defeated = id; return nil }
			executeTimeMagic(t, &s, c, d, "job-31-entry-1", []string{"ignored"}, row.draw)
			if defeated != row.want || c.DamageCalls != 0 || s.Actors[0].Status != corebattle.StatusConfusion {
				t.Fatal(defeated, row, s)
			}
		})
	}
}

func TestTimeMixedMagicExtendedTargetBranches(t *testing.T) {
	for _, target := range []string{"missing", "enemy", "self", "fallen"} {
		t.Run(target, func(t *testing.T) {
			s, c, d := timeMagicFixture()
			draws, wantEnemy, wantSelf := []float64{0, 0, 0, 0, 0, 0}, 702, 500
			if target == "enemy" {
				s.Actors[0].Tension, s.Actors[1].HP = 3, 5000
				draws, wantEnemy = []float64{0}, 3740
			} else if target == "self" || target == "fallen" {
				draws, wantEnemy, wantSelf = []float64{0, 0, 0}, 900, 950
			}
			executeTimeMagic(t, &s, c, d, "job-30-entry-8", []string{target, "tail"}, draws...)
			if s.Actors[1].HP != wantEnemy || s.Actors[0].HP != wantSelf {
				t.Fatal(s)
			}
			if target == "self" && s.Actors[0].Temporary != corebattle.TemporaryReduceAttack || target == "fallen" && s.Actors[2].Temporary != corebattle.TemporaryReduceAttack {
				t.Fatal(s)
			}
		})
	}
	for _, chance := range []float64{.4999, .5} {
		s, c, d := timeMagicFixture()
		executeTimeMagic(t, &s, c, d, "job-30-entry-8", []string{"self"}, chance, 0, 0)
		if (s.Actors[0].Temporary == corebattle.TemporaryReduceAttack) != (chance < .5) {
			t.Fatal(chance, s)
		}
	}
	s, c, d := timeMagicFixture()
	s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 100, 100
	executeTimeMagic(t, &s, c, d, "job-30-entry-8", nil, 0, 0, 0, 0, 0, 0)
	if s.Actors[1].HP != 630 || c.DamageCalls != 3 {
		t.Fatal(s, c)
	}
}

func TestTimeMixedMagicMultiHitUpperEndpointsAndDeathStop(t *testing.T) {
	for _, row := range []struct {
		id          string
		hits, power int
		randomCount bool
	}{
		{"job-29-entry-2", 3, 50, true}, {"job-29-entry-6", 6, 99, true},
		{"job-30-entry-7", 2, 180, false}, {"job-30-entry-8", 3, 100, false},
	} {
		t.Run(row.id, func(t *testing.T) {
			s, c, d := timeMagicFixture()
			s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 100, 100
			draws := make([]float64, row.hits*2)
			if row.randomCount {
				draws = append([]float64{.9999}, draws...)
			}
			executeTimeMagic(t, &s, c, d, row.id, nil, draws...)
			if s.Actors[1].HP != 900-row.hits*int(float64(row.power)*.9) || c.DamageCalls != row.hits {
				t.Fatal(s, c)
			}
			s, c, d = timeMagicFixture()
			s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectMagic
			draws = []float64{0, 0}
			if row.randomCount {
				draws = append([]float64{0}, draws...)
			}
			executeTimeMagic(t, &s, c, d, row.id, nil, draws...)
			if s.Actors[0].HP != 0 || s.Actors[1].HP != 900 || c.DamageCalls != 1 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestTimeMixedMagicHPReductionAndMissingHP(t *testing.T) {
	for _, hp := range []int{1, 200, 999, 1000} {
		s, c, d := timeMagicFixture()
		s.Actors[1].HP = hp
		executeTimeMagic(t, &s, c, d, "job-29-entry-5", []string{"enemy"})
		want := hp
		if hp > 200 && hp <= 999 {
			want = max(200, hp-hp/2)
		}
		if s.Actors[1].HP != want || c.StatDownCalls != 1 {
			t.Fatal(hp, s, c)
		}
	}
	for _, hp := range []int{1000, 1006} {
		s, c, d := timeMagicFixture()
		s.Actors[0].HP = hp
		draws, wantDamage := []float64{0}, 4
		if hp == 1006 {
			draws, wantDamage = []float64{0, 0}, 1
		}
		executeTimeMagic(t, &s, c, d, "job-31-entry-2", []string{"enemy"}, draws...)
		if s.Actors[1].HP != 900-wantDamage {
			t.Fatal(hp, s)
		}
	}
}

func TestTimeMixedMagicTemporaryLivingGuardsAndAliasing(t *testing.T) {
	for _, row := range []struct {
		id   string
		want corebattle.TemporaryEffect
	}{
		{"job-30-entry-2", ""}, {"job-30-entry-4", corebattle.TemporaryReflectMagic}, {"job-30-entry-6", corebattle.TemporaryRevival},
	} {
		s, c, d := timeMagicFixture()
		s.Actors[0].Status = corebattle.StatusConfusion
		executeTimeMagic(t, &s, c, d, row.id, []string{"self"}, .99)
		if s.Actors[2].Temporary != row.want {
			t.Fatal(row.id, s)
		}
	}
	s, c, d := timeMagicFixture()
	s.Actors[0].Status = corebattle.StatusConfusion
	executeTimeMagic(t, &s, c, d, "job-31-entry-0", []string{"self"}, 0)
	if s.Actors[0].HP != 0 || c.DamageCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestTimeMixedMagicAcceptedNoEffectSkipsUnreachableDescendants(t *testing.T) {
	s, c, d := timeMagicFixture()
	for i := range s.Actors {
		s.Actors[i].HP = 1000
	}
	c.Defeat = nil
	executeTimeMagic(t, &s, c, d, "job-31-entry-1", nil, .5)
	if s.Actors[1].HP != 1000 {
		t.Fatal(s)
	}
	s, c, d = timeMagicFixture()
	s.Actors[0].Agility, s.Actors[0].ItemDefinitionIDs = 100, []string{"item-201", "item-205"}
	// Agility is already at its 2.5x cap: neither damage nor requester context is needed.
	executeTimeMagic(t, &s, c, d, "job-29-entry-1", []string{"self"})
	if s.Actors[0].Agility != 100 || s.Actors[0].Temporary != "" {
		t.Fatal(s)
	}
}
