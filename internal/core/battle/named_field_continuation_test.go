package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestNamedFieldSavedTailAndQueueMatchUninterrupted(t *testing.T) {
	for _, field := range []string{"burn", "grave", "library", "darkness"} {
		s, roster := rosterFixture()
		helper := addRosterHelper(t, &s, &roster, 64)
		c := NamedFieldContext{DamageContext: roster.DamageContext, Roster: &roster}
		s.Fields = LifetimeFields{Temporary: field, Turns: 1}
		tail := NamedFieldTail{Before: field, ResponseEnabled: true, Grave: s.SnapshotGrave()}
		s.actor(helper).HP, s.actor(helper).MP = 0, 1
		c.End.RecordDarkness(80)
		if field == "darkness" {
			c.End.ImmediateRiskCalls = 1
		}
		// Save only facts and pending typed work. Function-bearing contexts are
		// reconstructed by the consumer; the accepted action is never replayed.
		data, err := json.Marshal(struct {
			State  LifetimeState
			Target TargetContext
			Tail   NamedFieldTail
			End    EndProcessing
		}{s, c.TargetContext, tail, c.End})
		if err != nil {
			t.Fatal(err)
		}
		var loaded struct {
			State  LifetimeState
			Target TargetContext
			Tail   NamedFieldTail
			End    EndProcessing
		}
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		lc := NamedFieldContext{DamageContext: DamageContext{EffectContext: EffectContext{TargetContext: loaded.Target}, End: loaded.End}, Roster: &roster}
		finish := func(state *LifetimeState, context *NamedFieldContext, tail NamedFieldTail) int {
			draws := []float64(nil)
			if field == "library" {
				draws = []float64{0, 0}
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := state.ApplyNamedFieldTail(context, tail, rng); err != nil {
				t.Fatal(err)
			}
			state.Fields.FinishOperation(true, false)
			context.End.Flush(state.actor(context.ActorID))
			rng.done()
			return rng.used
		}
		first, second := finish(&s, &c, tail), finish(&loaded.State, &lc, loaded.Tail)
		if !reflect.DeepEqual(s, loaded.State) || first != second || s.Fields.Temporary != "" {
			t.Fatal(field, s, loaded.State, first, second)
		}
	}
}

func TestNamedFieldFortressFactoryRejectsOrderedMissingDescendant(t *testing.T) {
	s, c := twoStrikeFixture()
	s.actor("self").HP, s.actor("self").MaxHP = 5000, 1
	s.actor("self").Gear.AccessoryID = "item-110"
	s.actor("self").ItemDefinitionIDs = []string{"item-110"}
	ally := *s.actor("self")
	ally.ID, ally.HP, ally.MaxHP = "ally", 1, 100
	ally.Gear, ally.ItemDefinitionIDs = &BattleGearStats{}, nil
	s.Actors = append(s.Actors, ally)
	c.PartyIDs = append(c.PartyIDs, ally.ID)
	c.Defeat.TargetContext, c.Defeat.FieldAtLoad = c.TargetContext, "fortress"
	c.Defeat.Fortress = FortressDefeat(&targetRNG{t: t})
	before := reloadLifetime(t, s)
	if err := s.ValidateStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

func TestNamedFieldBurnRejectsBeforeCreationAndReset(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("fallen").Gear.AccessoryID = "item-110"
	before := reloadLifetime(t, s)
	if err := s.ApplyNamedField(&c, "job-84-entry-4", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DamageCalls != 0 {
		t.Fatal(err, s, c)
	}
}

func TestNamedFieldInvocationProjectionOwnsOperationFacts(t *testing.T) {
	s, c := namedFieldFixture()
	c.End.RecordDarkness(80)
	frame := CallbackContext{TargetContext: c.TargetContext, Damage: &c.DamageContext}
	d := CallbackCatalog{Entries: map[string]CallbackDefinition{
		"prepare": {
			Validate: func(*LifetimeState, *CallbackContext) error { return nil },
			Execute:  func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
			Project: func(_ *LifetimeState, f *CallbackContext) (bool, error) {
				f.Damage.DamageCalls = 7
				f.Damage.PartyIDs[0] = "private"
				*f.Damage.RequesterLevel = 1
				*f.Damage.End.DarknessDebit = 3
				return true, nil
			},
		},
		"observe": {
			Validate: func(_ *LifetimeState, f *CallbackContext) error {
				if f.Damage.DamageCalls != 7 || f.Damage.PartyIDs[0] != "private" || *f.Damage.RequesterLevel != 1 || *f.Damage.End.DarknessDebit != 3 {
					t.Fatal(f.Damage)
				}
				return nil
			},
			Execute: func(*LifetimeState, *CallbackContext, random.Generator) error { return nil },
		},
	}}
	if err := s.ValidateInvocations(&frame, d, []CallbackInvocation{{Kind: "entry", EntryID: "prepare"}, {Kind: "entry", EntryID: "observe"}}, &targetRNG{t: t}); err != nil || c.DamageCalls != 0 || c.PartyIDs[0] != "self" || *c.RequesterLevel != 70 || *c.End.DarknessDebit != 40 {
		t.Fatal(err, c)
	}
}
