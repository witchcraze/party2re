package battle

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Synthetic leaves verify traversal only; native producer closure is separate.
func projectedNoop(d *CallbackCatalog) {
	def := d.Entries["noop"]
	def.Project = func(*LifetimeState, *CallbackContext) (bool, error) { return true, nil }
	d.Entries["noop"] = def
}

func TestInvocationWrapperProjectionRandomAlternativesAndMP(t *testing.T) {
	for _, kind := range []string{"random", "tool-84"} {
		s, c, d := invocationFixture()
		projectedNoop(&d)
		d.Entries["change"] = CallbackDefinition{
			Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
			ProjectBranches: func(s *LifetimeState, c *CallbackContext) ([]CallbackContinuation, error) {
				var branches []CallbackContinuation
				for _, mp := range []int{0, 35} {
					state, context := s.callbackCopy(), cloneCallbackContext(*c)
					state.actor("self").MP = mp
					state.actor("ally").MP = 3
					state.Actors = append(state.Actors, LifetimeActor{Participant: Participant{ID: "helper", TeamID: "party", HP: 1}})
					context.PartyIDs = append(context.PartyIDs, "helper")
					context.Arguments = context.Arguments[1:]
					context.ArithmeticDivisor = 7
					branches = append(branches, CallbackContinuation{State: state, Context: context})
				}
				return branches, nil
			},
		}
		d.JobTables[1] = invocationTable(CallbackEntry{ID: "change"}, CallbackEntry{ID: "change"}, CallbackEntry{ID: "noop", Extended: true})
		// The extended entry is admitted but has no projection. Random must redraw it.
		d.Entries["extended"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute}
		if kind == "random" {
			d.JobTables[1].Variants[0][2].ID = "extended"
		}
		counts := map[int]int{}
		d.Entries["later"] = CallbackDefinition{
			Validate: func(s *LifetimeState, c *CallbackContext) error {
				if c.AfterEffects || !reflect.DeepEqual(c.Arguments, []string{"ally", "tail"}) {
					t.Fatal("lost certified facts/original arguments", kind, c)
				}
				if c.ArithmeticDivisor == 7 {
					if s.actor("ally").MP != 3 || len(s.Actors) != 4 || len(c.PartyIDs) != 3 {
						t.Fatal("lost roster/resource continuation", s, c)
					}
					counts[s.actor("self").MP]++
				}
				return nil
			}, Execute: d.Entries["noop"].Execute,
		}
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: kind}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
			t.Fatal(kind, err)
		}
		want := 2
		if kind == "random" {
			want = 4 // Job73 proxies table1 for AT1, preserving its positions too.
		}
		if !reflect.DeepEqual(counts, map[int]int{20: want, 35: want}) || s.actor("self").MP != 20 || s.actor("ally").MP != 20 || len(s.Actors) != 3 || len(c.PartyIDs) != 2 {
			t.Fatal("wrong MP restoration or shared alternative", kind, counts, s, c)
		}
	}
}

func TestInvocationWrapperProjectionUnknownAndCyclesAreAllOrUnknown(t *testing.T) {
	for _, kind := range []string{"random", "tool-84"} {
		for _, cycle := range []bool{false, true} {
			s, c, d := invocationFixture()
			projectedNoop(&d)
			unknown := d.Entries["noop"]
			unknown.Project = func(s *LifetimeState, c *CallbackContext) (bool, error) {
				s.actor("self").MP = 0
				c.Arguments[0] = "changed"
				return false, nil
			}
			if cycle {
				unknown = CallbackDefinition{Invocation: CallbackInvocation{Kind: kind}}
			}
			d.Entries["unknown"] = unknown
			d.JobTables[1] = invocationTable(CallbackEntry{ID: "noop"}, CallbackEntry{ID: "unknown"})
			d.Entries["later"] = CallbackDefinition{
				Validate: func(s *LifetimeState, c *CallbackContext) error {
					if !c.AfterEffects || s.actor("self").MP != 20 || c.Arguments[0] != "ally" {
						t.Fatal("partial projection escaped", kind, cycle, s, c)
					}
					return ErrUnsupportedEffect
				}, Execute: d.Entries["noop"].Execute,
			}
			before := s.callbackCopy()
			if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: kind}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.Arguments[0] != "ally" {
				t.Fatal(kind, cycle, err, s, c)
			}
		}
	}
}

