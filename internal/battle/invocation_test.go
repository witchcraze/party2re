package battle_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func TestInvocationUsesAdapterRunJobsAndRealTransformationOwner(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-82", SP: 0, Gender: "m", Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 1, MaxMP: 100, Attack: 10, Defense: 20, Agility: 30}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a}}
	c := corebattle.CallbackContext{TargetContext: corebattle.TargetContext{ActorID: char.ID, PartyIDs: []string{char.ID}}}
	rng := random.NewDeterministic(1357)
	d := skill.BattleDefinitions{Invocations: corebattle.CallbackCatalog{
		JobTables: map[int]corebattle.CallbackTable{}, Entries: map[string]corebattle.CallbackDefinition{
			"current-job": {Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}},
		},
	}}
	// These are explicitly synthetic subset tables, not production certification.
	for job, id := range map[int]string{82: "job-82-form", 300: "job-300-tune"} {
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{{{ID: id, RequiredSP: 999, MPCost: 999}}}}
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, c *corebattle.CallbackContext) error {
				ctx := corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext}, RequesterGender: char.Gender}
				return s.ValidateTransformation(&ctx, id, "", rng)
			},
			Execute: func(s *corebattle.LifetimeState, c *corebattle.CallbackContext, rng random.Generator) error {
				ctx := corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext}, RequesterGender: char.Gender}
				_, err := s.ApplyTransformation(&ctx, id, "", rng)
				return err
			},
		}
	}
	for range 2 {
		ops, err := d.InvocationAction(&s, &c, []string{"current-job"}, rng)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&c, d.Invocations, ops, rng); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
	}
	if s.Actors[0].Jobs.JobID != "job-300" || s.Actors[0].MP != 1 || !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("invocation changed permanent jobs/resources or charged ordinary child cost", s)
	}
}
