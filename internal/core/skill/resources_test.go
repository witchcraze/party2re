package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestResourceRegistrationAndBoundaries(t *testing.T) {
	want := map[string][3]int{
		"gem_mp_heal_sky_1": {24, 24, 146}, "gem_mp_heal_sky_2": {25, 37, 191},
		"gem_drain_mp_sky": {27, 42, 119}, "gem_debuff_mp": {97, 17, 100},
		"gem_mp_burst": {98, 0, 0}, "gem_cmp_burst": {99, 1, 100},
		"gem_status_cmp_3": {103, -3, 100}, "gem_status_cmp_10": {104, -10, 100},
		"job-50-entry-5": {0, 0, 146},
	}
	if len(corebattle.ResourceEntries()) != len(want) {
		t.Fatal("entry coverage")
	}
	for _, entry := range corebattle.ResourceEntries() {
		t.Run(entry.ID, func(t *testing.T) {
			s, base := recoveryGemFixture(false)
			for i := range s.Actors {
				s.Actors[i].MaxMP = 1000
				s.Actors[i].Gear = &corebattle.BattleGearStats{}
				if s.Actors[i].ID == "enemy" {
					s.Actors[i].HP = 10000
					s.Actors[i].MaxMP = 100
				}
			}
			c := corebattle.DamageContext{EffectContext: base, GuaranteedHit: true}
			c.Defeat = func(string) error { return nil } // High-HP fixture cannot reach defeat.
			var d BattleDefinitions
			d.RegisterResources(&c)
			if len(d.Invocations.JobTables) != 0 || len(d.Gems) != 0 {
				t.Fatal("partial table or scalar effects")
			}
			w, ok := want[entry.ID]
			if !ok || entry.SourceID != w[0] || entry.CMPCost != w[1] {
				t.Fatal(entry, w)
			}
			if entry.ID == "job-50-entry-5" && (entry.RequiredSP != 110 || entry.MPCost != 50) {
				t.Fatal(entry)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
			before := append([]corebattle.LifetimeActor(nil), s.Actors...)
			ops, err := d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t})
			if err != nil || !reflect.DeepEqual(before, s.Actors) {
				t.Fatal(err, s)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, recoveryGemRNG{}); err != nil {
				t.Fatal(err)
			}
			if s.Actors[0].MP != w[2] || s.Actors[0].HP != 100 {
				t.Fatal(entry, s.Actors)
			}
			if entry.SourceID == 99 && s.Actors[0].CMP != 1 {
				t.Fatal(s.Actors)
			}
		})
	}
}

func TestResourceToolsRestoreOnlyMP(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int
	}{{"tool-11", 37}, {"tool-12", 91}, {"tool-13", 900}, {"tool-101", 46}} {
		s, c, d := ordinaryToolFixture()
		s.Actors[0].MP, s.Actors[0].MaxMP = 0, 1000
		hp := s.Actors[0].HP
		r := executeOrdinaryTool(t, &s, c, d, tc.id, []string{"self"}, 0)
		if s.Actors[0].MP != tc.want || s.Actors[0].HP != hp || !r.Consume {
			t.Fatal(tc, s.Actors, r)
		}
	}
}

func TestResourceRandomWrapperRestoresDecreaseRetainsIncrease(t *testing.T) {
	for _, tc := range []struct{ mp, want int }{{80, 80}, {0, 55}} {
		s, base := recoveryGemFixture(false)
		s.Fields.Default = "sanctuary"
		s.Actors[0].MP, s.Actors[0].MaxMP = tc.mp, 100
		s.Actors[0].Jobs.JobID = "50"
		base.Custom = false
		c := corebattle.DamageContext{EffectContext: base}
		var d BattleDefinitions
		d.RegisterResources(&c)
		// Only the invocation boundary is under test; production job50 assembly
		// requires its complete table from the table owner.
		entry := corebattle.ResourceEntries()[8].CallbackEntry
		d.Invocations.JobTables = map[int]corebattle.CallbackTable{50: {Variants: [][]corebattle.CallbackEntry{{entry}}}}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "tool-84"}}, recoveryGemRNG{}); err != nil || s.Actors[0].MP != tc.want {
			t.Fatal(err, s.Actors)
		}
	}
}

func TestResourceCMPOrderedChargeAndReload(t *testing.T) {
	for _, npc := range []bool{false, true} {
		s, base := recoveryGemFixture(npc)
		s.Actors[0].CMP = 10
		c := corebattle.DamageContext{EffectContext: base}
		var d BattleDefinitions
		d.RegisterResources(&c)
		// An existing owner can project an earlier state/resource change.
		change := func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext) (bool, error) {
			s.Actors[0].Status = corebattle.StatusPoison
			s.Actors[0].CMP = 9
			return true, nil
		}
		d.Invocations.Entries["earlier"] = corebattle.CallbackDefinition{Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil }, Project: change, Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, _ random.Generator) error {
			_, err := change(s, f)
			return err
		}}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		ids := []string{"earlier", "gem_status_cmp_3", "gem_status_cmp_10"}
		before, _ := json.Marshal(s)
		ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		after, _ := json.Marshal(s)
		if err != nil || string(before) != string(after) {
			t.Fatal(err, "impure projection")
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].CMP != 0 || s.Actors[0].Status != corebattle.StatusPoison {
			t.Fatal(err, s.Actors)
		}
		// The consumer charges the negative outer contribution AFTER all slots.
		s.Actors[0].CMP -= -13
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var loaded corebattle.LifetimeState
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		if loaded.Actors[0].CMP != 13 {
			t.Fatal("repeated resource gains", loaded.Actors)
		}
		var rebuilt BattleDefinitions
		rebuilt.RegisterResources(&c)
		if loaded.Actors[0].CMP != 13 || len(rebuilt.Invocations.Entries) != 9 {
			t.Fatal(loaded.Actors)
		}
	}
}

func TestResourceRejectsUnsupportedClosureBeforeEffects(t *testing.T) {
	s, base := recoveryGemFixture(false)
	c := corebattle.DamageContext{EffectContext: base}
	var d BattleDefinitions
	d.RegisterResources(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	before, _ := json.Marshal(s)
	for _, ids := range [][]string{{"gem_mp_heal_sky_1", "missing"}, {"gem_mp_heal_sky_1", "gem_mp_burst"}} {
		_, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s)
		if string(before) != string(after) {
			t.Fatal("admission mutated state")
		}
	}
}

func TestResourceBurstRejectsMissingDamageContextBeforeEffects(t *testing.T) {
	s, base := recoveryGemFixture(false)
	c := corebattle.DamageContext{EffectContext: base}
	var d BattleDefinitions
	d.RegisterResources(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"gem_mp_burst", "gem_cmp_burst"} {
		_, err := d.InvocationAction(&s, &f, []string{"gem_status_cmp_3", id}, noArithmeticDraws{t})
		if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		if _, err := s.ApplyResource(&c, id, "", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || c.DamageCalls != 0 {
		t.Fatal("partial resource execution", s, c)
	}
}
