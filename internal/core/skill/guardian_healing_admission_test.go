package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestGuardianAdmissionIsInertAndRejectsCompleteAction(t *testing.T) {
	_, _, catalog := guardianFixture()
	if len(catalog.Invocations.Entries) != 11 {
		t.Fatal(len(catalog.Invocations.Entries))
	}
	for id := range catalog.Invocations.Entries {
		for _, missing := range []bool{false, true} {
			s, c, d := guardianFixture()
			if missing {
				delete(d.Invocations.Entries, id)
			}
			c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			original := f
			_, err = d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
			if (err != nil) != missing {
				t.Fatal(id, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(f, original) || c.Arguments != nil || c.HealCalls != 0 || c.CureCalls != 0 || c.DamageCalls != 0 || c.StatusCalls != 0 {
				t.Fatal(id, s, c, f)
			}
		}
	}
	for _, reason := range []string{"defeat", "revived-target-gear", "status-gear", "conversion", "unknown-later", "unknown-transition"} {
		t.Run(reason, func(t *testing.T) {
			s, c, d := guardianFixture()
			ids := []string{"job-18-entry-7"}
			switch reason {
			case "defeat":
				c.Defeat = nil
			case "revived-target-gear":
				s.Actors[0].Status, s.Actors[2].Gear = corebattle.StatusConfusion, nil
			case "status-gear":
				ids, s.Actors[1].Gear = []string{"job-18-entry-1"}, nil
			case "conversion":
				ids, s.Actors[0].Extended, c.Command = []string{"job-18-entry-1"}, corebattle.ExtendedMagicSword, "attack"
			case "unknown-later":
				ids = []string{"job-18-entry-7", "missing"}
			case "unknown-transition":
				ids = []string{"job-18-entry-7", "job-18-entry-7"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
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
			if string(before) != string(after) || c.Arguments != nil || c.HealCalls != 0 || c.DamageCalls != 0 || c.StatusCalls != 0 || f.Arguments[0] != "enemy" {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestGuardianRegistrationAndRevivalOwners(t *testing.T) {
	s, c, _ := guardianFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missing BattleDefinitions
	missing.RegisterGuardianHealing(nil)
	for _, d := range []BattleDefinitions{{}, missing} {
		if _, err := d.InvocationAction(&s, &f, []string{"job-17-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, first := range []bool{false, true} {
		s, c, _ := guardianFixture()
		var d BattleDefinitions
		r := corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: func(string) error { return nil }}
		if first {
			d.RegisterRevivals(&r)
		}
		d.RegisterGuardianHealing(c)
		if !first {
			d.RegisterRevivals(&r)
		}
		for _, row := range []struct{ job, pos, sp, mp int }{{17, 4, 100, 1}, {17, 6, 160, 80}, {18, 4, 70, 27}, {18, 6, 120, 80}} {
			e := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if e.ID != fmt.Sprintf("job-%d-entry-%d", row.job, row.pos) || e.RequiredSP != row.sp || e.MPCost != row.mp || !d.RevivalSkills[e.ID] {
				t.Fatal(e)
			}
		}
		executeGuardian(t, &s, c, d, "job-17-entry-6", []string{"fallen"})
		if s.Actors[2].HP != 1000 {
			t.Fatal(s)
		}
		// Both full original pools are admitted with the independent owners.
		for _, job := range []int{17, 18} {
			s.Actors[0].Jobs.JobID = fmt.Sprintf("job-%d", job)
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); err != nil {
				t.Fatal(job, err)
			}
			delete(d.Invocations.Entries, fmt.Sprintf("job-%d-entry-4", job))
			if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(job, err)
			}
		}
	}
}

func TestGuardianIndirectCostsAndTargetArguments(t *testing.T) {
	for _, kind := range []string{"random", "tool-84", "gem-86"} {
		s, c, d := guardianFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-18", SP: 0}
		s.Actors[0].MP = 0
		d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: func(string) error { return nil }})
		d.Invocations.Entries["indirect"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: kind}}
		draws := []float64{7.0 / 8, 0, 0, 0}
		if kind == "random" {
			// Empty peer contributions isolate this real subset; production
			// catalog assembly still requires every original job table.
			for job := 1; job <= 87; job++ {
				if job != 17 && job != 18 {
					d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
				}
			}
			// Extended position7 must redraw to ordinary position5.
			draws = []float64{17.0 / 87, 7.0 / 8, 5.0 / 8, 0, 0}
		}
		if kind == "gem-86" {
			s.Actors[0].MP, s.Actors[0].Jobs.SP, draws = 100, 120, []float64{0, 0, 0}
		}
		executeGuardian(t, &s, c, d, "indirect", []string{"enemy", "tail"}, draws...)
		if s.Actors[0].MP != 0 || s.Actors[0].CMP != 90 {
			t.Fatal(kind, s)
		}
		if kind == "random" {
			if s.Actors[0].HP != 716 || s.Actors[2].HP != 0 || len(c.Arguments) != 2 {
				t.Fatal(s, c)
			}
		} else {
			if s.Actors[0].HP != 770 || s.Actors[1].HP != 414 || s.Actors[2].HP != 271 {
				t.Fatal(s, c)
			}
			want := []string{"tail"}
			if kind == "gem-86" {
				want = nil
			}
			if fmt.Sprint(c.Arguments) != fmt.Sprint(want) {
				t.Fatal(kind, c.Arguments)
			}
		}
	}
	for _, row := range []struct {
		sp, mp  int
		applied bool
	}{{119, 100, false}, {120, 99, false}, {120, 100, true}} {
		s, c, d := guardianFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-18", SP: row.sp}
		s.Actors[0].MP = row.mp
		d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		draws := []float64(nil)
		if row.applied {
			draws = []float64{0, 0, 0}
		}
		executeGuardian(t, &s, c, d, "extended", []string{"enemy"}, draws...)
		wantMP := row.mp
		if row.applied {
			wantMP -= 100
		}
		if s.Actors[0].MP != wantMP || (s.Actors[2].HP > 0) != row.applied {
			t.Fatal(row, s)
		}
	}
}
