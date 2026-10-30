package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestAdvancedMagicReloadFrozenFactsWithoutReplay(t *testing.T) {
	s, c, d := advancedMagicFixture()
	c.Command, c.Custom = "frozen-advanced-magic", true
	c.Cursor = corebattle.TargetCursor{ActorID: "enemy", Source: "item159"}
	executeAdvancedMagic(t, &s, c, d, "job-15-entry-0", []string{"enemy"}, 0, 0)
	type checkpoint struct {
		State                    corebattle.LifetimeState
		Target                   corebattle.TargetContext
		Cursor                   corebattle.TargetCursor
		Entry                    string
		Arguments                []string
		DamageCalls, StatusCalls int
		AdditionalEffect         bool
	}
	frozen := checkpoint{s, c.TargetContext, c.Cursor, "job-19-entry-8", []string{"missing", "tail"}, c.DamageCalls, c.Status.StatusCalls, c.AdditionalEffect}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	executeAdvancedMagic(t, &s, c, d, frozen.Entry, frozen.Arguments, 0, 0, .99, 0, .99, 0, .99, 0, .99)
	_, rebuilt, _ := advancedMagicFixture()
	rebuilt.TargetContext, rebuilt.Cursor = loaded.Target, loaded.Cursor
	rebuilt.DamageCalls, rebuilt.Status.StatusCalls = loaded.DamageCalls, loaded.StatusCalls
	rebuilt.AdditionalEffect = loaded.AdditionalEffect
	var definitions BattleDefinitions
	definitions.RegisterAdvancedMagic(rebuilt)
	executeAdvancedMagic(t, &loaded.State, rebuilt, definitions, loaded.Entry, loaded.Arguments, 0, 0, .99, 0, .99, 0, .99, 0, .99)
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].HP != 873 || rebuilt.DamageCalls != 1 || rebuilt.Status.StatusCalls != 1 || rebuilt.Status.DeathCalls != 4 || rebuilt.Cursor != c.Cursor || rebuilt.Command != c.Command || !rebuilt.Custom || !reflect.DeepEqual(rebuilt.Arguments, frozen.Arguments) {
		t.Fatal(s, loaded.State, rebuilt)
	}
}

func TestAdvancedMagicIndirectCostsAndFullPools(t *testing.T) {
	s, c, d := advancedMagicFixture()
	s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-15", 0
	d.Invocations.Entries["tool"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	executeAdvancedMagic(t, &s, c, d, "tool", nil, 1.5/8, 0, 0)
	if s.Actors[0].MP != 0 || s.Actors[1].HP != 869 {
		t.Fatal(s)
	}
	// Only the scoped invocation fixture supplies unrelated empty tables. The
	// actual15/16/19 tables retain every original callback and descendant gate.
	d.RegisterOrdinaryTools(&corebattle.OrdinaryToolContext{DamageContext: c.DamageContext})
	d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	for job := 1; job <= 87; job++ {
		if _, exists := d.Invocations.JobTables[job]; !exists {
			d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{}}}
		}
	}
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	// Reject the extended index and redraw within the intact table.
	executeAdvancedMagic(t, &s, c, d, "random", nil, 14.5/87, 7.5/8, 1.5/8, 0, 0)
	if s.Actors[1].HP != 838 || s.Actors[0].MP != 0 {
		t.Fatal(s)
	}
}

func TestAdvancedMagicDelegatedExtendedCostAndNoEffect(t *testing.T) {
	for _, sp := range []int{129, 130} {
		s, c, d := advancedMagicFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP = "job-15", sp
		d.Invocations.Entries["extended"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "gem-86"}}
		draws := []float64{0}
		if sp == 129 {
			draws = nil
		}
		executeAdvancedMagic(t, &s, c, d, "extended", []string{"enemy", "self"}, draws...)
		if sp == 130 {
			if s.Actors[0].MP != 16 || s.Actors[1].HP != 450 || len(c.Arguments) != 0 {
				t.Fatal(s, c)
			}
		} else if s.Actors[0].MP != 80 || s.Actors[1].HP != 900 {
			t.Fatal(s)
		}
	}
}
