package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func addGeneratedCallback(d *CallbackCatalog) {
	d.Entries["generated"] = CallbackDefinition{Invocation: CallbackInvocation{Kind: "job-82-generated"}}
	d.JobTables[82] = invocationTable(CallbackEntry{ID: "generated", RequiredSP: 10})
}

func TestInvocationGeneratedTableResiduesAndInternalMP(t *testing.T) {
	for sum := 3; sum < 180; sum++ {
		s, c, d := invocationFixture()
		addGeneratedCallback(&d)
		s.actor("self").Attack = sum - len(s.Actors)
		job := sum % 87
		if job == 0 || job == 73 || job == 82 {
			job = 8
		}
		called := false
		d.Entries["child"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(_ *LifetimeState, c *CallbackContext, _ random.Generator) error {
			if !reflect.DeepEqual(c.Arguments, []string{"ally", "tail"}) {
				t.Fatal(c)
			}
			called = true
			return nil
		}}
		entries := []CallbackEntry{{ID: "noop"}, {ID: "noop"}, {ID: "noop"}}
		entries[sum%3] = CallbackEntry{ID: "child", Name: "captured", Extended: true, RequiredSP: 999, MPCost: 20}
		d.JobTables[job] = invocationTable(entries...)
		s.actor("self").Jobs.JobID = "job-82"
		for _, mp := range []int{19, 20} {
			s.actor("self").MP, called = mp, false
			if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t, draws: []float64{0}}); err != nil || called != (mp == 20) || s.actor("self").MP != mp {
				t.Fatal(sum, mp, called, err)
			}
		}
	}
}

func TestInvocationGeneratedHiddenExtendedChildBlocksRandom(t *testing.T) {
	s, c, d := invocationFixture()
	addGeneratedCallback(&d)
	d.JobTables[2] = invocationTable(CallbackEntry{ID: "noop"}, CallbackEntry{ID: "missing", Extended: true})
	before := reloadLifetime(t, s)
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
		t.Fatal(s, err)
	}
}

func TestInvocationRecursiveRandomRetainsRemainingArguments(t *testing.T) {
	s, c, d := invocationFixture()
	var args []string
	d.Entries["consume"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(_ *LifetimeState, c *CallbackContext, _ random.Generator) error {
		args = append(args, c.Arguments[0])
		c.Arguments = c.Arguments[1:]
		return nil
	}}
	d.Entries["recursive"] = CallbackDefinition{Validate: d.Entries["noop"].Validate,
		Children: []CallbackInvocation{{Kind: "random"}}, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			if err := c.Invoke(CallbackInvocation{Kind: "random"}); err != nil {
				return err
			}
			return c.Invoke(CallbackInvocation{Kind: "random"})
		}}
	d.JobTables[1] = invocationTable(CallbackEntry{ID: "recursive"})
	d.JobTables[2] = invocationTable(CallbackEntry{ID: "consume"})
	rng := &targetRNG{t: t, draws: []float64{0, 0, 1.1 / 87, 0, 1.1 / 87, 0}}
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, rng); err != nil {
		t.Fatal(err)
	}
	rng.done()
	if !reflect.DeepEqual(args, []string{"ally", "tail"}) {
		t.Fatal(args)
	}
}

func TestInvocationTableVariantsRebuiltAfterEarlierEffect(t *testing.T) {
	s, c, d := invocationFixture()
	s.actor("self").Jobs = BattleJobs{JobID: "job-59"}
	var selected []int
	for i := range 2 {
		id := []string{"before", "after"}[i]
		d.Entries[id] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			selected = append(selected, i)
			s.actor(c.ActorID).Attack++
			// Slot2 must receive its original target, not slot1's consumed argument.
			if c.Arguments[0] != "ally" {
				t.Fatal(c)
			}
			c.Arguments = nil
			return nil
		}}
	}
	d.JobTables[59] = CallbackTable{Variants: [][]CallbackEntry{
		{{ID: "before", Name: "attack"}}, {{ID: "after", Name: "attack"}},
	}, Select: func(s *LifetimeState, c *CallbackContext) int {
		if s.actor(c.ActorID).Attack > 1 {
			return 1
		}
		return 0
	}}
	ops := []CallbackInvocation{{Kind: "gem-89"}, {Kind: "gem-89"}}
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); err != nil || !reflect.DeepEqual(selected, []int{0, 1}) {
		t.Fatal(selected, err)
	}
	// An unsupported alternate must reject before the first slot changes stats.
	s.actor("self").Attack = 1
	delete(d.Entries, "after")
	if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || s.actor("self").Attack != 1 {
		t.Fatal(s, err)
	}
}

func TestInvocationErrorsAndMalformedDefinitions(t *testing.T) {
	s, c, d := invocationFixture()
	failure := errors.New("asset transaction failed")
	d.Entries["error"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return failure }}
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "entry", EntryID: "error"}}, &targetRNG{t: t}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for _, def := range []CallbackDefinition{
		{}, {Validate: d.Entries["noop"].Validate}, {Execute: d.Entries["noop"].Execute},
		{Invocation: CallbackInvocation{Kind: "entry", EntryID: "bad"}},
		{Invocation: CallbackInvocation{Kind: "random"}, Execute: d.Entries["noop"].Execute},
	} {
		d.Entries["bad"] = def
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "entry", EntryID: "bad"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	d.JobTables[1] = CallbackTable{Variants: [][]CallbackEntry{{{ID: "noop"}}}, Select: func(*LifetimeState, *CallbackContext) int { return 3 }}
	if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
