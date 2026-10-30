package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNamedFieldRegistrationKeepsTablesAndWholeEntryGates(t *testing.T) {
	s, base, _ := dualAgilityFixture()
	c := &corebattle.NamedFieldContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterRecoilDarkState(&corebattle.RecoilDarkStateContext{DamageContext: base.DamageContext})
	d.RegisterTreatmentMischief(nil)
	before := d.Invocations.JobTables[22]
	d.RegisterNamedFields(c)
	if !reflect.DeepEqual(before, d.Invocations.JobTables[22]) {
		t.Fatal("partial field manifest replaced complete table")
	}
	for _, entry := range corebattle.NamedFieldEntries() {
		if d.Invocations.Entries[entry.ID].Execute == nil || entry.MPCost < 1 {
			t.Fatal(entry)
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(entry, err)
		}
	}
	// Later missing/custom descendants reject before the first direct write.
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	old, _ := json.Marshal(s)
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-56-entry-3"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	current, _ := json.Marshal(s)
	if string(old) != string(current) {
		t.Fatal(s)
	}
	// General random still needs every original table and successful branch.
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	if _, err := d.InvocationAction(&s, &f, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestNamedFieldDarknessUsesSubmittedDiscoveredBindingAndSharesFacts(t *testing.T) {
	for _, row := range []struct {
		selected         string
		excluded, reject bool
	}{
		{"job-22-entry-0", false, false}, {"job-22-entry-0", true, false}, {"missing", false, true},
	} {
		s, base, _ := dualAgilityFixture()
		s.Actors[1].HP = 10000
		s.Actors[0].HP, s.Actors[0].BaseStats.Attack = 500, 100
		c := &corebattle.NamedFieldContext{DamageContext: base.DamageContext, SubmittedCommand: "＠dark-strike", SubmittedTarget: " &gt;enemy ", Discovered: []corebattle.CallbackEntry{{ID: "missing", Name: "dark-strike"}, {ID: row.selected, Name: "dark-strike"}}}
		if row.excluded {
			c.CustomName = c.SubmittedCommand
		}
		dark := &corebattle.RecoilDarkStateContext{DamageContext: base.DamageContext}
		var d BattleDefinitions
		d.RegisterRecoilDarkState(dark)
		d.RegisterNamedFields(c)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		old, _ := json.Marshal(s)
		ops, err := d.InvocationAction(&s, &f, []string{"job-22-entry-5"}, noArithmeticDraws{t})
		if row.reject {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-22-entry-5"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			current, _ := json.Marshal(s)
			if string(current) != string(old) || c.DamageCalls != 0 || dark.DamageCalls != 0 {
				t.Fatal(s, c, dark)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		draws := []float64{0}
		if !row.excluded {
			draws = append(draws, 0)
		}
		rng := &damageGemDraws{t: t, draws: draws}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
			t.Fatal(err, rng.used)
		}
		if s.Fields.Temporary != "darkness" || s.Actors[0].Attack != 150 || s.Actors[1].Attack != s.Actors[1].BaseStats.Attack {
			t.Fatal(s)
		}
		if !row.excluded && (c.DamageCalls != 1 || dark.DamageCalls != 1 || c.End.DarknessDebit == nil || c.End.ImmediateRiskCalls != 1 || s.Actors[0].HP != 250 || !c.GuaranteedHit) {
			t.Fatal(c, dark, s)
		}
		if row.excluded && (c.DamageCalls != 0 || s.Actors[0].HP != 500) {
			t.Fatal(c, s)
		}
	}
}

func TestNamedFieldMalformedGemsRejectBeforeOrAfterTaggedDarkness(t *testing.T) {
	for _, gem := range []string{"gem-101", "gem-102"} {
		for _, first := range []bool{false, true} {
			s, base, _ := dualAgilityFixture()
			c := &corebattle.NamedFieldContext{DamageContext: base.DamageContext}
			c.End.RecordDarkness(80)
			var d BattleDefinitions
			d.RegisterNamedFields(c)
			ids := []string{"job-22-entry-5", gem}
			if first {
				ids[0], ids[1] = ids[1], ids[0]
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Damage: &c.DamageContext}
			before, _ := json.Marshal(s)
			if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(ids, err)
			}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: ids[0]}, {Kind: "entry", EntryID: ids[1]}}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(ids, err)
			}
			after, _ := json.Marshal(s)
			if string(before) != string(after) || *c.End.DarknessDebit != 40 || c.DamageCalls != 0 || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
				t.Fatal(s, c)
			}
		}
	}
}