func TestInvocationWrapperProjectionExtendedNumericBoundaries(t *testing.T) {
	for _, row := range []struct {
		mp, attack, wantMP, wantAttack int
	}{{0, 998, 0, 998}, {1, 998, 0, 1000}, {2, 999, 1, 999}} {
		s, c, d := invocationFixture()
		s.actor("self").Jobs = BattleJobs{JobID: "job-73", OldJobID: "job-59", OldSP: 999}
		s.actor("self").MP, s.actor("self").Attack = row.mp, row.attack
		d.JobTables[59] = invocationTable(CallbackEntry{ID: "missing", Extended: true})
		d.Entries["later"] = CallbackDefinition{
			Validate: func(s *LifetimeState, c *CallbackContext) error {
				if c.AfterEffects || s.actor("self").MP != row.wantMP || s.actor("self").Attack != row.wantAttack {
					t.Fatal(row, s, c)
				}
				return nil
			}, Execute: d.Entries["noop"].Execute,
		}
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "gem-86"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
			t.Fatal(row, err)
		}
	}
	for _, row := range []struct {
		sp, mp, after, want int
	}{{9, 20, 35, 20}, {10, 4, 35, 4}, {10, 5, 35, 30}, {10, 5, 1, 0}} {
		s, c, d := invocationFixture()
		s.actor("self").Jobs = BattleJobs{JobID: "job-1", OldJobID: "job-2", OldSP: row.sp}
		s.actor("self").MP = row.mp
		d.JobTables[2] = invocationTable(CallbackEntry{ID: "extended", Extended: true, RequiredSP: 10, MPCost: 5})
		d.Entries["extended"] = CallbackDefinition{
			Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
			Project: func(s *LifetimeState, c *CallbackContext) (bool, error) {
				if !reflect.DeepEqual(c.Arguments, []string{"ally"}) {
					t.Fatal(c)
				}
				s.actor("self").MP = row.after
				return true, nil
			},
		}
		d.Entries["later"] = CallbackDefinition{
			Validate: func(s *LifetimeState, c *CallbackContext) error {
				if c.AfterEffects || s.actor("self").MP != row.want {
					t.Fatal(row, s, c)
				}
				return nil
			}, Execute: d.Entries["noop"].Execute,
		}
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "gem-86"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
			t.Fatal(row, err)
		}
	}
}

