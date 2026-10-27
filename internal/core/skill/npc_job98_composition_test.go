package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC98ReflectionSealAndBarrierNoEffectTails(t *testing.T) {
	for _, cured := range []bool{false, true} {
		s, c, d := npc98Fixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		draw, want := .9, corebattle.TemporaryEffect("")
		if cured {
			draw, want = 0, corebattle.TemporaryReflectMagic
		}
		executeNPC98(t, &s, c, d, 2, nil, draw)
		if s.Actors[0].Temporary != want || s.Actors[2].Temporary != "" || (s.Actors[0].Status == "") != cured {
			t.Fatal(cured, s.Actors)
		}
	}
	for _, blocked := range []string{"cap", "binding", "fallen", "zero-modifier", "seal"} {
		t.Run(blocked, func(t *testing.T) {
			s, c, d := npc98Fixture()
			s.Actors[2].HP, s.Actors[2].MaxHP = 100, 100
			s.Actors[2].Defense, s.Actors[2].BaseStats.Defense = 100, 100
			var draws []float64
			switch blocked {
			case "cap":
				s.Actors[0].Defense, s.Actors[2].Defense = 200, 250
			case "binding":
				s.Actors[0].Status, s.Actors[2].Status = corebattle.StatusSabaku, corebattle.StatusSabaku
			case "fallen":
				s.Actors[0].HP = 0
				s.Actors[2].Defense = 250
			case "zero-modifier":
				s.Fields.Default = "darkness"
			case "seal":
				s.Actors[0].Status, draws = corebattle.StatusMagicSeal, []float64{.9}
			}
			self, ally := s.Actors[0].Defense, s.Actors[2].Defense
			executeNPC98(t, &s, c, d, 3, nil, draws...)
			wantSelf := corebattle.TemporaryReduceMagic
			if blocked == "fallen" {
				wantSelf = ""
			}
			if s.Actors[0].Defense != self || s.Actors[2].Defense != ally || s.Actors[0].Temporary != wantSelf || s.Actors[2].Temporary != corebattle.TemporaryReduceMagic {
				t.Fatal(s.Actors, c)
			}
		})
	}
	// A fallen party position falls back to the living caster: the baseline
	// manifest deliberately checks that its defense buff is applied twice.
	s, c, d := npc98Fixture()
	s.Actors[2].HP, s.Actors[2].MaxHP = 100, 100
	s.Actors[2].Defense, s.Actors[2].BaseStats.Defense = 100, 100
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
	level := 99
	c.RequesterLevel = &level
	executeNPC98(t, &s, c, d, 3, nil)
	// Self205 raises tension to1.7, so the following ally receives85 DF.
	if s.Actors[0].Attack != 140 || s.Actors[0].Defense != 120 || s.Actors[2].Defense != 185 || c.TensionCalls != 1 || s.Actors[0].Tension != 1 || s.Actors[0].Temporary != corebattle.TemporaryReduceMagic {
		t.Fatal(s.Actors, c)
	}
}

func TestNPC98TensionUsesRequesterAndRetainsEquipmentTails(t *testing.T) {
	s, c, d := npc98Fixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[0].Tension = 3
	level := 99
	c.RequesterLevel = &level
	executeNPC98(t, &s, c, d, 4, nil, 0)
	if s.Actors[0].Tension != 8 || s.Actors[0].Temporary != corebattle.TemporaryDouble || c.TensionCalls != 2 {
		t.Fatal(s.Actors, c)
	}
	s, c, d = npc98Fixture()
	s.Actors[0].Status = corebattle.StatusSabaku
	executeNPC98(t, &s, c, d, 4, nil)
	if s.Actors[0].Tension != 1 || c.TensionCalls != 2 || s.Actors[0].Temporary != corebattle.TemporaryDouble {
		t.Fatal(s.Actors, c)
	}
	s, c, d = npc98Fixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-161"}
	executeNPC98(t, &s, c, d, 4, nil, 0, .9, .9)
	if s.Actors[0].Tension != 3 || s.Actors[0].Temporary != corebattle.TemporaryDefense {
		t.Fatal(s.Actors, c)
	}
}

