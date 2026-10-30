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

func TestNPC98WholeEntryRejectionIsInert(t *testing.T) {
	for _, row := range []struct {
		name string
		pos  int
	}{
		{"symbolic-conversion", 0}, {"symbolic-conversion", 5},
		{"missing-reset", 1}, {"fallen-reset", 1}, {"item110", 1},
		{"requester-stat-tail", 3}, {"requester-tension", 4},
		{"missing-defeat", 6}, {"missing-defeat", 7}, {"missing-defeat", 8}, {"missing-defeat", 9}, {"missing-defeat", 10},
		{"later-hit", 7}, {"later-hit", 9},
	} {
		t.Run(fmt.Sprintf("%s/%d", row.name, row.pos), func(t *testing.T) {
			s, c, d := npc98Fixture()
			switch row.name {
			case "symbolic-conversion":
				s.Actors[0].Extended, c.Command = corebattle.ExtendedMagicSword, "attack"
			case "missing-reset":
				c.EquipmentReset = nil
			case "fallen-reset":
				s.Actors[2].Gear = nil
			case "item110":
				s.Actors[2].Gear.AccessoryID = "item-110"
			case "requester-stat-tail":
				s.Actors[2].HP, s.Actors[2].BaseStats.Defense = 1, 100
				s.Actors[2].ItemDefinitionIDs = []string{"item-205"}
			case "requester-tension":
				s.Actors[0].Tension = 3
			case "missing-defeat":
				c.Defeat = nil
			case "later-hit":
				s.Actors[1].Temporary, c.ToolCategory.Source = corebattle.TemporaryOneHitDefense, "unknown"
				if row.pos == 7 {
					// First breath can be blocked, then confusion can revisit that
					// original target after the unconditional vulnerability write.
					s.Actors[0].Status = corebattle.StatusConfusion
					s.Actors[0].Temporary, s.Actors[2].Temporary = corebattle.TemporaryOneHitDefense, corebattle.TemporaryOneHitDefense
					other := s.Actors[1]
					other.ID = "other"
					s.Actors = append(s.Actors, other)
					c.EnemyIDs = append(c.EnemyIDs, "other")
				}
			}
			c.Arguments = []string{"enemy", "tail"}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: append([]string(nil), c.Arguments...)}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			gear, args, party := s.Actors[1].Gear, c.Arguments, f.PartyIDs
			id := fmt.Sprintf("npc-job-98-position-%d", row.pos)
			if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyNPCJob98(c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.TensionCalls != 0 || c.Cursor != (corebattle.TargetCursor{}) || gear != s.Actors[1].Gear || !reflect.DeepEqual(args, c.Arguments) || !reflect.DeepEqual(f.Arguments, c.Arguments) || !reflect.DeepEqual(party, c.PartyIDs) || c.EquipmentReset != nil && c.EquipmentReset.Cursor != (corebattle.TargetCursor{}) {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestNPC98RegistrationWholePoolAndUnsupportedDescendants(t *testing.T) {
	s, c, d := npc98Fixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if len(d.Invocations.Entries) != 11 || len(d.Invocations.JobTables[98].Variants[0]) != 11 || len(d.JobSkills) != 0 {
		t.Fatal(d)
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{"npc-job-98-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	for _, id := range []string{"npc-job-98-position-11", "item-110", "gem-101", "gem-102"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	d.Invocations.Entries["current-job"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"current-job"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	// Either duplicate position is directly addressable by a naked invocation;
	// ordinary NPC name-to-last-binding remains the selector owner's contract.
	for _, pos := range []int{0, 5} {
		s, c, d := npc98Fixture()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, &npcJobRNG{t: t, draws: []float64{(float64(pos) + .5) / 11, 0}}); err != nil || s.Actors[1].HP != 1 || s.Actors[0].MP != 80 {
			t.Fatal(err, s)
		}
	}
	c.EquipmentReset = nil
	// A draw that could have selected any other entry cannot hide missing p1.
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	c.EquipmentReset = &corebattle.EquipmentResetContext{}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "npc-job-98-position-4"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Tension != 1 || s.Actors[0].Temporary != "" {
		t.Fatal(err, s)
	}
	d.Invocations.Entries["opaque"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute:  func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil },
	}
	if _, err := d.InvocationAction(&s, &f, []string{"opaque", "npc-job-98-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestNPC98NoEnemiesAndDamageNullification(t *testing.T) {
	for pos := range 11 {
		s, c, d := npc98Fixture()
		s.Actors[1].TeamID = "party"
		c.PartyIDs, c.EnemyIDs = append(c.PartyIDs, "enemy"), nil
		c.Defeat = nil
		var draws []float64
		switch pos {
		case 6:
			draws = []float64{0, 0, 0}
		case 9:
			draws = []float64{0}
		}
		executeNPC98(t, &s, c, d, pos, nil, draws...)
		wantHP := 1000
		if pos == 6 {
			wantHP = 999 // Healing retains its MaxHP clamp even above MaxHP.
		}
		if c.DamageCalls != 0 || s.Actors[1].HP != wantHP {
			t.Fatal(pos, s, c)
		}
	}
	// Magic nullification is accepted without a defeat port, while healing
	// still executes; accepted no-effect differs from whole-entry rejection.
	for _, pos := range []int{6, 9, 10} {
		s, c, d := npc98Fixture()
		s.Actors[1].Temporary, c.Defeat = corebattle.TemporaryNullMagic, nil
		draws := []float64(nil)
		if pos == 6 {
			draws = []float64{0, 0}
		} else if pos == 9 {
			draws = []float64{0, 0, 0, 0, 0}
		}
		executeNPC98(t, &s, c, d, pos, nil, draws...)
		if c.DamageCalls != 0 || s.Actors[1].HP != 1000 {
			t.Fatal(pos, s, c)
		}
	}
}