func TestInvocationWrapperProjectionGeneratedGuardAndCapture(t *testing.T) {
	for _, captured := range []bool{false, true} {
		for _, mp := range []int{19, 20} {
			s, c, d := invocationFixture()
			addGeneratedCallback(&d)
			s.actor("self").MP = mp
			// S=4: generated table4, position1. A frozen binding below retains
			// a different child even though these construction facts now select4.
			d.JobTables[4] = invocationTable(CallbackEntry{ID: "noop"}, CallbackEntry{ID: "live", MPCost: 20, Extended: true}, CallbackEntry{ID: "noop"})
			for _, id := range []string{"live", "frozen"} {
				divisor := 3
				if id == "frozen" {
					divisor = 7
				}
				d.Entries[id] = CallbackDefinition{
					Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
					Project: func(s *LifetimeState, c *CallbackContext) (bool, error) {
						c.ArithmeticDivisor = divisor
						return true, nil
					},
				}
			}
			op := CallbackInvocation{Kind: "job-82-generated"}
			if captured {
				op = CallbackInvocation{Kind: "entry", EntryID: "generated", GeneratedChildID: "frozen", GeneratedChildMP: 20}
			}
			d.Entries["later"] = CallbackDefinition{
				Validate: func(s *LifetimeState, c *CallbackContext) error {
					want := 0
					if mp == 20 {
						want = 3
						if captured {
							want = 7
						}
					}
					if c.AfterEffects || c.ArithmeticDivisor != want || s.actor("self").MP != mp {
						t.Fatal(captured, mp, s, c)
					}
					return nil
				}, Execute: d.Entries["noop"].Execute,
			}
			if err := s.ValidateInvocations(&c, d, []CallbackInvocation{op, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
				t.Fatal(captured, mp, err)
			}
		}
	}
}

func TestInvocationWrapperProjectionExplorationCounts(t *testing.T) {
	s, c, d := invocationFixture()
	projectedNoop(&d)
	visits := 0
	d.Entries["later"] = CallbackDefinition{
		Validate: func(_ *LifetimeState, c *CallbackContext) error {
			if c.AfterEffects {
				t.Fatal("unknown transition")
			}
			visits++
			return nil
		}, Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
	}
	start := time.Now()
	if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "random"}, {Kind: "random"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	if visits != 87*87 {
		t.Fatal(visits)
	}
	t.Logf("two complete job-domain traversals: %d later admissions in %s (synthetic leaves only)", visits, time.Since(start))
}

func TestInvocationWrapperProjectionCurrent300AndLaterRejection(t *testing.T) {
	s, c, d := invocationFixture()
	projectedNoop(&d)
	switchJob := d.Entries["noop"]
	switchJob.Project = func(s *LifetimeState, c *CallbackContext) (bool, error) {
		s.actor(c.ActorID).Jobs.JobID = "job-300"
		return true, nil
	}
	d.Entries["switch"] = switchJob
	grow := d.Entries["noop"]
	grow.Project = func(s *LifetimeState, c *CallbackContext) (bool, error) {
		s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "helper", TeamID: "party", HP: 1}})
		c.PartyIDs = append(c.PartyIDs, "helper")
		s.actor(c.ActorID).MP = 0
		return true, nil
	}
	d.Entries["grow"] = grow
	d.JobTables[300] = invocationTable(CallbackEntry{ID: "grow", Extended: true})
	d.Entries["later"] = CallbackDefinition{
		Validate: func(s *LifetimeState, c *CallbackContext) error {
			if c.AfterEffects || s.actor(c.ActorID).Jobs.JobID != "job-300" || s.actor(c.ActorID).MP != 20 || len(s.Actors) != 4 || len(c.PartyIDs) != 3 {
				t.Fatal("lost current300 or roster transition", s, c)
			}
			return nil
		}, Execute: d.Entries["noop"].Execute,
	}
	ops := []CallbackInvocation{{Kind: "entry", EntryID: "switch"}, {Kind: "tool-84"}, {Kind: "entry", EntryID: "later"}}
	if err := s.ValidateInvocations(&c, d, ops, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	delete(d.Entries, "grow")
	before := s.callbackCopy()
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || len(c.PartyIDs) != 2 {
		t.Fatal("unsupported later form child mutated input", err, s, c)
	}
}

func TestInvocationWrapperProjectionDeathAndEmptyTableNoEffect(t *testing.T) {
	s, c, d := invocationFixture()
	dead := d.Entries["noop"]
	dead.Project = func(s *LifetimeState, c *CallbackContext) (bool, error) {
		s.actor(c.ActorID).HP = 0
		return true, nil
	}
	d.Entries["noop"] = dead
	ops := []CallbackInvocation{{Kind: "random"}, {Kind: "entry", EntryID: "missing"}}
	if err := s.ValidateInvocations(&c, d, ops, &targetRNG{t: t}); err != nil {
		t.Fatal("all alternatives die before missing slot", err)
	}
	// An empty random table is a living unchanged branch, not an omitted path.
	d.JobTables[2] = invocationTable()
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || s.actor("self").HP != 100 {
		t.Fatal("living no-effect alternative skipped the later slot", err, s)
	}
}
