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

func TestCounterResetUsesReloadedAdapterBaselines(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-37", SP: 100, Stats: corecharacter.Stats{HP: 1000, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 100, Defense: 80, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Attack, a.Defense, a.Tension = "party", 300, 240, 5
	a.Temporary = corebattle.TemporaryDeflect
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 900, Gear: &corebattle.BattleGearStats{}}, Tension: 1, Temporary: corebattle.TemporaryReflectAttack, NPC: true}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[0].Jobs.JobID != "job-37" || loaded.Actors[0].Jobs.SP != 100 || loaded.Actors[0].Gear == nil {
		t.Fatal(loaded)
	}
	c := corebattle.CounterResetContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}}}
	var definitions skill.BattleDefinitions
	definitions.RegisterCounterReset(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	ops, err := definitions.InvocationAction(&loaded, &f, []string{"job-37-entry-7"}, random.NewDeterministic(17))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&f, definitions.Invocations, ops, random.NewDeterministic(17)); err != nil {
		t.Fatal(err)
	}
	a = loaded.Actors[0]
	if a.Attack != 100 || a.Defense != 80 || a.BaseStats.Attack != 100 || a.BaseStats.Defense != 80 || a.Temporary != "" || a.Tension != 1 || a.HP != 1000 || a.MP != 80 || c.StatDownCalls != 0 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
		t.Fatal(a, c)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("intrinsic body wrote persistent character", chars.characters[char.ID])
	}
}
