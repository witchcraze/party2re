package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRecoilDarkAdmissionPurity(t *testing.T) {
	for _, job := range []int{21, 22, 38} {
		for _, entry := range corebattle.RecoilDarkStateEntries(job) {
			if entry.ID == "job-22-entry-5" {
				continue
			}
			for _, reject := range []bool{false, true} {
				s, c, d := recoilDarkFixture()
				if reject {
					delete(d.Invocations.Entries, entry.ID)
				}
				weapon := 7
				s.Actors[0].Gear.WeaponAttack = &weapon
				gear := s.Actors[0].Gear
				c.RawDefeat = rawDefeatContext()
				c.RawDefeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
					t.Fatal("reward Apply during admission")
					return nil
				}
				c.Defeat = func(string) error { t.Fatal("external defeat during admission"); return nil }
				c.Reset = func(string) error { t.Fatal("external reset during admission"); return nil }
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				original, raw := frame, *c.RawDefeat
				_, err = d.InvocationAction(&s, &frame, []string{entry.ID}, noArithmeticDraws{t})
				if (err != nil) != reject {
					t.Fatal(entry.ID, reject, err)
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || !reflect.DeepEqual(original, frame) || c.DamageCalls != 0 || c.HealCalls != 0 || c.StatusCalls != 0 || c.StatUpCalls != 0 || c.StatDownCalls != 0 || c.TensionCalls != 0 || len(c.Arguments) != 0 || weapon != 7 || s.Actors[0].Gear != gear || gear.WeaponAttack != &weapon || !reflect.DeepEqual(raw.EquipmentResetContext, c.RawDefeat.EquipmentResetContext) {
					t.Fatal(entry.ID, s, c, frame)
				}
			}
		}
	}
}

func TestRecoilDarkMissingDescendantsRejectBeforeEffects(t *testing.T) {
	for _, bad := range []string{"raw-defeat", "raw-rewards", "raw-reset", "raw-floor-two", "damage-port", "second-resolution", "second-tension", "unsupported-conversion"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := recoilDarkFixture()
			id := "job-21-entry-5"
			s.Actors[1].HP = 1
			c.RawDefeat = rawDefeatContext()
			switch bad {
			case "raw-defeat":
				c.RawDefeat = nil
			case "raw-rewards":
				c.RawDefeat.Rewards = nil
			case "raw-reset":
				s.Actors[1].Gear.AccessoryID = "item-110"
			case "raw-floor-two":
				c.RawDefeat = nil
				s.Actors[0].Attack, s.Actors[1].Defense, s.Actors[1].HP = 1, 5, 2
			case "damage-port":
				id, c.Defeat = "job-22-entry-0", nil
			case "second-resolution":
				id = "job-38-entry-5"
				// First magic check reflects to self; the second may then choose
				// another enemy whose damage gear is missing.
				s.Actors[1].HP, s.Actors[1].Temporary = 900, corebattle.TemporaryReflectMagic
				other := s.Actors[1]
				other.ID, other.Gear, other.Temporary = "other", nil, ""
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, "other")
			case "second-tension":
				id, s.Actors[0].Tension = "job-21-entry-6", 3
			case "unsupported-conversion":
				id, c.Command = "job-22-entry-2", "attack"
				s.Actors[0].Extended = corebattle.ExtendedMagicSword
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
			if string(before) != string(after) || c.DamageCalls != 0 || c.TensionCalls != 0 || len(c.Arguments) != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestRecoilDarkRegistrationNoEffectAndIndirectCosts(t *testing.T) {
	s, c, d := recoilDarkFixture()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	for _, definitions := range []BattleDefinitions{{}, func() BattleDefinitions { var d BattleDefinitions; d.RegisterRecoilDarkState(nil); return d }()} {
		if _, err := definitions.InvocationAction(&s, &frame, []string{"job-21-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	s.Actors[1].HP, c.Defeat = 0, nil
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-0"}, []string{"enemy"})
	if c.DamageCalls != 0 || c.End.ImmediateRiskCalls != 0 {
		t.Fatal(c)
	}
	for _, job := range []int{21, 22, 38} {
		s, c, d = recoilDarkFixture()
		s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
		d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
		id := "job-22-entry-5"
		if job != 22 {
			entries := corebattle.RecoilDarkStateEntries(job)
			id = entries[len(entries)-1].ID
			delete(d.Invocations.Entries, id)
		}
		frame.TargetContext = c.TargetContext
		if _, err := d.InvocationAction(&s, &frame, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
	}
	s, c, d = recoilDarkFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-21", 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	executeRecoilDark(t, &s, c, d, []string{"tool"}, []string{"ignored"}, 6.0/7)
	if s.Actors[0].MP != 0 || s.Actors[0].Extended != corebattle.ExtendedFrenzy {
		t.Fatal(s)
	}
	s, c, d = recoilDarkFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-21", 110
	d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
	executeRecoilDark(t, &s, c, d, []string{"extended"}, nil)
	if s.Actors[0].MP != 60 || s.Actors[0].Tension != 3 {
		t.Fatal(s)
	}
	// A real random pool retains all87 jobs; unrelated jobs have explicit empty
	// tables here. Job22's extended field is redrawn rather than certified.
	s, c, d = recoilDarkFixture()
	for job := 1; job <= 87; job++ {
		if _, ok := d.Invocations.JobTables[job]; !ok {
			d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{nil}}
		}
	}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	s.Actors[0].MP = 0
	executeRecoilDark(t, &s, c, d, []string{"random"}, []string{"enemy", "tail"}, 21.0/87, 5.0/6, 0, 0)
	if s.Actors[1].HP != 833 || s.Actors[0].MP != 0 || len(c.Arguments) != 1 || c.Arguments[0] != "tail" {
		t.Fatal(s, c)
	}
}

func TestRecoilDarkOrderedProjection(t *testing.T) {
	s, c, d := recoilDarkFixture()
	// The first exact stance replaces one-hit defense before a confused self hit.
	s.Actors[0].Temporary = corebattle.TemporaryOneHitDefense
	s.Actors[0].Status = corebattle.StatusConfusion
	executeRecoilDark(t, &s, c, d, []string{"job-38-entry-2", "job-38-entry-0"}, []string{"self"})
	if s.Actors[0].HP != 500 || s.Actors[1].HP != 900 || c.DamageCalls != 0 {
		t.Fatal(s, c)
	}
	// Unknown earlier random effects cannot authorize a later damage callback.
	s, c, d = recoilDarkFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-21-entry-5", "job-38-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
