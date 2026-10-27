package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestMartialWholeActionAdmissionIsInert(t *testing.T) {
	for _, id := range []string{"job-4-entry-7", "job-23-entry-3"} {
		for _, rejected := range []bool{false, true} {
			s, c, d := martialFixture()
			if rejected {
				s.Fields.Default = "cosmic"
			}
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
			c.Reset = func(string) error { t.Fatal("admission ran reset"); return nil }
			c.Defeat = func(string) error { t.Fatal("admission ran defeat"); return nil }
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "enemy"}}
			c.Arguments = []string{"frozen"}
			gear, args, party := s.Actors[1].Gear, frame.Arguments, frame.PartyIDs
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t})
			if (err != nil) != rejected || rejected && !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(id, rejected, err)
			}
			if rejected {
				ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-4-entry-1"}, {Kind: "entry", EntryID: id}}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || *gear != (corebattle.BattleGearStats{}) || !reflect.DeepEqual(args, []string{"enemy", "enemy"}) || !reflect.DeepEqual(party, []string{"self", "fallen"}) || !reflect.DeepEqual(c.Arguments, []string{"frozen"}) || c.DamageCalls != 0 {
				t.Fatal(s, c, frame)
			}
		}
	}
}

func TestMartialMissingRegistrationDescendantsAndNoEffect(t *testing.T) {
	s, c, d := martialFixture()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	var missing BattleDefinitions
	missing.RegisterMartial(nil)
	for _, definitions := range []BattleDefinitions{{}, missing} {
		if _, err := definitions.InvocationAction(&s, &frame, []string{"job-4-entry-1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	if len(d.Invocations.Entries) != 22 || len(d.Invocations.JobTables) != 3 {
		t.Fatal(d)
	}
	// A reached death helper requires its complete defeat owner at HP999.
	s.Actors[1].HP = 999
	if _, err := d.InvocationAction(&s, &frame, []string{"job-4-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// No enemy is accepted no-effect and does not need damage/defeat context.
	c.EnemyIDs, c.Defeat = nil, nil
	s.Actors[1].TeamID = "party"
	c.PartyIDs = append(c.PartyIDs, "enemy")
	for _, id := range []string{"job-4-entry-3", "job-4-entry-4", "job-4-entry-5", "job-4-entry-6", "job-23-entry-0", "job-23-entry-2", "job-23-entry-4", "job-25-entry-2"} {
		executeMartial(t, &s, c, d, id, []string{"enemy"})
	}
	if s.Actors[1].HP != 999 || c.DamageCalls != 0 || s.Actors[0].Temporary != corebattle.TemporaryDouble {
		t.Fatal(s, c)
	}
	s.Actors[0].Temporary = corebattle.TemporaryDeflect
	executeMartial(t, &s, c, d, "job-23-entry-2", []string{"enemy"})
	if c.HealCalls != 0 {
		t.Fatal(c)
	}
}

func TestMartialStatDescendantsAndOrderedTension(t *testing.T) {
	for _, item := range []string{"item-201", "item-205"} {
		s, c, d := martialFixture()
		s.Actors[0].ItemDefinitionIDs = []string{item}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		if _, err := d.InvocationAction(&s, &frame, []string{"job-4-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(item, err)
		}
	}
	s, c, d := martialFixture()
	s.Actors[0].Tension = 3
	executeMartial(t, &s, c, d, "job-23-entry-1", nil)
	if s.Actors[0].Attack != 220 || s.Actors[0].Defense != 70 || c.StatUpCalls != 2 {
		t.Fatal(s, c)
	}
	// A capped first helper preserves tension for the second, which still
	// requires the item205 requester context before either helper executes.
	s, c, d = martialFixture()
	s.Actors[0].Attack, s.Actors[0].Tension = 250, 3
	s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &frame, []string{"job-4-entry-8"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Extended != "" || c.StatUpCalls != 0 {
		t.Fatal(err, s, c)
	}
	level := 99
	c.RequesterLevel = &level
	executeMartial(t, &s, c, d, "job-4-entry-8", nil)
	if s.Actors[0].Attack != 310 || s.Actors[0].Defense != 110 || s.Actors[0].Temporary != corebattle.TemporaryDouble || s.Actors[0].Extended != corebattle.ExtendedFlame || c.TensionCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestMartialExactCoverProjectionProtectsLaterAdmission(t *testing.T) {
	s, c, d := martialFixture()
	s.Actors[0].Temporary = corebattle.TemporaryPhantom
	s.Actors[1].Temporary = corebattle.TemporaryNullAttack
	c.Defeat = nil
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&s, &frame, []string{"job-25-entry-4", "job-23-entry-4"}, noArithmeticDraws{t})
	if err != nil || s.Actors[0].Temporary != corebattle.TemporaryPhantom {
		t.Fatal(err, s)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].Temporary != corebattle.TemporaryStrongDefense || s.Actors[1].HP != 1000 {
		t.Fatal(err, s)
	}
	// Cover's exact projection overwrites even a fallen ally's temporary.
	s, c, d = martialFixture()
	s.Actors[2].Temporary = corebattle.TemporaryReflectMagic
	frame = corebattle.CallbackContext{TargetContext: c.TargetContext}
	known, err := d.Invocations.Entries["job-25-entry-6"].Project(&s, &frame)
	if err != nil || !known || s.Actors[2].Temporary != corebattle.TemporaryCover || s.Actors[0].Temporary != corebattle.TemporaryCovering {
		t.Fatal(err, s)
	}
}
