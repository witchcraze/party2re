package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestHighPowerAdmissionPurity(t *testing.T) {
	for _, job := range []int{47, 48, 49, 52} {
		for _, entry := range corebattle.HighPowerEntries(job) {
			for _, reject := range []bool{false, true} {
				s, c, d := highPowerFixture()
				if reject {
					delete(d.Invocations.Entries, entry.ID)
				}
				weapon := 7
				s.Actors[0].Gear.WeaponAttack = &weapon
				gear := s.Actors[0].Gear
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				original := frame
				resetOriginal := *c.EquipmentReset
				c.Reset = func(string) error { t.Fatal("external reset during validation"); return nil }
				c.Defeat = func(string) error { t.Fatal("external defeat during validation"); return nil }
				_, err = d.InvocationAction(&s, &frame, []string{entry.ID}, noArithmeticDraws{t})
				if (err != nil) != reject {
					t.Fatal(entry.ID, reject, err)
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || !reflect.DeepEqual(original, frame) || !reflect.DeepEqual(resetOriginal, *c.EquipmentReset) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 || c.StatusCalls != 0 || c.TensionCalls != 0 || len(c.Arguments) != 0 || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
					t.Fatal(entry.ID, s, c, frame)
				}
			}
		}
	}
}

func TestHighPowerMissingDescendantsRejectBeforeAnyEffect(t *testing.T) {
	for _, bad := range []string{"reset-port", "last-reset-gear", "unsupported-reset", "later-hit", "later-status"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := highPowerFixture()
			id := "job-48-entry-5"
			switch bad {
			case "reset-port":
				c.EquipmentReset = nil
			case "last-reset-gear":
				s.Actors[2].Gear = nil
			case "unsupported-reset":
				s.Actors[2].Gear.AccessoryID = "item-110"
			case "later-hit", "later-status":
				id = "job-47-entry-4"
				if bad == "later-status" {
					id = "job-48-entry-0"
				}
				s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
				if bad == "later-status" {
					s.Actors[1].Temporary = ""
				}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(bad, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestHighPowerRevivalChangesLaterEligibility(t *testing.T) {
	for _, reject := range []bool{false, true} {
		s, c, d := highPowerFixture()
		s.Actors[1].HP = 0
		s.Actors[0].Tension, s.Actors[1].Tension, s.Actors[2].Tension = 1.7, 3, 5
		s.Actors[0].Status = corebattle.StatusPoison
		s.Actors[1].Extended, s.Actors[1].ExtendedTurns = corebattle.ExtendedCrown, 4
		if reject {
			c.Defeat = nil
		}
		ids := []string{"job-48-entry-5", "job-47-entry-0"}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		_, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t})
		if reject {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || s.Actors[1].HP != 0 || s.Actors[0].Status != corebattle.StatusPoison {
				t.Fatal(err, s)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		executeHighPower(t, &s, c, d, ids, []string{"enemy"}, 0)
		if s.Actors[1].HP != 0 || s.Actors[2].HP != 1 || s.Actors[0].Status != "" || s.Actors[1].Extended != "" || s.Actors[1].Tension != 3 || s.Actors[2].Tension != 5 || s.Actors[0].Temporary != corebattle.TemporaryAbsorbMagic {
			t.Fatal(s)
		}
	}
	// Reset-only replay preserves every member's tension and applies the
	// accessory before revival: a fallen reflection accessory stays inactive.
	s, c, d := highPowerFixture()
	s.Actors[2].Gear.AccessoryID = "item-163"
	s.Actors[0].Tension, s.Actors[1].Tension, s.Actors[2].Tension = 1.7, 3, 5
	executeHighPower(t, &s, c, d, []string{"job-48-entry-5"}, nil)
	if s.Actors[0].Tension != 1.7 || s.Actors[1].Tension != 3 || s.Actors[2].Tension != 5 || s.Actors[2].Temporary != "" || s.Actors[2].HP != 1 {
		t.Fatal(s)
	}
}

func TestHighPowerEmptyRegistrationNoEffectAndIndirectCosts(t *testing.T) {
	s, c, d := highPowerFixture()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	for _, definitions := range []BattleDefinitions{{}, func() BattleDefinitions { var d BattleDefinitions; d.RegisterHighPower(nil); return d }()} {
		if _, err := definitions.InvocationAction(&s, &frame, []string{"job-47-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	// A source no-effect still succeeds without unreachable damage ports.
	s.Actors[1].HP = 0
	c.Defeat = nil
	executeHighPower(t, &s, c, d, []string{"job-47-entry-0"}, []string{"enemy"})
	if c.DamageCalls != 0 {
		t.Fatal(c)
	}
	for _, job := range []int{47, 48, 49, 52} {
		s, c, d = highPowerFixture()
		s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		delete(d.Invocations.Entries, corebattle.HighPowerEntries(job)[len(corebattle.HighPowerEntries(job))-1].ID)
		frame.TargetContext = c.TargetContext
		if _, err := d.InvocationAction(&s, &frame, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
	}
	s, c, d = highPowerFixture()
	s.Actors[0].Jobs.JobID = "job-47"
	s.Actors[0].MP = 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	// Ordinary SP/MP are ignored, but the selected extension's entire body runs.
	executeHighPower(t, &s, c, d, []string{"tool"}, []string{"ignored"}, 5.0/6)
	if s.Actors[0].MP != 0 || s.Actors[0].Attack != 225 {
		t.Fatal(s)
	}
	s, c, d = highPowerFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-47", 160
	d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
	executeHighPower(t, &s, c, d, []string{"extended"}, []string{"ignored"})
	if s.Actors[0].MP != 33 || s.Actors[0].Attack != 225 {
		t.Fatal(s)
	}
}

func TestHighPowerOrderedProjectionAndUnknownEffects(t *testing.T) {
	s, c, d := highPowerFixture()
	executeHighPower(t, &s, c, d, []string{"job-47-entry-5", "job-47-entry-0"}, []string{"enemy"}, 0)
	if s.Actors[0].Attack != 225 || s.Actors[1].HP != 779 {
		t.Fatal(s)
	}
	s, c, d = highPowerFixture()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	for _, ids := range [][]string{{"job-47-entry-0", "job-48-entry-5"}, {"job-52-entry-3", "job-47-entry-0"}} {
		if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
	}
	// A random accessory reset cannot be sampled to authorize a later hit.
	s.Actors[2].Gear.AccessoryID = "item-159"
	if _, err := d.InvocationAction(&s, &frame, []string{"job-48-entry-5", "job-47-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[2].HP != 0 || c.EquipmentReset.Cursor.Source != "" {
		t.Fatal(s, c)
	}
}

func TestHighPowerDamageCountSuppressesUnreachableStatus(t *testing.T) {
	s, c, d := highPowerFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	s.Actors[1].Gear = nil
	c.Defeat = nil
	executeHighPower(t, &s, c, d, []string{"job-48-entry-0"}, nil, 0)
	if s.Actors[1].HP != 900 || s.Actors[1].Status != "" || c.DamageCalls != 1 || c.StatusCalls != 1 {
		t.Fatal(s, c)
	}
}
