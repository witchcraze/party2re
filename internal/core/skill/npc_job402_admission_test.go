package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPC402AdmissionRejectsBeforeAnyMutation(t *testing.T) {
	for _, row := range []struct {
		name string
		pos  int
	}{
		{"later-target", 0}, {"later-hit", 0}, {"requester", 1}, {"agility-hook", 1},
		{"physical-descendant", 2}, {"lethal-reset", 2}, {"later-hit", 3},
		{"cleared-defense", 4}, {"shared-mana", 5}, {"item110-reset", 0}, {"later-lethal", 0},
	} {
		t.Run(fmt.Sprintf("%s/%d", row.name, row.pos), func(t *testing.T) {
			s, c, d := npc402Fixture()
			args := []string{"enemy", "enemy", "enemy"}
			switch row.name {
			case "later-target":
				other := s.Actors[1]
				other.ID, other.Gear = "other", nil
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, "other")
				args[1] = "other"
			case "later-hit":
				c.ToolCategory.Source, s.Actors[1].Temporary = "unknown", corebattle.TemporaryOneHitDefense
			case "requester":
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			case "agility-hook":
				s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
			case "physical-descendant":
				s.Actors[1].Temporary, c.Death = corebattle.TemporaryNullMagic, nil
				s.Actors[1].HP = 1
			case "lethal-reset":
				s.Actors[1].HP = 1
				c.Death = nil
			case "cleared-defense":
				s.Actors[1].Temporary, c.ToolCategory.Source = corebattle.TemporaryOneHitDefense, "unknown"
			case "shared-mana":
				s.Actors[2].MP, s.Actors[1].HP = 5, 1
				c.Death = nil
			case "item110-reset":
				s.Actors[1].HP, s.Actors[1].Gear.AccessoryID = 1, "item-110"
			case "later-lethal":
				s.Actors[1].HP, s.Actors[1].Gear.AccessoryID = 150, "item-110"
			}
			c.Arguments = append([]string(nil), args...)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			gear, arguments, party := *s.Actors[0].Gear, c.Arguments, c.PartyIDs
			id := fmt.Sprintf("npc-job-402-position-%d", row.pos)
			for _, validate := range []func() error{
				func() error { _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); return err },
				func() error {
					return s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t})
				},
				func() error { return s.ApplyNPCJob402(c, id, noArithmeticDraws{t}) },
			} {
				if err := validate(); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(gear, *s.Actors[0].Gear) || !reflect.DeepEqual(arguments, c.Arguments) || !reflect.DeepEqual(party, c.PartyIDs) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.Cursor != (corebattle.TargetCursor{}) {
				t.Fatal(s, c)
			}
		})
	}
}

func TestNPC402AdmissionSuccessIsPureAndNoEffectIsAccepted(t *testing.T) {
	for pos := range 6 {
		s, c, d := npc402Fixture()
		s.Actors[2].MP = 5
		c.Arguments = []string{"enemy", "enemy", "enemy"}
		c.Death.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
			t.Fatal("admission executed reward")
			return nil
		}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		args := c.Arguments
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: c.Arguments}
		if _, err := d.InvocationAction(&s, &f, []string{fmt.Sprintf("npc-job-402-position-%d", pos)}, noArithmeticDraws{t}); err != nil {
			t.Fatal(pos, err)
		}
		after, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || !reflect.DeepEqual(args, c.Arguments) || c.DamageCalls != 0 || c.PartyStatUpCalls != 0 {
			t.Fatal(s, c)
		}
		// Complete empty-enemy contexts do not need a defeat/reset port.
		s.Actors[1].TeamID = "party"
		c.PartyIDs, c.EnemyIDs, c.Death = append(c.PartyIDs, "enemy"), nil, nil
		var draws []float64
		if pos == 2 {
			draws = []float64{0, 0}
		}
		executeNPC402(t, &s, c, d, pos, nil, draws...)
		if c.DamageCalls != 0 || s.Actors[1].HP != 1000 {
			t.Fatal(pos, s, c)
		}
	}
	// Three hits stay nonlethal here; an unreachable defeat port is unnecessary.
	s, c, d := npc402Fixture()
	c.Death = nil
	executeNPC402(t, &s, c, d, 0, []string{"enemy", "enemy", "enemy"}, 0, 0, 0)
	if s.Actors[1].HP != 703 {
		t.Fatal(s)
	}
}

func TestNPC402WholePoolRegistrationAndUnknownDescendants(t *testing.T) {
	s, c, d := npc402Fixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if len(d.Invocations.Entries) != 6 || len(d.JobSkills) != 0 {
		t.Fatal(d)
	}
	for _, id := range []string{"npc-job-402-position-6", "item-110", "gem-101", "gem-102"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{"npc-job-402-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "npc-job-402-position-1"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Attack != 100 {
		t.Fatal(err, s)
	}
	// Original current-job random invocation visits all six positions, including
	// SP100 entries when actor SP0: bare invocation does not inherit NPC gates.
	s.Actors[0].Jobs.SP = 0
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, &npcJobRNG{t: t, draws: []float64{5.5 / 6, 0, 0}}); err != nil || s.Actors[1].HP != 949 || s.Actors[1].CMPNeedCharge != 6 {
		t.Fatal(err, s)
	}
	// A missing descendant rejects the entire pool, regardless of selected index.
	s, c, d = npc402Fixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
