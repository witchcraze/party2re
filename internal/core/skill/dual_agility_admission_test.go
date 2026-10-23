package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestDualAgilityWholeActionAdmissionIsInert(t *testing.T) {
	for _, id := range []string{"job-77-entry-0", "job-77-entry-2", "job-77-entry-3", "job-77-entry-4", "job-77-entry-4-temporary", "job-83-entry-3"} {
		s, c, d := dualAgilityFixture()
		s.Fields.Default = "cosmic"
		s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		c.Reset = func(string) error { t.Fatal("validation ran reset"); return nil }
		c.Defeat = func(string) error { t.Fatal("validation ran defeat"); return nil }
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "enemy", "enemy"}}
		gear, args, party := s.Actors[1].Gear, frame.Arguments, frame.PartyIDs
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		after, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || gear != s.Actors[1].Gear || gear.ArmorDefense != 0 || !reflect.DeepEqual(args, []string{"enemy", "enemy", "enemy"}) || !reflect.DeepEqual(party, []string{"self", "fallen"}) {
			t.Fatal(id, s, frame)
		}
	}
	// A single blocked strike needs no unsupported tail; a possible item147
	// followup must be admitted before the defense is consumed.
	for _, extra := range []bool{false, true} {
		s, c, d := dualAgilityFixture()
		s.Fields.Default = "cosmic"
		s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		if extra {
			s.Actors[0].ItemDefinitionIDs = []string{"item-147"}
		}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		_, err := d.InvocationAction(&s, &frame, []string{"job-83-entry-0"}, noArithmeticDraws{t})
		if extra != errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(extra, err)
		}
		if !extra {
			executeDual(t, &s, c, d, "job-83-entry-0", []string{"enemy"}, .5)
		}
	}
}

func TestDualAgilityRegistrationMissingDescendantsAndExactProjection(t *testing.T) {
	s, c, d := dualAgilityFixture()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if len(d.Invocations.Entries) != 12 || len(d.Invocations.JobTables[77].Variants) != 2 {
		t.Fatal(d)
	}
	if _, err := (BattleDefinitions{}).InvocationAction(&s, &frame, []string{"job-77-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// An inline stance projection removes phantom before the following attack.
	s.Actors[0].Temporary = corebattle.TemporaryPhantom
	s.Actors[1].Temporary = corebattle.TemporaryNullAttack
	c.Defeat = nil
	before := s.Actors[0].Temporary
	ids := []string{"job-83-entry-4", "job-83-entry-5"}
	ops, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t})
	if err != nil || s.Actors[0].Temporary != before {
		t.Fatal(err, s)
	}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].Temporary != corebattle.TemporaryRage || s.Actors[1].HP != 1000 {
		t.Fatal(err, s)
	}
	// Missing reachable work rejects before an earlier inline mutation.
	s.Actors[0].Hit = 50
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-83-entry-1"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[0].Hit != 50 {
		t.Fatal(err, s)
	}
	// Opaque earlier effects cannot launder now-unreachable damage descendants.
	d.Invocations.Entries["opaque"] = corebattle.CallbackDefinition{Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil }, Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil }}
	if _, err := d.InvocationAction(&s, &frame, []string{"opaque", "job-83-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestDualAgilityEveryPositionAcceptsNoEnemyContext(t *testing.T) {
	for _, job := range []int{77, 83} {
		for _, temporary := range []bool{false, true} {
			for _, entry := range corebattle.DualAgilityEntries(job, temporary) {
				s, c, d := dualAgilityFixture()
				s.Actors[1].TeamID = "party"
				c.PartyIDs = append(c.PartyIDs, "enemy")
				c.EnemyIDs = nil
				c.Defeat = nil
				c.Status = &corebattle.StatusContext{}
				draws := []float64{}
				if entry.ID == "job-77-entry-2" {
					draws = []float64{0}
				}
				executeDual(t, &s, c, d, entry.ID, nil, draws...)
				if s.Actors[0].HP != 1000 || s.Actors[1].HP != 1000 || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 || c.DamageCalls != 0 {
					t.Fatal(entry, s)
				}
			}
		}
	}
}
