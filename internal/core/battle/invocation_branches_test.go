package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestInvocationBranchesValidateEveryLivingOutcome(t *testing.T) {
	for _, allDead := range []bool{false, true} {
		s, c, d := invocationFixture()
		before := s.callbackCopy()
		d.Entries["prior"] = CallbackDefinition{
			Validate: d.Entries["noop"].Validate,
			Execute: func(*LifetimeState, *CallbackContext, random.Generator) error {
				t.Fatal("rejected callback executed")
				return nil
			},
			ProjectBranches: func(s *LifetimeState, c *CallbackContext) ([]CallbackContinuation, error) {
				dead := s.callbackCopy()
				dead.actor(c.ActorID).HP = 0
				branches := []CallbackContinuation{{State: dead, Context: *c}}
				if !allDead {
					branches = append(branches, CallbackContinuation{State: *s, Context: *c})
				}
				return branches, nil
			},
		}
		ops := []CallbackInvocation{{Kind: "entry", EntryID: "prior"}, {Kind: "entry", EntryID: "missing"}}
		err := s.ValidateInvocations(&c, d, ops, &targetRNG{t: t})
		if allDead && err != nil || !allDead && !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(allDead, err, s)
		}
	}
}

func TestInvocationBranchesIsolateSiblingsAndNestedReferents(t *testing.T) {
	s, c, d := invocationFixture()
	level, debit, weapon, sp := 99, 7, 50, 30
	c.Damage = &DamageContext{EffectContext: EffectContext{TargetContext: c.TargetContext, RequesterLevel: &level}, End: EndProcessing{DarknessDebit: &debit}}
	s.Actors[0].Gear = &BattleGearStats{WeaponAttack: &weapon}
	s.Actors[0].JobHistory = map[string]*int{"job-01": &sp}
	s.SkillHistory = map[string]map[string]bool{"self": {"original": true}}
	d.Entries["fork"] = CallbackDefinition{
		Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
		ProjectBranches: func(s *LifetimeState, c *CallbackContext) ([]CallbackContinuation, error) {
			other := *c
			other.ArithmeticDivisor = 2
			// Deliberately share referents; admission owns sibling isolation.
			return []CallbackContinuation{{State: *s, Context: *c}, {State: *s, Context: other}}, nil
		},
	}
	d.Entries["change"] = CallbackDefinition{
		Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
		Project: func(s *LifetimeState, c *CallbackContext) (bool, error) {
			*s.actor("self").Gear.WeaponAttack++
			*s.actor("self").JobHistory["job-01"]++
			*c.Damage.RequesterLevel++
			*c.Damage.End.DarknessDebit++
			c.Damage.DamageCalls++
			s.SkillHistory["self"]["original"] = false
			c.Arguments = c.Arguments[1:]
			return true, nil
		},
	}
	visited := []int{}
	d.Entries["later"] = CallbackDefinition{
		Validate: func(s *LifetimeState, c *CallbackContext) error {
			if c.AfterEffects || *s.actor("self").Gear.WeaponAttack != 51 || *s.actor("self").JobHistory["job-01"] != 31 || *c.Damage.RequesterLevel != 100 || *c.Damage.End.DarknessDebit != 8 || c.Damage.DamageCalls != 1 || s.SkillHistory["self"]["original"] || !reflect.DeepEqual(c.Arguments, []string{"ally", "tail"}) {
				t.Fatal("incorrect predecessor or shared sibling", s, c)
			}
			visited = append(visited, c.ArithmeticDivisor)
			return nil
		},
		Execute: d.Entries["noop"].Execute,
	}
	ops := []CallbackInvocation{{Kind: "entry", EntryID: "fork"}, {Kind: "entry", EntryID: "change"}, {Kind: "entry", EntryID: "later"}}
	if err := s.ValidateInvocations(&c, d, ops, &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(visited, []int{0, 2}) || level != 99 || debit != 7 || weapon != 50 || sp != 30 || !s.SkillHistory["self"]["original"] || c.Damage.DamageCalls != 0 || !reflect.DeepEqual(c.Arguments, []string{"ally", "tail"}) {
		t.Fatal("admission mutated caller or skipped alternative", visited, s, c)
	}
	later := d.Entries["later"]
	later.Validate = func(_ *LifetimeState, c *CallbackContext) error {
		if c.ArithmeticDivisor == 2 {
			return ErrUnsupportedEffect
		}
		return nil
	}
	d.Entries["later"] = later
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || level != 99 || debit != 7 || weapon != 50 || sp != 30 || !s.SkillHistory["self"]["original"] || c.Damage.DamageCalls != 0 {
		t.Fatal("rejection changed an independently observed referent", err, s, c)
	}
}

func TestInvocationUnknownProjectionDiscardsTentativeWrites(t *testing.T) {
	for _, branched := range []bool{false, true} {
		s, c, d := invocationFixture()
		mutate := func(s *LifetimeState, c *CallbackContext) {
			s.actor("self").MP = 0
			c.Arguments[0] = "changed"
			c.ArithmeticDivisor = 9
		}
		prior := CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute}
		if branched {
			prior.ProjectBranches = func(s *LifetimeState, c *CallbackContext) ([]CallbackContinuation, error) {
				mutate(s, c)
				return nil, nil
			}
		} else {
			prior.Project = func(s *LifetimeState, c *CallbackContext) (bool, error) {
				mutate(s, c)
				return false, nil
			}
		}
		d.Entries["prior"] = prior
		d.Entries["later"] = CallbackDefinition{
			Validate: func(s *LifetimeState, c *CallbackContext) error {
				if !c.AfterEffects || s.actor("self").MP != 20 || c.ArithmeticDivisor != 0 || c.Arguments[0] != "ally" {
					t.Fatal("unknown projection leaked tentative facts", s, c)
				}
				return nil
			}, Execute: d.Entries["noop"].Execute,
		}
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "entry", EntryID: "prior"}, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
			t.Fatal(branched, err)
		}
	}
}

