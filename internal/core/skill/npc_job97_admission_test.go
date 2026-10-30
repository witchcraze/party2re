package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC97WholeEntryRejectionIsInert(t *testing.T) {
	for _, row := range []struct {
		name string
		pos  int
	}{
		{"missing-defeat", 0}, {"missing-defeat", 1}, {"missing-defeat", 4}, {"missing-defeat", 5}, {"missing-defeat", 7}, {"missing-defeat", 8}, {"missing-defeat", 10},
		{"missing-reset", 2}, {"fallen-reset", 2}, {"item110", 2}, {"missing-requester", 6},
		{"later-hit", 0}, {"later-hit", 1}, {"later-status", 8}, {"later-status", 10}, {"symbolic-conversion", 9},
	} {
		t.Run(fmt.Sprintf("%s/%d", row.name, row.pos), func(t *testing.T) {
			s, c, d := npc97Fixture()
			switch row.name {
			case "missing-defeat":
				c.Defeat = nil
			case "missing-reset":
				c.EquipmentReset = nil
			case "fallen-reset":
				s.Actors[2].Gear = nil
			case "item110":
				s.Actors[2].Gear.AccessoryID = "item-110"
			case "missing-requester":
				s.Actors[0].Tension = 3
			case "later-hit":
				s.Actors[1].Temporary, c.ToolCategory.Source = corebattle.TemporaryOneHitDefense, "unknown"
			case "later-status":
				// Numeric magic is nullified by item166. The status tail still
				// requires the living target's gear, despite no damage descendant.
				s.Actors[0].ItemDefinitionIDs = []string{"item-166"}
				s.Actors[1].HP, s.Actors[1].Gear = 800, nil
			case "symbolic-conversion":
				s.Actors[0].Extended, c.Command = corebattle.ExtendedMagicSword, "attack"
			}
			c.Arguments = []string{"enemy", "tail"}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: append([]string(nil), c.Arguments...)}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			contextBefore := *c
			gear := s.Actors[0].Gear
			id := fmt.Sprintf("npc-job-97-position-%d", row.pos)
			if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyNPCJob97(c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(c.Arguments, contextBefore.Arguments) || !reflect.DeepEqual(f.Arguments, contextBefore.Arguments) || c.DamageCalls != 0 || c.StatusCalls != 0 || c.StatUpCalls != 0 || c.TensionCalls != 0 || c.AdditionalEffect || c.Cursor != contextBefore.Cursor || gear != s.Actors[0].Gear || c.EquipmentReset != nil && c.EquipmentReset.Cursor != (corebattle.TargetCursor{}) {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestNPC97WholePoolRegistrationAndNoEffect(t *testing.T) {
	s, c, d := npc97Fixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if len(d.Invocations.Entries) != 11 || len(d.Invocations.JobTables[97].Variants[0]) != 11 || len(d.JobSkills) != 0 {
		t.Fatal(d)
	}
	for _, id := range []string{"npc-job-97-position-11", "item-110", "gem-101", "gem-102"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{"npc-job-97-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.Invocations.Entries["current-job"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"current-job"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	// The full random pool is admitted before selection, including reset p2.
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, &npcJobRNG{t: t, draws: []float64{6.5 / 11}}); err != nil || s.Actors[0].Tension != 3 {
		t.Fatal(err, s)
	}
	s, c, d = npc97Fixture()
	c.EquipmentReset = nil
	f = corebattle.CallbackContext{TargetContext: c.TargetContext}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	c.EquipmentReset = &corebattle.EquipmentResetContext{}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "npc-job-97-position-6"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Temporary != "" || s.Actors[0].Tension != 1 {
		t.Fatal(err, s)
	}
	// Accepted no-target paths still execute count rolls and unconditional tails.
	for pos := range 11 {
		s, c, d := npc97Fixture()
		s.Actors[1].TeamID = "party"
		c.PartyIDs, c.EnemyIDs, c.Defeat = append(c.PartyIDs, "enemy"), nil, nil
		var draws []float64
		if pos == 0 || pos == 1 {
			draws = []float64{0}
		}
		executeNPC97(t, &s, c, d, pos, nil, draws...)
		if c.DamageCalls != 0 || s.Actors[1].HP != 1000 {
			t.Fatal(pos, s.Actors, c)
		}
	}
}
