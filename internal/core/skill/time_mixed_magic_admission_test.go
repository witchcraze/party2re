package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestTimeMixedMagicAdmissionIsInert(t *testing.T) {
	_, _, catalog := timeMagicFixture()
	if len(catalog.Invocations.Entries) != 21 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := timeMagicFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon := 7
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			c.Reset = func(string) error { t.Fatal("reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
			c.Damage = func(corebattle.TargetContext, corebattle.TargetEffect, random.Generator) error {
				t.Fatal("stat tail during admission")
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
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.HealCalls != 0 || c.Arguments != nil || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
				t.Fatal(id, s, c, f)
			}
		}
	}
}

func TestTimeMixedMagicMissingDescendantsRejectBeforeMutation(t *testing.T) {
	for _, row := range []string{"damage", "defeat", "reset", "agility-tail", "stat-tension", "later-hit", "activation", "unknown-later", "unsupported-later-field"} {
		t.Run(row, func(t *testing.T) {
			s, c, d := timeMagicFixture()
			ids, target := []string{"job-30-entry-8"}, "missing"
			switch row {
			case "damage":
				c.Defeat = nil
			case "defeat":
				ids[0], c.Defeat = "job-31-entry-1", nil
			case "reset":
				s.Actors[0].Extended = corebattle.ExtendedPurification
			case "agility-tail":
				ids[0], target = "job-29-entry-1", "self"
				s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
			case "stat-tension":
				ids[0] = "job-31-entry-3"
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			case "later-hit":
				s.Actors[1].Temporary, c.Defeat = corebattle.TemporaryOneHitDefense, nil
			case "activation":
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
			case "unknown-later":
				ids = append(ids, "unknown")
			case "unsupported-later-field":
				ids = append(ids, "job-29-entry-7")
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{target, "tail"}}
			ops := make([]corebattle.CallbackInvocation, len(ids))
			for i, id := range ids {
				ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
			}
			if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.Arguments != nil || f.Arguments[0] != target {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestTimeMixedMagicDefaultsSeparateOwnersAndRegistrationOrder(t *testing.T) {
	s, c, d := timeMagicFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missing BattleDefinitions
	missing.RegisterTimeMixedMagic(nil)
	for _, empty := range []BattleDefinitions{{}, missing} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-30-entry-1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, reverse := range []bool{false, true} {
		var combined BattleDefinitions
		arithmetic := &corebattle.ArithmeticContext{TargetContext: c.TargetContext, Defeat: c.Defeat}
		if reverse {
			combined.RegisterArithmetic(arithmetic)
		}
		combined.RegisterTimeMixedMagic(c)
		if !reverse {
			combined.RegisterArithmetic(arithmetic)
		}
		if !reflect.DeepEqual(combined.Invocations.JobTables[31], d.Invocations.JobTables[31]) || len(combined.Invocations.Entries) != 32 {
			t.Fatal(combined)
		}
		if _, err := combined.InvocationAction(&s, &f, []string{"job-31-gravity"}, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
		if _, err := combined.InvocationAction(&s, &f, []string{"job-31-entry-1"}, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.InvocationAction(&s, &f, []string{"job-31-gravity"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"job-29-entry-7", "item-110", "gem-101", "gem-102"} {
		if _, err := d.InvocationAction(&s, &f, []string{forbidden}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(forbidden, err)
		}
	}
}
