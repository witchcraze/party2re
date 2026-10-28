package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestLateSongRegistrationAndPureAdmission(t *testing.T) {
	for _, job := range []int{84, 85, 86} {
		for _, entry := range corebattle.LateBreathSongEntries(job) {
			s, c, d := lateSongFixture()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			original := f
			c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
			_, bound := d.Invocations.Entries[entry.ID]
			_, err = d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t})
			if bound != (err == nil) {
				t.Fatal(entry.ID, bound, err)
			}
			if !bound && !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(f, original) || c.DamageCalls != 0 || c.Death.DeathCalls != 0 || c.TensionCalls != 0 || c.Arguments != nil {
				t.Fatal(entry.ID, s, c, f)
			}
			if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(entry.ID, err)
			}
		}
	}
	s, c, d := lateSongFixture()
	if len(d.Invocations.Entries) != 10 || len(d.Invocations.JobTables[84].Variants[0]) != 5 || len(d.Invocations.JobTables[85].Variants[0]) != 5 || len(d.Invocations.JobTables[86].Variants[0]) != 6 {
		t.Fatal(d)
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	d.RegisterLateBreathSong(nil)
	if _, err := d.InvocationAction(&s, &f, []string{"job-84-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestLateSongMissingDescendantsAndOrderedProjection(t *testing.T) {
	for _, bad := range []string{"unknown", "gear", "death", "book", "book-changed", "tension-level", "later-tension", "opaque", "undo-exposes-gear"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := lateSongFixture()
			ids := []string{"job-84-entry-0", "missing"}
			args := []string{"enemy"}
			switch bad {
			case "gear":
				s.Actors[1].Gear = nil
				ids = []string{"job-84-entry-1"}
			case "death":
				s.Actors[1].HP = 999
				c.Death.Defeat = nil
				ids = []string{"job-85-entry-1"}
			case "book":
				c.HeldBooks = nil
				ids = []string{"job-86-entry-4"}
			case "book-changed":
				ids = []string{"job-86-entry-4"}
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
				if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); err != nil {
					t.Fatal(err)
				}
				c.HeldBooks["self"] = "unclassified"
			case "tension-level":
				s.Actors[0].Tension = 5
				ids, args = []string{"job-84-entry-3"}, []string{"self"}
			case "later-tension":
				// A successful dance cure enables195 confusion, so an enemy can
				// receive the second tension even with an explicit self target.
				s.Actors[0].Status, s.Actors[1].Tension = corebattle.StatusDanceSeal, 5
				c.Item195Activated = true
				ids, args = []string{"job-84-entry-3"}, []string{"self"}
			case "opaque":
				ids = []string{"job-85-entry-3", "job-85-entry-4"}
			case "undo-exposes-gear":
				s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryNullMagic, nil
				ids = []string{"job-85-entry-2", "job-86-entry-4"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
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
			if string(before) != string(after) || c.DamageCalls != 0 || c.TensionCalls != 0 || c.MPHealCalls != 0 || c.Arguments != nil {
				t.Fatal(s, c)
			}
		})
	}
	// Inline undo removes the actor's chain before the later dance helper.
	s, c, d := lateSongFixture()
	s.Actors[0].Status = corebattle.StatusSabaku
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	ops, err := d.InvocationAction(&s, &f, []string{"job-85-entry-2", "job-84-entry-3"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[0].Status != "" || s.Actors[0].Tension != 1.7 {
		t.Fatal(s)
	}
}

func TestLateSongDelegationKeepsCostsAndCompletePools(t *testing.T) {
	s, c, d := lateSongFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-85", 0, 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	// Tool84 uses the complete job85 table, ignores SP/listed MP, and does not
	// consume parent arguments. The ritual's MP gain survives the wrapper.
	executeLateSong(t, &s, c, d, "tool", []string{"enemy", "tail"}, 0, 0, 0)
	if s.Actors[0].HP != 230 || s.Actors[0].MP != 102 || len(c.Arguments) != 2 {
		t.Fatal(s, c)
	}
	// Job84's missing burn body prevents selecting even a supported child.
	s, c, d = lateSongFixture()
	s.Actors[0].Jobs.JobID = "job-84"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
