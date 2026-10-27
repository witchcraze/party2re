package skill

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPCStatusSacrificeRevivalThenOuterCharge(t *testing.T) {
	for _, row := range []struct {
		kind  string
		hp    int
		draws []float64
	}{
		{"none", 0, nil}, {"pharaoh", 999, nil}, {"amulet", 499, nil}, {"temporary", 99, []float64{0}},
	} {
		t.Run(row.kind, func(t *testing.T) {
			s, c, d := npcStatusFixture()
			// The area helper accepts no enemies; self defeat and MP assignment follow.
			s.Actors[1].TeamID = "party"
			c.PartyIDs = append(c.PartyIDs, "enemy")
			c.EnemyIDs = nil
			s.Actors[0].MaxMP = 100
			switch row.kind {
			case "pharaoh":
				s.Actors[0].Abilities = []string{"pharaoh"}
			case "amulet":
				s.Actors[0].ItemDefinitionIDs = []string{"item-193"}
				s.Actors[0].Gear.AccessoryID = "item-193"
			case "temporary":
				s.Actors[0].Temporary = corebattle.TemporaryRevival
			}
			executeNPCStatus(t, &s, c, d, 94, 0, nil, row.draws...)
			if s.Actors[0].HP != row.hp || s.Actors[0].MP != 1 || c.AreaDeathCalls != 1 {
				t.Fatal(s.Actors, c)
			}
			if row.kind == "amulet" && (s.Actors[0].Status != corebattle.StatusDofuu || len(s.Actors[0].ConsumedItems) != 1) {
				t.Fatal(s.Actors)
			}
			// _npc_action.cgi:317–318 is an outer listed debit, after internal MP1.
			cost := d.Invocations.JobTables[94].Variants[0][0].MPCost
			s.Actors[0].MP = max(0, s.Actors[0].MP-cost)
			if s.Actors[0].MP != 0 {
				t.Fatal(s.Actors[0])
			}
		})
	}
}

func TestNPCStatusAreaDeathContinuesAfterActorDefeat(t *testing.T) {
	s, c, d := npcStatusFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].HP = 500
	s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
	executeNPCStatus(t, &s, c, d, 93, 1, nil, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[1].HP != 800 || s.Actors[3].HP != 0 || c.DeathCalls != 2 {
		t.Fatal(s.Actors, c)
	}
}

func TestNPCStatusSacrificeSecondDefeatAfterConsumedRevival(t *testing.T) {
	s, c, d := npcStatusFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].HP = 500
	s.Actors[0].Status = corebattle.StatusConfusion
	s.Actors[0].ItemDefinitionIDs = []string{"item-193"}
	s.Actors[0].Gear.AccessoryID = "item-193"
	// First area death resolves to self and consumes the amulet. The second
	// enemy death then precedes the unconditional, now unprotected self defeat.
	executeNPCStatus(t, &s, c, d, 94, 0, nil, 0, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].MP != 1 || s.Actors[1].HP != 800 || s.Actors[3].HP != 0 || len(s.Actors[0].ConsumedItems) != 1 {
		t.Fatal(s.Actors, c)
	}
}
