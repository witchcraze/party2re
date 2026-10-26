package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestWeaponWeatherStanceAdmissionIsInert(t *testing.T) {
	_, _, catalog := weaponWeatherStanceFixture()
	if len(catalog.Invocations.Entries) != 21 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := weaponWeatherStanceFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon := 7
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			c.Reset = func(string) error { t.Fatal("external reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("external defeat during admission"); return nil }
			c.StatusDefeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
				t.Fatal("external rewards during admission")
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
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || c.Arguments != nil || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.StatDownCalls != 0 || c.HealCalls != 0 || c.PartyHealCalls != 0 || c.CureCalls != 0 || c.StatusCalls != 0 || c.DeathCalls != 0 || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
				t.Fatal(id, s, c, f)
			}
		}
	}
}

func TestWeaponWeatherStanceRejectsMissingDescendantsBeforeMutation(t *testing.T) {
	for _, bad := range []string{"damage", "reset", "status", "death", "later-area", "unknown-later", "unknown-after-damage"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := weaponWeatherStanceFixture()
			ids := []string{"job-24-entry-0"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "reset":
				s.Actors[0].Extended = corebattle.ExtendedPurification
			case "status":
				ids[0], s.Actors[1].Gear = "job-28-entry-1", nil
			case "death":
				ids[0], c.StatusDefeat = "job-28-entry-6", nil
			case "later-area":
				ids[0] = "job-27-entry-3"
				other := s.Actors[1]
				other.ID, other.Gear = "other", nil
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, "other")
			case "unknown-later":
				ids = []string{"job-28-entry-0", "job-28-entry-3"}
			case "unknown-after-damage":
				ids = append(ids, "job-27-entry-3")
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			original := f
			ops := make([]corebattle.CallbackInvocation, len(ids))
			for i, id := range ids {
				ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(f, original) || c.DamageCalls != 0 || c.StatusCalls != 0 || c.Arguments != nil {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestWeaponWeatherStanceDefaultEmptyAndWalletOwner(t *testing.T) {
	s, c, d := weaponWeatherStanceFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missing BattleDefinitions
	missing.RegisterWeaponWeatherStance(nil)
	for _, empty := range []BattleDefinitions{{}, missing} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-24-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	s.Actors[0].Jobs.JobID = "job-28"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	called := false
	wallet := corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			called = true
			return nil
		},
	}
	d.Invocations.Entries["job-28-entry-3"] = wallet
	d.RegisterWeaponWeatherStance(c)
	executeWeaponWeatherStance(t, &s, c, d, "tool", nil, 3.5/7)
	if !called || len(d.Invocations.JobTables[28].Variants[0]) != 7 || s.Actors[0].MP != 80 {
		t.Fatal(called, s)
	}
}
