package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestBeastCloneAdmissionIsInert(t *testing.T) {
	_, _, catalog := beastCloneFixture()
	if len(catalog.Invocations.Entries) != 26 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := beastCloneFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon, history := 7, 9
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			s.Actors[0].JobHistory = map[string]*int{"job-10": &history}
			c.Arguments = []string{"original", "tail"}
			c.Reset = func(string) error { t.Fatal("external reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("external defeat during admission"); return nil }
			c.StatusDefeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
				t.Fatal("external settlement during admission")
				return nil
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			original := f
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
			if (err != nil) != reject {
				t.Fatal(id, reject, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || c.DamageCalls != 0 || c.StatusCalls != 0 || c.DeathCalls != 0 || c.StatUpCalls != 0 || c.HealCalls != 0 || c.End.ImmediateRiskCalls != 0 || !reflect.DeepEqual(c.Arguments, []string{"original", "tail"}) || weapon != 7 || history != 9 || *c.RequesterLevel != 40 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear || s.Actors[0].JobHistory["job-10"] != &history {
				t.Fatal(id, s, c, f)
			}
		}
	}
}

func TestBeastCloneMissingDescendantsRejectBeforeMutation(t *testing.T) {
	for _, bad := range []string{"damage", "second-hit", "later-status", "death", "self-buff", "party-buff", "requester", "summon-sequence", "conversion", "unknown-later", "unknown-transition"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := beastCloneFixture()
			ids := []string{"job-10-entry-0"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "second-hit":
				ids[0] = "job-12-entry-7"
				s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				s.Actors[1].Gear = nil
			case "later-status":
				ids[0] = "job-12-entry-4"
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
			case "death":
				ids[0] = "job-26-entry-5"
				c.StatusDefeat = nil
			case "self-buff":
				ids[0] = "job-10-entry-7"
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
				c.RequesterLevel = nil
			case "party-buff":
				ids[0] = "job-26-entry-4"
				s.Actors[2].HP = 100
				s.Actors[2].BaseStats.Agility = 100
				s.Actors[2].ItemDefinitionIDs = []string{"item-201"}
			case "requester":
				ids[0] = "job-26-entry-8"
				c.RequesterLevel = nil
			case "summon-sequence":
				ids[0] = "job-26-entry-8"
				s.RosterSequence = 1e15
			case "conversion":
				ids[0] = "job-12-entry-4"
				s.Actors[0].Abilities = []string{"seal_kotowari"}
			case "unknown-later":
				ids = append(ids, "unknown")
			case "unknown-transition":
				ids = []string{"job-26-entry-8", "job-10-entry-8"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			ops := make([]corebattle.CallbackInvocation, len(ids))
			for i, id := range ids {
				ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
			}
			err = s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t})
			if err == nil {
				t.Fatal("missing descendant admitted", bad)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatusCalls != 0 || c.StatUpCalls != 0 || c.Arguments != nil {
				t.Fatal(s, c)
			}
		})
	}
}

func TestBeastCloneEmptyRegistrationAndFullPools(t *testing.T) {
	s, c, d := beastCloneFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missingContext BattleDefinitions
	missingContext.RegisterBeastClone(nil)
	for _, empty := range []BattleDefinitions{{}, missingContext} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-26-entry-8"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	if _, err := d.InvocationAction(&s, &f, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("must not narrow general-random pool", err)
	}
	s.Actors[0].Jobs.JobID = "job-26"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	delete(d.Invocations.Entries, "job-26-entry-8")
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("missing extended child must reject tool pool", err)
	}
}
