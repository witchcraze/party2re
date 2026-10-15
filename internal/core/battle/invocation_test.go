package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func invocationFixture() (LifetimeState, CallbackContext, CallbackCatalog) {
	s := LifetimeState{Actors: []LifetimeActor{
		{Participant: Participant{ID: "self", TeamID: "party", HP: 100, MP: 20, Attack: 1}, Jobs: BattleJobs{JobID: "job-1"}},
		{Participant: Participant{ID: "ally", TeamID: "party", HP: 100, MP: 20}},
		{Participant: Participant{ID: "enemy", TeamID: "enemy", HP: 100}},
	}}
	c := CallbackContext{TargetContext: TargetContext{ActorID: "self", PartyIDs: []string{"self", "ally"}, EnemyIDs: []string{"enemy"}}, Arguments: []string{"ally", "tail"}}
	d := CallbackCatalog{JobTables: map[int]CallbackTable{}, Entries: map[string]CallbackDefinition{}}
	d.Entries["noop"] = CallbackDefinition{
		Validate: func(*LifetimeState, *CallbackContext) error { return nil },
		Execute:  func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
	}
	for i := 1; i <= 87; i++ {
		if i != 73 {
			d.JobTables[i] = invocationTable(CallbackEntry{ID: "noop", Name: "ordinary"})
		}
	}
	d.JobTables[300] = d.JobTables[1]
	return s, c, d
}

func invocationTable(entries ...CallbackEntry) CallbackTable {
	return CallbackTable{Variants: [][]CallbackEntry{entries}}
}

func TestInvocationTableDrawsRedrawAndDuplicateNames(t *testing.T) {
	s, c, d := invocationFixture()
	var selected []string
	for _, id := range []string{"first", "second"} {
		d.Entries[id] = CallbackDefinition{Validate: d.Entries["noop"].Validate,
			Execute: func(_ *LifetimeState, c *CallbackContext, _ random.Generator) error {
				selected = append(selected, id+":"+c.Arguments[0])
				c.Arguments = c.Arguments[1:]
				return nil
			}}
	}
	d.JobTables[1] = invocationTable(CallbackEntry{ID: "first", Name: "duplicate", RequiredSP: 999, MPCost: 999}, CallbackEntry{ID: "second", Name: "duplicate"}, CallbackEntry{ID: "missing", Extended: true})
	d.JobTables[2] = invocationTable(CallbackEntry{ID: "second", Name: "duplicate"})
	// Generated job82 can reach extended entries, so its full closure needs this too.
	d.Entries["missing"] = d.Entries["noop"]
	rng := &targetRNG{t: t, draws: []float64{0, .99, .5, 0, 0, 1.1 / 87, 0}}
	for range 3 {
		c.Arguments = []string{"ally", "tail"}
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, rng); err != nil {
			t.Fatal(err)
		}
	}
	rng.done()
	if !reflect.DeepEqual(selected, []string{"second:ally", "first:ally", "second:ally"}) || s.actor("self").MP != 20 {
		t.Fatal(selected, s)
	}
}

func TestInvocationClosureRejectsBeforeAnyMutationOrDraw(t *testing.T) {
	for _, edge := range []string{"random", "tool-84", "entry"} {
		s, c, d := invocationFixture()
		d.Entries["outer"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: d.Entries["noop"].Execute,
			Children: []CallbackInvocation{{Kind: edge, EntryID: "missing"}}}
		d.JobTables[1] = invocationTable(CallbackEntry{ID: "outer"})
		// Even a hidden generated job82 child or temporary-job tool child blocks all slots.
		if edge == "random" {
			d.JobTables[2] = invocationTable(CallbackEntry{ID: "noop"}, CallbackEntry{ID: "missing"})
		} else if edge == "tool-84" {
			d.JobTables[300] = invocationTable(CallbackEntry{ID: "missing"})
		}
		d.Entries["mutate"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, _ *CallbackContext, _ random.Generator) error {
			s.actor("self").CMP--
			s.actor("self").ConsumedItems = append(s.actor("self").ConsumedItems, ConsumedItem{})
			return nil
		}}
		before := reloadLifetime(t, s)
		ops := []CallbackInvocation{{Kind: "entry", EntryID: "mutate"}, {Kind: "entry", EntryID: "outer"}}
		if err := s.ApplyInvocations(&c, d, ops, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(edge, s, err)
		}
	}
}

func TestInvocationRandomMPOnlyRestoresActorDecrease(t *testing.T) {
	for _, after := range []int{0, 35} {
		s, c, d := invocationFixture()
		d.Entries["effect"] = CallbackDefinition{Validate: d.Entries["noop"].Validate, Execute: func(s *LifetimeState, c *CallbackContext, _ random.Generator) error {
			s.actor(c.ActorID).MP, s.actor(c.ActorID).HP = after, 0
			s.actor("ally").MP = 3
			s.DroppedCrystals++
			s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "helper", TeamID: "party", HP: 1}})
			c.PartyIDs = append(c.PartyIDs, "helper")
			return nil
		}}
		d.JobTables[1] = invocationTable(CallbackEntry{ID: "effect", MPCost: 999, RequiredSP: 999})
		if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}, {Kind: "entry", EntryID: "effect"}}, &targetRNG{t: t, draws: []float64{0, 0}}); err != nil {
			t.Fatal(err)
		}
		if s.actor("self").MP != max(20, after) || s.actor("self").HP != 0 || s.actor("ally").MP != 3 || s.DroppedCrystals != 1 || len(s.Actors) != 4 {
			t.Fatal(s)
		}
	}
}

func TestInvocationEmptyAndAllExtendedTables(t *testing.T) {
	s, c, d := invocationFixture()
	d.JobTables[1] = invocationTable()
	if err := s.ApplyInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, &targetRNG{t: t, draws: []float64{0}}); err != nil {
		t.Fatal(err)
	}
	// Tool84 has no source empty-table success; neither it nor all-ex random is repaired.
	if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "tool-84"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	d.JobTables[1] = invocationTable(CallbackEntry{ID: "noop", Extended: true})
	if err := s.ValidateInvocations(&c, d, []CallbackInvocation{{Kind: "random"}}, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