func TestNPC98RainContinuesHealingAfterCasterDeath(t *testing.T) {
	s, c, d := npc98Fixture()
	s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectMagic
	s.Actors[2].HP, s.Actors[2].MaxHP = 10, 1000
	other := s.Actors[1]
	other.ID, other.Temporary = "other", ""
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	executeNPC98(t, &s, c, d, 6, nil, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[1].HP != 1000 || s.Actors[3].HP != 1000 || s.Actors[2].HP != 190 || c.DamageCalls != 1 || c.HealCalls != 1 || c.PartyHealCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	// The area helper and later healing helper attempt magic-seal cures
	// separately. Failure of the first does not suppress the second.
	s, c, d = npc98Fixture()
	s.Actors[0].HP, s.Actors[0].Status = 100, corebattle.StatusMagicSeal
	executeNPC98(t, &s, c, d, 6, nil, .9, 0, 0, 0)
	if s.Actors[0].HP != 460 || s.Actors[0].Status != "" || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || c.HealCalls != 2 {
		t.Fatal(s.Actors, c)
	}
}

func TestNPC98BreathWritesOriginalTargetUnconditionally(t *testing.T) {
	for _, kind := range []string{"dead", "one-hit", "reflection", "cover", "death"} {
		t.Run(kind, func(t *testing.T) {
			s, c, d := npc98Fixture()
			other := s.Actors[1]
			other.ID = "other"
			s.Actors = append(s.Actors, other)
			c.EnemyIDs = append(c.EnemyIDs, "other")
			draws := []float64{0, 0}
			wantSelf, wantEnemy, wantOther := 1000, 910, 910
			switch kind {
			case "dead":
				s.Actors[1].HP, wantEnemy, draws = 0, 0, []float64{0}
			case "one-hit":
				s.Actors[1].Temporary, wantEnemy, draws = corebattle.TemporaryOneHitDefense, 1000, []float64{0}
			case "reflection":
				s.Actors[1].Temporary, s.Actors[0].HP = corebattle.TemporaryReflectBreath, 1
				wantSelf, wantEnemy = 0, 1000 // No caster-death guard before other.
			case "cover":
				s.Actors[1].Temporary, s.Actors[3].Temporary = corebattle.TemporaryCover, corebattle.TemporaryCovering
				wantEnemy, wantOther = 1000, 820
			case "death":
				s.Actors[1].HP, wantEnemy = 1, 0
			}
			executeNPC98(t, &s, c, d, 7, nil, draws...)
			if s.Actors[0].HP != wantSelf || s.Actors[1].HP != wantEnemy || s.Actors[3].HP != wantOther || s.Actors[1].Temporary != corebattle.TemporaryDouble || s.Actors[3].Temporary != corebattle.TemporaryDouble {
				t.Fatal(s.Actors, c)
			}
		})
	}
}

func TestNPC98AreaDecayAndMeteorCountDeath(t *testing.T) {
	for _, row := range []struct{ pos, firstHP, secondHP int }{{6, 820, 847}, {8, 685, 733}, {10, 640, 694}} {
		s, c, d := npc98Fixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		draws := []float64{0, 0}
		if row.pos == 6 {
			draws = append(draws, 0, 0)
		}
		executeNPC98(t, &s, c, d, row.pos, nil, draws...)
		if s.Actors[1].HP != row.firstHP || s.Actors[3].HP != row.secondHP || c.AreaDamageCalls != 1 {
			t.Fatal(row, s.Actors, c)
		}
	}
	for count, draw := range []float64{0, .5, .999} {
		t.Run(fmt.Sprintf("hits-%d", count+4), func(t *testing.T) {
			s, c, d := npc98Fixture()
			s.Actors[1].HP = 5000
			draws := []float64{draw}
			for range count + 4 {
				draws = append(draws, 0, 0)
			}
			executeNPC98(t, &s, c, d, 9, nil, draws...)
			if s.Actors[1].HP != 5000-225*(count+4) || c.DamageCalls != count+4 {
				t.Fatal(s.Actors, c)
			}
		})
	}
	s, c, d := npc98Fixture()
	s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectMagic
	executeNPC98(t, &s, c, d, 9, nil, .999, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[1].HP != 1000 || c.DamageCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	s, c, d = npc98Fixture()
	s.Actors[0].HP, c.Defeat = 0, nil
	executeNPC98(t, &s, c, d, 9, nil, .999)
	if c.DamageCalls != 0 {
		t.Fatal(c)
	}
}
