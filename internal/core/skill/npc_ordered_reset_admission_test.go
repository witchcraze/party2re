package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestNPCOrderedWholeEntryAdmissionIsInert(t *testing.T) {
	for _, row := range []struct {
		name string
		pos  int
	}{
		{"missing-damage", 0}, {"requester-tail", 1}, {"later-hit", 2}, {"restored-tension", 2},
		{"fallen-ally-reset", 3}, {"reset-removes-defense", 3}, {"item110", 3},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, c, d := npcOrderedFixture()
			switch row.name {
			case "missing-damage":
				c.Defeat = nil
			case "requester-tail":
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			case "later-hit", "reset-removes-defense":
				s.Actors[1].Temporary, s.Fields.Default = corebattle.TemporaryOneHitDefense, "cosmic"
			case "restored-tension":
				s.Actors[0].Attack, s.Actors[0].Tension = 2, 5
				s.Actors[0].ItemDefinitionIDs, s.Fields.Default = []string{"item-166"}, "cosmic"
			case "fallen-ally-reset":
				s.Actors[2].Gear = nil
			case "item110":
				s.Actors[2].Gear.AccessoryID = "item-110"
			}
			c.Reset = func(string) error { t.Fatal("reset ran during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("defeat ran during admission"); return nil }
			if row.name == "missing-damage" {
				c.Defeat = nil
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			args, party, gear := f.Arguments, f.PartyIDs, s.Actors[1].Gear
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			id := fmt.Sprintf("npc-job-403-position-%d", row.pos)
			if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.EquipmentReset.Cursor != (corebattle.TargetCursor{}) || gear != s.Actors[1].Gear || gear.ArmorDefense != 0 || !reflect.DeepEqual(args, []string{"enemy", "tail"}) || !reflect.DeepEqual(party, []string{"self", "fallen"}) {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestNPCOrderedRegistrationAndDescendants(t *testing.T) {
	s, c, d := npcOrderedFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if len(d.Invocations.Entries) != 4 || len(d.Invocations.JobTables[403].Variants[0]) != 4 || len(d.JobSkills) != 0 {
		t.Fatal(d)
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{"npc-job-403-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	for _, id := range []string{"npc-job-403-position-4", "item-110", "gem-101", "gem-102"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	d.Invocations.Entries["current-job"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"current-job"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	// All four original positions remain in the pool even if the selected
	// draw would have avoided the missing reset descendant.
	c.EquipmentReset = nil
	if _, err := d.InvocationAction(&s, &f, []string{"current-job"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "npc-job-403-position-1"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Defense != 80 || s.Actors[0].Temporary != "" {
		t.Fatal(err, s)
	}
	d.Invocations.Entries["opaque"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute:  func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil },
	}
	if _, err := d.InvocationAction(&s, &f, []string{"opaque", "npc-job-403-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestNPCOrderedNoEnemyAndDeadCaster(t *testing.T) {
	for pos := range 4 {
		s, c, d := npcOrderedFixture()
		s.Actors[1].TeamID = "party"
		c.PartyIDs = append(c.PartyIDs, "enemy")
		c.EnemyIDs = nil
		c.Defeat = nil
		executeNPCOrdered(t, &s, c, d, pos, nil)
		if c.DamageCalls != 0 || s.Actors[0].MP != 80 || s.Actors[1].HP != 1000 {
			t.Fatal(pos, s, c)
		}
	}
	for _, pos := range []int{0, 2} {
		s, c, d := npcOrderedFixture()
		s.Actors[0].HP, s.Actors[0].Tension = 0, 5
		c.Defeat = nil
		executeNPCOrdered(t, &s, c, d, pos, []string{"enemy"})
		if c.DamageCalls != 0 || pos == 2 && s.Actors[0].Tension != 1 {
			t.Fatal(pos, s, c)
		}
	}
}
