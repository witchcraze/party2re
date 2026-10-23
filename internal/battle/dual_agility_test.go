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

func TestDualAgilityUsesReloadedAdapterActorAndGear(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-83", SP: 100, Stats: corecharacter.Stats{HP: 1000, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 100, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1000, Gear: &corebattle.BattleGearStats{}}, Tension: 1, NPC: true}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[0].Jobs.JobID != "job-83" || loaded.Actors[0].Jobs.SP != 100 || loaded.Actors[0].Gear == nil || loaded.Actors[0].BaseStats.Attack != 100 {
		t.Fatal(loaded.Actors[0])
	}
	c := corebattle.DualAgilityContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	var definitions skill.BattleDefinitions
	definitions.RegisterDualAgility(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := definitions.InvocationAction(&loaded, &frame, []string{"job-83-entry-5"}, random.NewDeterministic(17))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&frame, definitions.Invocations, ops, random.NewDeterministic(17)); err != nil || loaded.Actors[1].HP >= 1000 || c.DamageCalls != 1 {
		t.Fatal(loaded, err)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("intrinsic effects wrote persistent character", chars.characters[char.ID])
	}
}