func TestInvocationBranchesFollowExistingExplicitAndGeneratedEdges(t *testing.T) {
	for _, kind := range []string{"entry", "gem-91", "captured-generated"} {
		s, c, d := invocationFixture()
		c.Arguments = []string{"enemy", "tail"}
		s.Actors[0].Jobs.OldJobID = "job-59"
		d.JobTables[59] = invocationTable(CallbackEntry{ID: "branch", Name: "defend"})
		d.Entries["branch"] = CallbackDefinition{
			Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
			ProjectBranches: func(s *LifetimeState, c *CallbackContext) ([]CallbackContinuation, error) {
				want := []string{"enemy", "tail"}
				if kind == "gem-91" {
					want = []string{"enemy"}
				}
				if !reflect.DeepEqual(c.Arguments, want) {
					t.Fatal(kind, c.Arguments)
				}
				c.ArithmeticDivisor = 13
				c.Arguments = nil
				return []CallbackContinuation{{State: *s, Context: *c}}, nil
			},
		}
		d.Entries["alias"] = CallbackDefinition{Invocation: CallbackInvocation{Kind: "gem-91"}}
		op := CallbackInvocation{Kind: "entry", EntryID: "alias"}
		if kind == "entry" {
			d.Entries["alias"] = d.Entries["branch"]
		} else if kind == "captured-generated" {
			d.Entries["alias"] = CallbackDefinition{Invocation: CallbackInvocation{Kind: "job-82-generated"}}
			op.GeneratedChildID, op.GeneratedChildMP = "branch", 1
		}
		d.Entries["later"] = CallbackDefinition{
			Validate: func(_ *LifetimeState, c *CallbackContext) error {
				if c.AfterEffects || c.ArithmeticDivisor != 13 || !reflect.DeepEqual(c.Arguments, []string{"enemy", "tail"}) {
					t.Fatal(kind, "lost projected context or original target", c)
				}
				return nil
			}, Execute: d.Entries["noop"].Execute,
		}
		if err := s.ValidateInvocations(&c, d, []CallbackInvocation{op, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil {
			t.Fatal(kind, err)
		}
	}
}

func TestInvocationBranchesResolveCurrentJobPerOutcome(t *testing.T) {
	s, c, d := invocationFixture()
	d.Entries["fork"] = CallbackDefinition{
		Validate: d.Entries["noop"].Validate,
		Execute: func(*LifetimeState, *CallbackContext, random.Generator) error {
			t.Fatal("unsupported later branch executed prior slot")
			return nil
		},
		ProjectBranches: func(s *LifetimeState, c *CallbackContext) ([]CallbackContinuation, error) {
			other := s.callbackCopy()
			other.actor(c.ActorID).Jobs.JobID = "job-2"
			return []CallbackContinuation{{State: *s, Context: *c}, {State: other, Context: *c}}, nil
		},
	}
	d.JobTables[2] = invocationTable(CallbackEntry{ID: "missing"})
	ops := []CallbackInvocation{{Kind: "entry", EntryID: "fork"}, {Kind: "tool-84"}}
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || s.actor("self").Jobs.JobID != "job-1" {
		t.Fatal(err, s)
	}
}

func TestInvocationRetainsNewOperationContext(t *testing.T) {
	// Synthetic callbacks check frame propagation only, not any concrete pool.
	for _, kind := range []string{"entry", "gem-87"} {
		s, c, d := invocationFixture()
		s.actor(c.ActorID).Jobs.OldJobID = "job-59"
		project := func(_ *LifetimeState, f *CallbackContext) (bool, error) {
			f.Damage = &DamageContext{EffectContext: EffectContext{TargetContext: f.TargetContext, TensionCalls: 1}}
			return true, nil
		}
		d.Entries["created"] = CallbackDefinition{
			Validate: d.Entries["noop"].Validate,
			Project:  project,
			Execute: func(s *LifetimeState, f *CallbackContext, _ random.Generator) error {
				_, err := project(s, f)
				return err
			},
		}
		d.JobTables[59] = invocationTable(CallbackEntry{ID: "created", Name: "summon"})
		check := func(_ *LifetimeState, f *CallbackContext) error {
			if f.AfterEffects || f.Damage == nil || f.Damage.TensionCalls != 1 {
				t.Fatal(kind, "lost newly created operation context", f)
			}
			return nil
		}
		d.Entries["later"] = CallbackDefinition{Validate: check, Execute: func(s *LifetimeState, f *CallbackContext, _ random.Generator) error { return check(s, f) }}
		op := CallbackInvocation{Kind: kind}
		if kind == "entry" {
			op.EntryID = "created"
		}
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{op, {Kind: "entry", EntryID: "later"}}, &targetRNG{t: t}); err != nil || c.Damage == nil || c.Damage.TensionCalls != 1 {
			t.Fatal(kind, err, c)
		}
	}
}
