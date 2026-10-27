package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC97AreaDecayAndIndependentStatusResolution(t *testing.T) {
	for _, row := range []struct{ pos, firstHP, secondHP int }{{4, 730, 771}, {7, 640, 694}, {8, 775, 809}, {10, 766, 802}} {
		s, c, d := npc97Fixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		draws := []float64{0, 0}
		if row.pos == 8 || row.pos == 10 {
			draws = append(draws, 0, 0)
		}
		executeNPC97(t, &s, c, d, row.pos, nil, draws...)
		if s.Actors[1].HP != row.firstHP || s.Actors[3].HP != row.secondHP || c.AreaDamageCalls != 1 {
			t.Fatal(row, s.Actors, c)
		}
	}
	for _, pos := range []int{8, 10} {
		s, c, d := npc97Fixture()
		s.Actors[0].HP = 900
		s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
		// The hit activates195; the later status helper independently selects
		// self through confusion instead of reusing the damage recipient.
		executeNPC97(t, &s, c, d, pos, nil, 0, 0, 0, 0)
		status := corebattle.StatusParalyze
		if pos == 10 {
			status = corebattle.StatusSleep
		}
		if s.Actors[0].HP != 900 || s.Actors[0].Status != status || s.Actors[1].Status != "" || !c.Item195Activated || !c.AdditionalEffect || c.AreaStatusCalls != 1 {
			t.Fatal(s.Actors, c)
		}
		// Death stops area damage, but the area status loop still visits the
		// second enemy, after the first reflected status's dead-target no-op.
		s, c, d = npc97Fixture()
		other := s.Actors[1]
		other.ID, other.HP = "other", 900
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectMagic
		executeNPC97(t, &s, c, d, pos, nil, 0, 0)
		if s.Actors[0].HP != 0 || s.Actors[1].HP != 1000 || s.Actors[3].HP != 900 || s.Actors[3].Status != status || c.DamageCalls != 1 || c.StatusCalls != 1 {
			t.Fatal(s.Actors, c)
		}
		// Each helper attempts its own magic-seal cure.
		s, c, d = npc97Fixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		s.Actors[1].HP = 800
		executeNPC97(t, &s, c, d, pos, nil, .9, 0, 0)
		if s.Actors[1].HP != 800 || s.Actors[1].Status != status || c.DamageCalls != 0 || c.StatusCalls != 1 {
			t.Fatal(s.Actors, c)
		}
		chance := .2
		if pos == 10 {
			chance = .3
		}
		for _, draw := range []float64{chance - .001, chance} {
			s, c, d = npc97Fixture()
			executeNPC97(t, &s, c, d, pos, nil, 0, draw)
			if (s.Actors[1].Status == status) != (draw < chance) {
				t.Fatal(pos, draw, s.Actors)
			}
		}
	}
}

func TestNPC97DeterministicPoisonGuardsAndSecondTargetHelper(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, row := range []struct {
			maxHP, baseDF, hp, wantHP int
			poison                    bool
		}{
			{998, 998, 800, 200, true}, {999, 999, 800, 200, true},
			{1000, 999, 800, 800, false}, {999, 1000, 800, 800, false},
			{999, 999, 1000, 1000, true}, {999, 999, 100, 100, true},
		} {
			t.Run(fmt.Sprintf("npc-%t/%d/%d/%d", npc, row.maxHP, row.baseDF, row.hp), func(t *testing.T) {
				s, c, d := npc97Fixture()
				s.Actors[1].NPC, s.Actors[1].MaxHP, s.Actors[1].BaseStats.Defense, s.Actors[1].HP = npc, row.maxHP, row.baseDF, row.hp
				executeNPC97(t, &s, c, d, 9, []string{"enemy"}) // No stochastic gate (unlike93.3).
				if s.Actors[1].HP != row.wantHP || (s.Actors[1].Status == corebattle.StatusDeadlyPoison) != row.poison {
					t.Fatal(row, s.Actors, c)
				}
			})
		}
	}
	for _, kind := range []string{"item159", "null", "shield", "second-reflection", "seal-failure"} {
		s, c, d := npc97Fixture()
		s.Actors[1].HP = 800
		draws, enemyHP, selfHP, poison := []float64(nil), 800, 1000, "enemy"
		switch kind {
		case "item159":
			s.Actors[1].ItemDefinitionIDs = []string{"item-159"}
		case "null":
			s.Actors[1].Temporary = corebattle.TemporaryNullMagic
		case "shield":
			s.Actors[1].ItemDefinitionIDs, s.Actors[1].Tension, enemyHP = []string{"item-161"}, 5, 200
		case "second-reflection":
			// First helper reflects to self; numeric HP-down treats self as an
			// ally and randomly resolves enemy again, reflecting back to self.
			s.Actors[0].MaxHP, s.Actors[0].HP, s.Actors[1].Temporary = 999, 800, corebattle.TemporaryReflectMagic
			draws, selfHP, poison = []float64{0}, 200, "self"
		case "seal-failure":
			s.Actors[0].Status, draws, poison = corebattle.StatusMagicSeal, []float64{.9}, ""
		}
		executeNPC97(t, &s, c, d, 9, []string{"enemy"}, draws...)
		if s.Actors[0].HP != selfHP || s.Actors[1].HP != enemyHP || (s.Actors[0].Status == corebattle.StatusDeadlyPoison) != (poison == "self") || (s.Actors[1].Status == corebattle.StatusDeadlyPoison) != (poison == "enemy") {
			t.Fatal(kind, s.Actors, c)
		}
	}
}
