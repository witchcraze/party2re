package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func revivalRegistryFixture() (corebattle.LifetimeState, corebattle.RevivalContext, BattleDefinitions) {
	s, c, d := rosterRegistryFixture()
	s.Actors = append(s.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "fallen", TeamID: "party", MaxHP: 101, MaxMP: 50, Gear: &corebattle.BattleGearStats{}}, Tension: 1})
	c.PartyIDs = append(c.PartyIDs, "fallen")
	return s, corebattle.RevivalContext{TargetContext: c.TargetContext}, d
}

func TestRevivalRegistrationConcreteProducersAndMetadata(t *testing.T) {
	s, c, d := revivalRegistryFixture()
	if _, err := d.RevivalAction(&s, &c, "gem-63", "fallen", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.RegisterRevivals(&c)
	jobs := 0
	seen := map[string]bool{}
	for _, entry := range corebattle.RevivalEntries() {
		if seen[entry.ID] || entry.Name == "" {
			t.Fatal(entry)
		}
		seen[entry.ID] = true
		if strings.HasPrefix(entry.ID, "job-") {
			jobs++
		}
		if d.Invocations.Entries[entry.ID].Execute == nil || !d.RevivalSkills[entry.ID] {
			t.Fatal(entry)
		}
	}
	if jobs != 20 || len(seen) != 28 {
		t.Fatal(jobs, len(seen))
	}
	for _, tc := range []struct {
		id       string
		sp, mp   int
		extended bool
	}{
		{"job-3-entry-6", 60, 1, false}, {"job-5-entry-7", 60, 20, false}, {"job-7-entry-8", 80, 1, false},
		{"job-14-entry-4", 45, 1, false}, {"job-16-entry-6", 100, 40, false}, {"job-17-entry-4", 100, 1, false},
		{"job-17-entry-6", 160, 80, false}, {"job-18-entry-4", 70, 27, false}, {"job-18-entry-6", 120, 80, false},
		{"job-32-entry-4", 100, 40, false}, {"job-33-entry-6", 160, 80, false}, {"job-39-entry-5", 50, 20, false},
		{"job-43-entry-6", 150, 70, false}, {"job-50-entry-6", 145, 1, false}, {"job-51-entry-5", 130, 30, false},
		{"job-57-entry-4", 80, 20, false},
		{"job-66-revive", 150, 0, true}, {"job-70-entry-3", 200, 27, false}, {"job-75-entry-5", 120, 0, false}, {"job-76-entry-3", 90, 1, false},
	} {
		var got corebattle.CallbackEntry
		for _, entry := range corebattle.RevivalEntries() {
			if entry.ID == tc.id {
				got = entry
			}
		}
		if got.RequiredSP != tc.sp || got.MPCost != tc.mp || got.Extended != tc.extended {
			t.Fatal(tc, got)
		}
	}
	if len(d.Invocations.JobTables) != 0 {
		t.Fatal("registered partial job table")
	}
	entry, err := d.RevivalAction(&s, &c, "gem-63", "fallen", noArithmeticDraws{t})
	if err != nil || entry.ID != "gem-63" {
		t.Fatal(entry, err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen", "tail"}}
	ops, err := d.InvocationAction(&s, &frame, []string{"gem-63"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[2].HP != 50 {
		t.Fatal(s, err)
	}
	// Reloaded facts are used directly; closures alone are rebuilt.
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err = json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	loaded.Actors[2].HP = 0
	var rebuilt BattleDefinitions
	rebuilt.RegisterRevivals(&c)
	frame.Arguments = []string{"fallen", "tail"}
	if err = rebuilt.Invocations.Entries["gem-64"].Execute(&loaded, &frame, noArithmeticDraws{t}); err != nil || loaded.Actors[2].HP != 101 || !reflect.DeepEqual(frame.Arguments, []string{"tail"}) {
		t.Fatal(loaded, frame, err)
	}
}

func TestRevivalUnsupportedClosuresRejectBeforeRandomOrDelegatedExecution(t *testing.T) {
	s, c, d := revivalRegistryFixture()
	d.RegisterRevivals(&c)
	d.Invocations.JobTables[3] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: "job-3-entry-6", Name: "献命回復"}}}}
	s.Actors[0].Jobs.JobID = "job-3"
	d.Invocations.Entries["delegated"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen"}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{{"gem-66"}, {"job-7-entry-8"}, {"delegated"}, {"random"}, {"gem-63", "gem-64"}, {"tool-110"}} {
		if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		var ops []corebattle.CallbackInvocation
		for _, id := range ids {
			ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("rejected closure mutated state")
	}
}

func TestSacrificeAutomaticRevivalControlsLaterSlotsAndCMP(t *testing.T) {
	for _, protect := range []bool{false, true} {
		s, c, d := revivalRegistryFixture()
		s.Actors[0].CMP = 15
		if protect {
			s.Actors[0].Temporary = corebattle.TemporaryRevival
		}
		reset := func(id string) error {
			for i := range s.Actors {
				if s.Actors[i].ID == id {
					s.Actors[i].ResetEffects(corebattle.LifetimeReset{})
				}
			}
			return nil
		}
		defeats := 0
		c.Defeat = func(id string) error {
			defeats++
			r, err := s.ApplyAutomaticRevival(&corebattle.AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: reset}, id, random.NewDeterministic(1))
			if err != nil || r.Prevented {
				return err
			}
			s.Actors[0].HP = 0
			return reset(id)
		}
		d.RegisterRevivals(&c)
		later := 0
		d.Invocations.Entries["later"] = corebattle.CallbackDefinition{
			Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
			Execute: func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext, _ random.Generator) error {
				later++
				if s.Actors[0].HP <= 0 || s.Actors[0].MP != 0 || s.Actors[0].CMP != 15 {
					t.Fatal(s)
				}
				s.Actors[0].CMP -= 5
				return nil
			},
		}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		ops, err := d.InvocationAction(&s, &frame, []string{"gem-66", "later"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
		wantLater, wantCMP := 0, 15
		if protect {
			wantLater, wantCMP = 1, 10
		}
		if later != wantLater || s.Actors[0].CMP != wantCMP || defeats != 1 || s.Actors[2].HP != 101 {
			t.Fatal(s, later, defeats)
		}
	}
}

func TestRevivalOrdinaryFallenAndBanGatesRetainIndirectBodySemantics(t *testing.T) {
	for _, helperHP := range []int{-1, 0, 1} {
		s, c, d := revivalRegistryFixture()
		s.Actors[0].HP = 0
		if helperHP >= 0 {
			s.Actors[2].RosterIdentity, s.Actors[2].HP = "calling-66", helperHP
		}
		d.RegisterRevivals(&c)
		if _, err := d.RevivalAction(&s, &c, "job-66-revive", "", noArithmeticDraws{t}); (err == nil) != (helperHP > 0) {
			t.Fatal(helperHP, err)
		}
		if _, err := d.RevivalAction(&s, &c, "gem-64", "fallen", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	s, c, d := revivalRegistryFixture()
	s.Actors[0].MP = 100
	s.Actors[1].RosterIdentity = "calling-66"
	s.SkillBans = map[string]int{"job-66-revive": 3}
	d.RegisterRevivals(&c)
	if _, err := d.RevivalAction(&s, &c, "job-66-revive", "fallen", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen"}}
	ops, err := d.InvocationAction(&s, &frame, []string{"job-66-revive"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[2].HP != 1 || s.Actors[0].MP != 60 {
		t.Fatal(s, err)
	}
}
