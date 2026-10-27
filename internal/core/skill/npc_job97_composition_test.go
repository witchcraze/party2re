package skill

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC97RepeatedCountDeathAndPerHitAttack(t *testing.T) {
	for _, pos := range []int{0, 1} {
		maxHits, damage := 4, 38
		if pos == 1 {
			maxHits, damage = 5, 40
		}
		for count := 3; count <= maxHits; count++ {
			s, c, d := npc97Fixture()
			draws := []float64{(float64(count-3) + .5) / float64(maxHits-2)}
			for range count {
				draws = append(draws, 0, 0)
			}
			executeNPC97(t, &s, c, d, pos, nil, draws...)
			if s.Actors[1].HP != 1000-count*damage || c.DamageCalls != count {
				t.Fatal(s.Actors, c)
			}
		}
		s, c, d := npc97Fixture()
		s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectAttack
		executeNPC97(t, &s, c, d, pos, nil, .999, 0, 0)
		if s.Actors[0].HP != 0 || s.Actors[1].HP != 1000 || c.DamageCalls != 1 {
			t.Fatal(s.Actors, c)
		}
		s, c, d = npc97Fixture()
		s.Actors[0].HP, c.Defeat = 0, nil
		executeNPC97(t, &s, c, d, pos, nil, .999)
		if c.DamageCalls != 0 {
			t.Fatal(c)
		}
		// A defeat hook may update actor AT; subsequent hits read it anew.
		s, c, d = npc97Fixture()
		s.Actors[1].HP = 1
		c.Defeat = func(string) error { s.Actors[0].Attack = 200; s.Actors[1].HP = 1000; return nil }
		executeNPC97(t, &s, c, d, pos, nil, 0, 0, 0, 0, 0, 0, 0)
		laterDamage := 76
		if pos == 1 {
			laterDamage = 81
		}
		if s.Actors[1].HP != 1000-2*laterDamage {
			t.Fatal(s.Actors, c)
		}
	}
}

func TestNPC97DirectDefenseLossAndUnconditionalTails(t *testing.T) {
	for _, kind := range []string{"normal", "capped", "bound", "fallen", "zero-modifier"} {
		t.Run(kind, func(t *testing.T) {
			s, c, d := npc97Fixture()
			s.Actors[0].Defense, s.Actors[0].Tension = 81, 3
			s.Actors[0].ItemDefinitionIDs = []string{"item-159"}
			wantAT, wantTension := 250, 1.0
			switch kind {
			case "capped":
				s.Actors[0].Attack, wantAT, wantTension = 250, 250, 3
			case "bound":
				s.Actors[0].Status, wantAT, wantTension = corebattle.StatusSabaku, 100, 3
			case "fallen":
				s.Actors[0].HP, wantAT, wantTension = 0, 100, 3
			case "zero-modifier":
				s.Fields.Default, wantAT, wantTension = "darkness", 100, 3
			}
			executeNPC97(t, &s, c, d, 3, nil)
			if s.Actors[0].Defense != 41 || s.Actors[0].Attack != wantAT || s.Actors[0].Tension != wantTension {
				t.Fatal(s.Actors)
			}
		})
	}
	for _, kind := range []string{"seal", "fallen", "reflection"} {
		s, c, d := npc97Fixture()
		var draws []float64
		switch kind {
		case "seal":
			s.Actors[0].Status, draws = corebattle.StatusPhysicalSeal, []float64{.9}
		case "fallen":
			s.Actors[0].HP = 0
		case "reflection":
			s.Actors[0].HP, s.Actors[1].Temporary, draws = 1, corebattle.TemporaryReflectAttack, []float64{0}
		}
		executeNPC97(t, &s, c, d, 4, nil, draws...)
		if s.Actors[0].Temporary != corebattle.TemporaryDouble {
			t.Fatal(s.Actors)
		}
	}
}

func TestNPC97RecoilUsesReturnedDamage(t *testing.T) {
	for _, kind := range []string{"positive", "one-hit", "null", "miss", "reflection-death", "darkness", "item160"} {
		t.Run(kind, func(t *testing.T) {
			s, c, d := npc97Fixture()
			draws, wantHP, risk := []float64{0}, 977, 1
			switch kind {
			case "one-hit":
				s.Actors[1].Temporary, draws, wantHP, risk = corebattle.TemporaryOneHitDefense, nil, 1000, 0
			case "null":
				s.Actors[1].Temporary, draws, wantHP, risk = corebattle.TemporaryNullAttack, nil, 1000, 0
			case "miss":
				c.GuaranteedHit, s.Actors[0].Hit, draws, wantHP, risk = false, 0, []float64{.9}, 1000, 0
			case "reflection-death":
				s.Actors[0].HP, s.Actors[1].Temporary, wantHP, risk = 1, corebattle.TemporaryReflectAttack, 0, 0
			case "darkness":
				s.Fields.Default, wantHP = "darkness", 1000
			case "item160":
				s.Actors[0].HP, s.Actors[0].Status, s.Actors[0].ItemDefinitionIDs = 500, corebattle.StatusSleep, []string{"item-160"}
				draws, wantHP = []float64{0, 0}, 523
			}
			executeNPC97(t, &s, c, d, 5, []string{"enemy"}, draws...)
			if s.Actors[0].HP != wantHP || c.End.ImmediateRiskCalls != risk {
				t.Fatal(s.Actors, c.End)
			}
			if kind == "item160" && (s.Actors[0].Status != "" || s.Actors[0].Attack != 170) {
				t.Fatal(s.Actors)
			}
		})
	}
}

func TestNPC97TensionRequesterAndEquipment(t *testing.T) {
	for _, row := range []struct {
		level      int
		draw, want float64
	}{{24, -1, 5}, {25, 0, 8}, {25, .9, 5}, {99, .9, 8}} {
		s, c, d := npc97Fixture()
		s.Actors[0].NPC, c.ActorNPC, s.Actors[0].Tension = true, true, 3
		c.RequesterLevel = &row.level
		var draws []float64
		if row.draw >= 0 {
			draws = []float64{row.draw}
		}
		executeNPC97(t, &s, c, d, 6, nil, draws...)
		if s.Actors[0].Tension != row.want || c.TensionCalls != 2 {
			t.Fatal(row, s.Actors, c)
		}
	}
	s, c, d := npc97Fixture()
	s.Actors[0].Status = corebattle.StatusSabaku
	executeNPC97(t, &s, c, d, 6, nil)
	if s.Actors[0].Tension != 1 || c.TensionCalls != 2 || s.Actors[0].Temporary != corebattle.TemporaryDouble {
		t.Fatal(s.Actors, c)
	}
	s, c, d = npc97Fixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-161"}
	executeNPC97(t, &s, c, d, 6, nil, 0, .9, .9)
	if s.Actors[0].Tension != 3 || s.Actors[0].Temporary != corebattle.TemporaryDefense {
		t.Fatal(s.Actors)
	}
}
