package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func assertAdvancedMagicRejected(t *testing.T, s *corebattle.LifetimeState, c *corebattle.AdvancedMagicContext, d BattleDefinitions, ids ...string) {
	t.Helper()
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	original := f
	if _, err := d.InvocationAction(s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(ids, err)
	}
	ops := make([]corebattle.CallbackInvocation, len(ids))
	for i, id := range ids {
		ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(ids, err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !reflect.DeepEqual(f, original) || c.DamageCalls != 0 || c.Status.DeathCalls != 0 || c.Arguments != nil {
		t.Fatal(s, c, f)
	}
}

func TestAdvancedMagicDefaultEmptyAndWholeActionRejection(t *testing.T) {
	s, c, d := advancedMagicFixture()
	for _, definitions := range []BattleDefinitions{{}, func() BattleDefinitions { var d BattleDefinitions; d.RegisterAdvancedMagic(nil); return d }()} {
		assertAdvancedMagicRejected(t, &s, c, definitions, "job-15-entry-1")
	}
	assertAdvancedMagicRejected(t, &s, c, d, "job-16-entry-4", "unbound")
	// A supported registration cannot claim arbitrary unprojected prior effects.
	assertAdvancedMagicRejected(t, &s, c, d, "job-15-entry-1", "job-16-entry-8")
}

func TestAdvancedMagicPoisonAdmitsNewlyActivatedConfusionTargets(t *testing.T) {
	s, c, d := advancedMagicFixture()
	s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
	s.Actors[2].HP, s.Actors[2].Gear = 50, nil
	assertAdvancedMagicRejected(t, &s, c, d, "job-15-entry-0")
}

func TestAdvancedMagicLoopAdmitsDeathAfterHPBoundaryCrossing(t *testing.T) {
	s, c, d := advancedMagicFixture()
	s.Actors[1].HP = 1100
	c.Status.Defeat = nil
	// First damage cannot kill at tension1, but it makes later instant death
	// reachable. Reject before the first target/power draw or HP change.
	assertAdvancedMagicRejected(t, &s, c, d, "job-19-entry-8")
}

func TestAdvancedMagicLoopProjectsRewardsBeforeLaterBranch(t *testing.T) {
	s, c, d := advancedMagicFixture()
	c.Status.Defeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
		t.Fatal("Apply in admission")
		return nil
	}
	c.Status.Defeat.Rewards.Project = func(p *corebattle.LifetimeState, _ corebattle.TargetContext, _ string) error {
		p.Actors[1].HP = 2000
		p.Actors[1].Gear = nil
		return nil
	}
	assertAdvancedMagicRejected(t, &s, c, d, "job-19-entry-8")
}

func TestAdvancedMagicFullTableMissingSiblingRejection(t *testing.T) {
	s, c, d := advancedMagicFixture()
	s.Actors[0].Jobs.JobID = "job-16"
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	assertAdvancedMagicRejected(t, &s, c, d, "tool")
	s.Actors[0].Jobs.JobID = "job-19"
	delete(d.Invocations.Entries, "job-19-entry-7")
	assertAdvancedMagicRejected(t, &s, c, d, "tool")
}
