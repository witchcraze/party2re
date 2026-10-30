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

func TestAdvancedMagicUsesReloadedAdapterFacts(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-15", SP: 130, Stats: corecharacter.Stats{HP: 500, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 100, Defense: 80, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Tension = "party", 6
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 10000, MaxHP: 10000, Gear: &corebattle.BattleGearStats{}}, Tension: 1, NPC: true}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[0].Jobs.JobID != "job-15" || loaded.Actors[0].Jobs.SP != 130 || loaded.Actors[0].Gear == nil || loaded.Actors[0].BaseStats.Defense != 80 {
		t.Fatal(loaded)
	}
	c := &corebattle.AdvancedMagicContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}, Defeat: func(string) error { t.Fatal("nonlethal vector"); return nil }}}
	var d skill.BattleDefinitions
	d.RegisterAdvancedMagic(c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	rng := random.NewDeterministic(1461)
	ops, err := d.InvocationAction(&loaded, &frame, []string{"job-15-entry-7"}, rng)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[1].HP < 3400 || loaded.Actors[1].HP > 5050 || loaded.Actors[0].MP != 80 || loaded.Actors[0].Tension != 1 || c.DamageCalls != 1 {
		t.Fatal(loaded, c)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("intrinsic callback wrote persistent character")
	}
}
