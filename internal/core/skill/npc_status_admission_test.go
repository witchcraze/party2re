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

func TestNPCStatusRejectsBeforeMutation(t *testing.T) {
	for _, row := range []struct {
		job, pos int
		failure  string
	}{
		{88, 0, "target-gear"}, {88, 1, "actor-gear"}, {88, 2, "missing-confused-context"},
		{90, 0, "later-symbol"}, {90, 1, "tool-provenance"}, {90, 2, "target-gear"},
		{91, 0, "later-symbol"}, {91, 1, "later-symbol"}, {91, 2, "target-gear"},
		{92, 0, "target-gear"}, {92, 1, "later-symbol"}, {92, 2, "target-gear"},
		{93, 0, "defeat"}, {93, 1, "defeat"}, {93, 2, "defeat"}, {93, 3, "symbol-conversion"},
		{94, 0, "later-reset"}, {94, 0, "last-fortress-tail"}, {94, 1, "invalid-heal"},
	} {
		t.Run(fmt.Sprintf("%d/%d/%s", row.job, row.pos, row.failure), func(t *testing.T) {
			s, c, d := npcStatusFixture()
			args := []string{"enemy", "tail"}
			switch row.failure {
			case "target-gear":
				s.Actors[1].Gear = nil
			case "actor-gear":
				s.Actors[0].Gear = nil
			case "missing-confused-context":
				s.SpecialBossField = true
				s.Actors[0].Status = corebattle.StatusConfusion
				args = nil
			case "later-symbol":
				s.Actors[0].Abilities = []string{"seal_kotowari"}
			case "tool-provenance":
				c.ToolCategory.Source = "unknown"
			case "defeat":
				c.Death = nil
			case "symbol-conversion":
				s.Actors[0].Extended = corebattle.ExtendedMagicSword
				c.Command = "attack"
			case "later-reset":
				s.Actors[0].Gear.AccessoryID = "item-110" // Earlier enemy death is supported.
			case "last-fortress-tail":
				s.Actors[0].NPC, c.ActorNPC = false, false
				neutral := func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil }
				c.Death.Rewards = &corebattle.DefeatRewards{Validate: neutral, Apply: neutral}
				c.Death.FieldAtLoad = "fortress"
				c.Death.Fortress = &corebattle.DefeatRewards{Validate: neutral, Apply: neutral}
			case "invalid-heal":
				s.Actors[0].MaxHP = -1
			}
			c.Arguments = args
			c.Defeat = func(string) error { t.Fatal("rejected callback invoked defeat"); return nil }
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			contextBefore := *c
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			id := fmt.Sprintf("npc-job-%d-position-%d", row.job, row.pos)
			if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyNPCStatus(c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(c.Arguments, args) || !reflect.DeepEqual(f.Arguments, args) || c.EffectContext.DamageCalls != contextBefore.DamageCalls || c.StatusCalls != 0 || c.Cursor != (corebattle.TargetCursor{}) || c.Item195Activated != contextBefore.Item195Activated || c.Command != contextBefore.Command {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestNPCStatusRegistrationAndIndirectAdmission(t *testing.T) {
	s, c, d := npcStatusFixture()
	if len(d.Invocations.Entries) != 19 || len(d.Invocations.JobTables) != 6 || len(d.JobSkills) != 0 {
		t.Fatal(d)
	}
	for _, job := range []int{88, 90, 91, 92, 93, 94} {
		if !reflect.DeepEqual(d.Invocations.JobTables[job].Variants[0], corebattle.NPCJobEntries(job)) {
			t.Fatal(job)
		}
	}
	eligible := 0
	for _, entry := range corebattle.NPCJobEntries(88) {
		if entry.RequiredSP <= 20 {
			eligible++
		}
	}
	if eligible != 2 {
		t.Fatal(eligible)
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	for _, id := range []string{"npc-job-88-position-4", "npc-job-89-position-0", "item-110", "tool-110", "gem-101", "gem-102", "job-88-entry-0"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{"npc-job-90-position-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	s.Actors[0].Jobs.JobID = "job-93"
	c.Death = nil
	// Whole-pool validation must retain the missing instant-death descendants,
	// even if the attempted random draw could have selected life reduction.
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.Invocations.Entries["opaque"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			t.Fatal("executed before tail admission")
			return nil
		},
	}
	if _, err := d.InvocationAction(&s, &f, []string{"opaque", "npc-job-94-position-1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if _, err := d.InvocationAction(&s, &f, []string{"npc-job-88-position-3", "missing"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
