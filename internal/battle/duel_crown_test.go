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

func TestDuelCrownUsesReloadedAdapterFacts(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-74", SP: 150, Stats: corecharacter.Stats{HP: 1000, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 100, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1000, MaxHP: 1000, Attack: 100, Gear: &corebattle.BattleGearStats{}}, Tension: 1, NPC: true}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[0].Jobs.JobID != "job-74" || loaded.Actors[0].Jobs.SP != 150 || loaded.Actors[0].Gear == nil || loaded.Actors[0].BaseStats.Attack != 100 {
		t.Fatal(loaded)
	}
	c := &corebattle.DuelCrownContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	d := skill.BattleDefinitions{}
	d.RegisterDuelCrown(c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&loaded, &frame, []string{"job-74-entry-2"}, random.NewDeterministic(74))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&frame, d.Invocations, ops, random.NewDeterministic(74)); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[0].Attack != 100 && loaded.Actors[0].Attack != 120 || c.DamageCalls != 4 || loaded.Actors[1].HP >= 1000 || loaded.Actors[0].MP != 80 || loaded.Actors[0].CMP != state.Actors[0].CMP {
		t.Fatal(loaded, c)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("intrinsic callback wrote persistent character")
	}
}
