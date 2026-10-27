package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPCStatusDamageReturnAndArmedClaw(t *testing.T) {
	for _, row := range []struct {
		weapon string
		hp     int
	}{{"", 665}, {"weapon-01", 728}} {
		s, c, d := npcStatusFixture()
		s.Actors[0].Gear.WeaponID = row.weapon
		executeNPCStatus(t, &s, c, d, 88, 1, []string{"enemy"}, 0)
		if s.Actors[1].HP != row.hp {
			t.Fatal(row, s.Actors)
		}
	}
	for _, key := range []string{"one-hit", "miss", "dead", "null", "no-enemies", "lethal", "revived", "zero-modifier"} {
		t.Run(key, func(t *testing.T) {
			s, c, d := npcStatusFixture()
			c.GuaranteedHit = false
			draws := []float64{0, .5, 0}
			wantCalls := 0
			switch key {
			case "one-hit":
				s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				draws = nil
			case "miss":
				s.Actors[0].Hit = 0
				draws = []float64{.9}
			case "dead":
				s.Actors[1].HP = 0
				draws = nil
			case "null":
				s.Actors[1].Temporary = corebattle.TemporaryNullAttack
				draws = nil
			case "no-enemies":
				s.Actors[1].TeamID = "party"
				c.PartyIDs = append(c.PartyIDs, "enemy")
				c.EnemyIDs = nil
				draws = nil
			case "lethal":
				s.Actors[1].HP = 1
				c.Defeat = func(string) error { return nil }
			case "revived":
				s.Actors[1].HP = 1
				c.Defeat = func(string) error { s.Actors[1].HP = 300; return nil }
				draws = append(draws, 0)
				wantCalls = 1
			case "zero-modifier":
				s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
				c.DamageCalls = 1
				draws = append(draws, 0)
				wantCalls = 1
			}
			executeNPCStatus(t, &s, c, d, 90, 0, []string{"enemy"}, draws...)
			if c.StatusCalls != wantCalls || key != "revived" && s.Actors[1].Status != "" || key == "revived" && s.Actors[1].Status != corebattle.StatusDeadlyPoison {
				t.Fatal(s.Actors, c)
			}
		})
	}
	// A physical reflection returns the caster; the second helper retains that
	// argument and resolves it again, including its same-side fallback and a second reflection.
	s, c, d := npcStatusFixture()
	s.Actors[0].HP = 500
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	executeNPCStatus(t, &s, c, d, 91, 0, []string{"enemy"}, 0, 0, 0)
	if s.Actors[0].HP != 468 || s.Actors[0].Status != corebattle.StatusParalyze || s.Actors[1].Status != "" {
		t.Fatal(s.Actors)
	}
}

func TestNPCStatusProbabilityAndImmunityBoundaries(t *testing.T) {
	for _, row := range []struct {
		job, pos int
		chance   float64
		death    bool
		prefix   []float64
	}{
		{88, 0, 60, false, nil}, {90, 0, 70, false, []float64{0}}, {90, 1, 80, false, []float64{0}}, {90, 2, 60, false, nil},
		{91, 0, 30, false, []float64{0}}, {91, 1, 40, false, []float64{0}}, {91, 2, 35, false, nil},
		{92, 0, 65, false, nil}, {92, 1, 55, false, []float64{0}}, {92, 2, 35, false, nil},
		{93, 0, 20, true, nil}, {93, 1, 20, true, nil}, {93, 2, 17, true, nil}, {94, 0, 60, true, nil},
	} {
		for _, draw := range []float64{(row.chance - .0001) / 100, row.chance / 100, (row.chance + .0001) / 100} {
			s, c, d := npcStatusFixture()
			executeNPCStatus(t, &s, c, d, row.job, row.pos, []string{"enemy"}, append(append([]float64(nil), row.prefix...), draw)...)
			applied := s.Actors[1].Status != ""
			if row.death {
				applied = s.Actors[1].HP == 0
			}
			expected := draw < row.chance/100 || row.death && draw == row.chance/100
			if applied != expected {
				t.Fatal(row, draw, s.Actors)
			}
		}
	}
	for _, npc := range []bool{false, true} {
		for _, row := range []struct {
			hp, maxHP, df int
			immune        bool
		}{{999, 999, 999, false}, {1000, 1000, 999, true}, {800, 1000, 999, npc}, {800, 999, 1000, true}} {
			s, c, d := npcStatusFixture()
			s.Actors[1].NPC = npc
			s.Actors[1].HP, s.Actors[1].MaxHP, s.Actors[1].BaseStats.Defense = row.hp, row.maxHP, row.df
			draws := []float64{0}
			if row.immune {
				draws = nil
			}
			executeNPCStatus(t, &s, c, d, 92, 0, []string{"enemy"}, draws...)
			if (s.Actors[1].Status == "") != row.immune {
				t.Fatal(npc, row, s.Actors)
			}
		}
	}
	// Damage can lower a player's current HP through the immunity boundary.
	s, c, d := npcStatusFixture()
	s.Actors[1].HP, s.Actors[1].MaxHP = 1000, 1000
	executeNPCStatus(t, &s, c, d, 90, 1, []string{"enemy"}, 0, 0)
	if s.Actors[1].HP != 978 || s.Actors[1].Status != corebattle.StatusDeadlyPoison {
		t.Fatal(s.Actors)
	}
}

