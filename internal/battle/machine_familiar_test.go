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

func TestMachineFamiliarUsesReloadedAdapterStats(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-79", SP: 99, Stats: corecharacter.Stats{HP: 1000, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 10, Defense: 80, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Agility = "party", 100
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1000, Gear: &corebattle.BattleGearStats{}}, Tension: 1, NPC: true}}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	c := corebattle.MachineFamiliarContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	var d skill.BattleDefinitions
	d.RegisterMachineFamiliar(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&loaded, &f, []string{"job-79-entry-1"}, random.NewDeterministic(17))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&f, d.Invocations, ops, random.NewDeterministic(17)); err != nil {
		t.Fatal(err)
	}
	// Current AG100×2.2 gives physical99..131; AT10/baseAG40 are not inputs.
	damage := 1000 - loaded.Actors[1].HP
	if damage < 99 || damage > 131 || c.DamageCalls != 1 || loaded.Actors[0].Jobs.JobID != "job-79" || loaded.Actors[0].BaseStats.Agility != 40 || loaded.Actors[0].Gear == nil || !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal(loaded, chars.characters[char.ID])
	}
}
