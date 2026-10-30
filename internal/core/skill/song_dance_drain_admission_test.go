package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestSongDanceDrainRegistrationAndPureAdmission(t *testing.T) {
	s, c, d := songDanceFixture()
	if len(d.Invocations.Entries) != 19 || len(d.Invocations.JobTables[13].Variants[0]) != 6 || len(d.Invocations.JobTables[14].Variants[0]) != 7 || len(d.Invocations.JobTables[20].Variants[0]) != 7 {
		t.Fatal(d)
	}
	for _, job := range []int{13, 14, 20} {
		for _, entry := range corebattle.SongDanceDrainEntries(job) {
			s, c, d := songDanceFixture()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			original, gear := f, *s.Actors[1].Gear
			c.Defeat = func(string) error { t.Fatal("admission executed damage hook"); return nil }
			c.Status.Defeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
				t.Fatal("admission executed defeat hook")
				return nil
			}
			_, err = d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t})
			if entry.ID == "job-14-entry-4" {
				if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(entry, err)
				}
			} else if err != nil {
				t.Fatal(entry, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(original, f) || !reflect.DeepEqual(gear, *s.Actors[1].Gear) || c.Arguments != nil || c.DamageCalls != 0 || c.Status.DeathCalls != 0 {
				t.Fatal(entry, s, c, f)
			}
			if _, err := (BattleDefinitions{}).InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(entry, err)
			}
			d.RegisterSongDanceDrain(nil)
			if _, err := d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(entry, err)
			}
		}
	}
	// Registration preserves the separately owned sacrifice regardless of order.
	revival := &corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: func(string) error { return nil }}
	d.RegisterRevivals(revival)
	d.RegisterSongDanceDrain(c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &f, []string{"job-14-entry-4"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
}

func TestSongDanceDrainWholeActionRejection(t *testing.T) {
	for _, bad := range []string{"unknown", "later-hit", "status-gear", "death", "agility-tail", "defense-tail", "opaque", "item110", "gem101", "gem102"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := songDanceFixture()
			ids := []string{"job-14-entry-2", "missing"}
			switch bad {
			case "later-hit":
				ids = []string{"job-14-entry-2", "job-14-entry-5"}
				s.Actors[1].Temporary, s.Actors[1].Gear = corebattle.TemporaryOneHitDefense, nil
			case "status-gear":
				ids, s.Actors[1].Gear = []string{"job-20-entry-0"}, nil
			case "death":
				ids, c.Status.Defeat = []string{"job-20-entry-5"}, nil
			case "agility-tail":
				ids, s.Actors[0].ItemDefinitionIDs = []string{"job-14-entry-0"}, []string{"item-201"}
			case "defense-tail":
				ids, s.Actors[0].ItemDefinitionIDs = []string{"job-13-entry-3"}, []string{"item-205"}
			case "opaque":
				ids = []string{"job-13-entry-1", "job-14-entry-0"}
			case "item110":
				ids = []string{"job-14-entry-2", "tool-110"}
			case "gem101":
				ids = []string{"job-14-entry-2", "gem-101"}
			case "gem102":
				ids = []string{"job-14-entry-2", "gem-102"}
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			args := f.Arguments
			if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(ids, err)
			}
			var ops []corebattle.CallbackInvocation
			for _, id := range ids {
				ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(ids, err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || !reflect.DeepEqual(args, []string{"enemy", "tail"}) || c.Arguments != nil || c.DamageCalls != 0 || c.StatUpCalls != 0 {
				t.Fatal(s, c, f)
			}
		})
	}
	// The first inline write replaces a nullifier and exposes a later target.
	s, c, d := songDanceFixture()
	s.Actors[0].Status, s.Actors[0].Gear = corebattle.StatusConfusion, nil
	s.Actors[0].Temporary = corebattle.TemporaryOneHitDefense
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"job-20-entry-1"}, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.InvocationAction(&s, &f, []string{"job-20-entry-2", "job-20-entry-1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if s.Actors[0].Temporary != corebattle.TemporaryOneHitDefense {
		t.Fatal("projection escaped", s)
	}
}

func TestSongDanceDrainDelegatedCostsAndCompletePools(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		s, c, d := songDanceFixture()
		s.Actors[0].Jobs.JobID = "job-20"
		s.Actors[0].Abilities = []string{"seal_kotowari"}
		id, draws := "job-20-entry-1", []float64{0, 0}
		if delegated {
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			id, draws = "tool", []float64{.2, 0, 0}
		}
		executeSongDance(t, &s, c, d, id, []string{"enemy", "tail"}, draws...)
		wantMP := 80 // Incidental debit3; returned32 => floor3 recovery=>+3.
		if s.Actors[0].MP != wantMP || s.Actors[1].HP != 868 || c.Arguments[0] != "tail" {
			t.Fatal(delegated, s, c)
		}
	}
	// A debit without a returned target is restored only by the tool wrapper.
	for _, delegated := range []bool{false, true} {
		s, c, d := songDanceFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].Abilities = "job-20", []string{"seal_kotowari"}
		s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		id, draws, wantMP := "job-20-entry-1", []float64{}, 77
		if delegated {
			d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
			id, draws, wantMP = "tool", []float64{.2}, 80
		}
		executeSongDance(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[0].MP != wantMP || c.MPHealCalls != 0 {
			t.Fatal(delegated, s, c)
		}
	}
	s, c, d := songDanceFixture()
	s.Actors[0].Jobs.JobID = "job-14"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("missing14.4 must reject entire pool", err)
	}
	revival := &corebattle.RevivalContext{TargetContext: c.TargetContext, Defeat: func(string) error { return nil }}
	d.RegisterRevivals(revival)
	executeSongDance(t, &s, c, d, "tool", []string{"enemy"}, .3) // Select stance14.2.
	if s.Actors[0].MP != 80 || s.Actors[0].Temporary != corebattle.TemporaryDeflect {
		t.Fatal(s)
	}
	delete(d.Invocations.Entries, "job-14-entry-6")
	if _, err := d.InvocationAction(&s, &f, []string{"tool"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("missing later body must reject entire pool", err)
	}
	if _, err := d.InvocationAction(&s, &f, []string{"random"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("subset registration cannot certify all original jobs", err)
	}
}

func TestSongDanceDrainAcceptedNoEffectAndOrderedProjection(t *testing.T) {
	for _, id := range []string{"job-14-entry-3", "job-14-entry-5", "job-20-entry-0", "job-20-entry-1", "job-20-entry-3", "job-20-entry-4", "job-20-entry-5", "job-20-entry-6"} {
		s, c, d := songDanceFixture()
		s.Actors[1].HP, s.Actors[1].Gear, c.Defeat, c.Status.Defeat = 0, nil, nil, nil
		draws := []float64{}
		if id == "job-14-entry-5" {
			draws = []float64{0, 0, 0} // Count and two fresh absent targets.
		}
		executeSongDance(t, &s, c, d, id, []string{"enemy"}, draws...)
		if s.Actors[1].HP != 0 || s.Actors[0].HP != 500 || s.Actors[0].MP != 80 {
			t.Fatal(id, s)
		}
	}
	s, c, d := songDanceFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	ops, err := d.InvocationAction(&s, &f, []string{"job-14-entry-2", "job-20-entry-2", "job-20-entry-1"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: []float64{0, 0}}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[0].Temporary != corebattle.TemporaryReduceMagic || s.Actors[0].MP != 84 || s.Actors[1].HP != 860 {
		t.Fatal(s)
	}
}