func TestNPCStatusReductionGuardsAndUnconditionalPoison(t *testing.T) {
	for _, row := range []struct {
		maxHP, df int
		draw      float64
		hp        int
		poison    bool
	}{
		{999, 999, 0, 200, true}, {1000, 999, 0, 800, false}, {999, 1000, 0, 800, false},
		{999, 999, .499, 200, true}, {999, 999, .5, 800, false},
	} {
		s, c, d := npcStatusFixture()
		s.Actors[1].MaxHP, s.Actors[1].BaseStats.Defense = row.maxHP, row.df
		draws := []float64{row.draw}
		if row.maxHP > 999 || row.df > 999 {
			draws = nil
		}
		executeNPCStatus(t, &s, c, d, 93, 3, []string{"enemy"}, draws...)
		if s.Actors[1].HP != row.hp || (s.Actors[1].Status == corebattle.StatusDeadlyPoison) != row.poison {
			t.Fatal(row, s.Actors)
		}
	}
	for _, key := range []string{"null", "stone", "minimum", "hp-immune", "modifier-zero", "zero-tension", "fallen-cover"} {
		t.Run(key, func(t *testing.T) {
			s, c, d := npcStatusFixture()
			draws := []float64{0}
			target := 1
			switch key {
			case "null":
				s.Actors[1].Temporary = corebattle.TemporaryNullMagic
			case "stone":
				s.Actors[1].ItemDefinitionIDs = []string{"item-159"}
			case "minimum":
				s.Actors[1].HP = 199
			case "hp-immune":
				s.Actors[1].HP = 1000
			case "modifier-zero":
				s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
				c.DamageCalls = 1
			case "zero-tension":
				s.Actors[0].Tension = 0
			case "fallen-cover":
				target = 2
				s.Actors[1].Temporary = corebattle.TemporaryCover
				s.Actors[2].TeamID = "enemy"
				s.Actors[2].Temporary = corebattle.TemporaryCovering
				c.PartyIDs = []string{"self"}
				c.EnemyIDs = []string{"enemy", "fallen"}
			}
			hp := s.Actors[target].HP
			executeNPCStatus(t, &s, c, d, 93, 3, []string{"enemy"}, draws...)
			if s.Actors[target].HP != hp || s.Actors[target].Status != corebattle.StatusDeadlyPoison {
				t.Fatal(s.Actors, c)
			}
		})
	}
}

func TestNPCStatusNoEffectHelpersAndRest(t *testing.T) {
	s, c, d := npcStatusFixture()
	c.Item195Activated = true
	c.Arguments = []string{"self", "tail"}
	// The ally helper can activate confusion despite the body's absent writes.
	executeNPCStatus(t, &s, c, d, 88, 2, []string{"self", "tail"}, 0, 0)
	if s.Actors[0].Status != corebattle.StatusConfusion || len(c.Arguments) != 1 || c.Arguments[0] != "tail" {
		t.Fatal(s.Actors, c)
	}
	s, c, d = npcStatusFixture()
	c.Item195Activated = true
	c.Defeat = nil
	c.Death = nil
	s.Actors[0].Gear = nil
	executeNPCStatus(t, &s, c, d, 88, 3, []string{"self", "tail"})
	if len(c.Arguments) != 2 || s.Actors[0].Status != "" {
		t.Fatal(c, s.Actors)
	}
	for _, key := range []string{"normal", "magic-seal", "fallen", "no-effect"} {
		t.Run(key, func(t *testing.T) {
			s, c, d := npcStatusFixture()
			s.Actors[0].HP = 100
			s.Actors[0].MaxHP = 1000
			s.Fields.Default = "sanctuary" // Omitted category must not get magic's 1.2x.
			draws := []float64{0}
			want := 550
			switch key {
			case "magic-seal":
				s.Actors[0].Status = corebattle.StatusMagicSeal
			case "fallen":
				s.Actors[0].HP = 0
				draws = nil
				want = 0
			case "no-effect":
				s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
				c.DamageCalls = 1
				draws = nil
				want = 100
			}
			executeNPCStatus(t, &s, c, d, 94, 1, nil, draws...)
			if s.Actors[0].HP != want || s.Actors[0].Status != corebattle.StatusSleep {
				t.Fatal(fmt.Sprint(s.Actors))
			}
		})
	}
}
