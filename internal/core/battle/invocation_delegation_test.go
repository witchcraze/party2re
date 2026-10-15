package battle

import (
	"errors"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestInvocationExtendedHandledVersusFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		currentEx bool
		sp, mp    int
		want      string
		wantMP    int
	}{
		{"insufficient SP blocks old", true, 9, 20, "", 20},
		{"insufficient MP blocks old", true, 10, 4, "", 4},
		{"current executable", true, 10, 20, "current", 0},
		{"unhandled falls back", false, 10, 20, "previous", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, d := invocationFixture()
			a := s.actor("self")
			a.Jobs = BattleJobs{JobID: "job-1", SP: tc.sp, OldJobID: "job-2", OldSP: 100}
			a.MP = tc.mp
			called := ""
			for _, id := range []string{"current", "previous"} {
				d.Entries[id] = CallbackDefinition{Validate: func(_ *LifetimeState, c *CallbackContext) error {
					if len(c.Arguments) != 1 || c.Arguments[0] != "ally" {
						t.Fatal("explicit target was not preserved in admission", c)
					}
					return nil
				}, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
					called = id
					s.actor(c.ActorID).MP = 3
					return nil
				}}
			}
			// A prior extended entry is never inspected, including when unsupported.
			d.JobTables[1] = invocationTable(CallbackEntry{ID: "missing", Extended: true}, CallbackEntry{ID: "current", Extended: tc.currentEx, RequiredSP: 10, MPCost: 5})
			d.JobTables[2] = invocationTable(CallbackEntry{ID: "previous", Extended: true, MPCost: 7})
			if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "gem-86"}}, &targetRNG{t: t}); err != nil || called != tc.want || a.MP != tc.wantMP {
				t.Fatal(called, a, err)
			}
		})
	}
	// An unusable last extended callback needs neither an executor nor its context.
	s, c, d := invocationFixture()
	d.JobTables[1] = invocationTable(CallbackEntry{ID: "unsupported", Extended: true, RequiredSP: 100})
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "gem-86"}}, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
}

func TestInvocationExtendedJob73(t *testing.T) {
	for _, attack := range []int{998, 999, 1000} {
		for _, mp := range []int{0, 1} {
			s, c, d := invocationFixture()
			a := s.actor("self")
			a.Jobs, a.Attack, a.MP = BattleJobs{JobID: "job-73", OldJobID: "job-2", OldSP: 999}, attack, mp
			d.JobTables[2] = invocationTable(CallbackEntry{ID: "missing", Extended: true})
			// No SP, item admission or proxied table is required by this special case.
			d.JobTables = map[int]CallbackTable{2: d.JobTables[2]}
			if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "gem-86"}}, &targetRNG{t: t}); err != nil {
				t.Fatal(err)
			}
			want := attack
			if attack < 999 && mp > 0 {
				want += 2
			}
			if a.Attack != want || a.MP != 0 {
				t.Fatal(attack, mp, a)
			}
		}
	}
}

func TestInvocationCallingFirstMatchAndPreviousPreference(t *testing.T) {
	for source := 87; source <= 92; source++ {
		s, c, d := invocationFixture()
		s.actor("self").Jobs = BattleJobs{JobID: "job-59", OldJobID: "job-60"}
		kind := fmt.Sprintf("gem-%d", source)
		name, _ := callingCommand(kind)
		selected := ""
		for _, id := range []string{"first", "duplicate", "current"} {
			d.Entries[id] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(_ *LifetimeState, c *CallbackContext, _ random.Generator) error {
				if len(c.Arguments) != 1 || c.Arguments[0] != "ally" {
					t.Fatal(c)
				}
				selected = id
				return nil
			}}
		}
		d.JobTables[59] = invocationTable(CallbackEntry{ID: "current", Name: name})
		entries := []CallbackEntry{{ID: "first", Name: name, MPCost: 999, RequiredSP: 999}, {ID: "duplicate", Name: name}}
		if source == 92 {
			entries = append([]CallbackEntry{{ID: "missing", Name: "summon"}, {ID: "missing", Name: "excluded", Extended: true}}, entries...)
		}
		d.JobTables[60] = invocationTable(entries...)
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: kind}}, &targetRNG{t: t}); err != nil || selected != "first" || s.actor("self").MP != 20 {
			t.Fatal(source, selected, err)
		}
		// An absent command in a calling previous job still prevents current fallback.
		d.JobTables[60] = invocationTable()
		selected = ""
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: kind}}, &targetRNG{t: t}); err != nil || selected != "" {
			t.Fatal(source, selected, err)
		}
		// With no calling previous job, the current job supplies the command.
		s.actor("self").Jobs.OldJobID = "job-1"
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: kind}}, &targetRNG{t: t}); err != nil || selected != "current" {
			t.Fatal(source, selected, err)
		}
	}
}

func TestInvocationJob73ResiduesAndTool300Extended(t *testing.T) {
	for residue := range 73 {
		s, c, d := invocationFixture()
		s.actor("self").Jobs.JobID = "job-73"
		s.actor("self").Attack = residue
		want := residue
		if want == 0 {
			want = 8
		}
		called := 0
		for job := 1; job < 73; job++ {
			id := fmt.Sprintf("position-%d", job)
			d.Entries[id] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
				if s.actor(c.ActorID).Jobs.JobID != "job-73" || c.Arguments[0] != "ally" {
					t.Fatal("proxy replaced actor or arguments")
				}
				called = job
				return nil
			}}
			d.JobTables[job] = invocationTable(CallbackEntry{ID: id})
		}
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t, draws: []float64{0}}); err != nil || called != want {
			t.Fatal(residue, called, err)
		}
	}
	s, c, d := invocationFixture()
	s.actor("self").Jobs.JobID = "job-300"
	d.JobTables[300] = invocationTable(CallbackEntry{ID: "missing"}, CallbackEntry{ID: "noop", Extended: true})
	// Every tool position, including unselected ones, must be supported.
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.Entries["missing"] = d.Entries["noop"]
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t, draws: []float64{.99}}); err != nil {
		t.Fatal(err)
	}
}
