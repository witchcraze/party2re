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

func TestSongDanceDrainUsesReloadedAdapterAttack(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-20", SP: 100, Stats: corecharacter.Stats{HP: 500, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 50, Defense: 50, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID, a.Attack = "party", 100
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "npc", HP: 1, MaxHP: 1000, Gear: &corebattle.BattleGearStats{}}, Tension: 1, NPC: true}}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Actors[0].Jobs.JobID != "job-20" || loaded.Actors[0].Jobs.SP != 100 || loaded.Actors[0].BaseStats.Attack != 50 || loaded.Actors[0].Gear == nil {
		t.Fatal(loaded.Actors[0])
	}
	c := corebattle.SongDanceDrainContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"enemy"}}}, GuaranteedHit: true, Defeat: func(string) error { return nil }}}
	var d skill.BattleDefinitions
	d.RegisterSongDanceDrain(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	ops, err := d.InvocationAction(&loaded, &f, []string{"job-20-entry-6"}, random.NewDeterministic(1460))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.ApplyInvocations(&f, d.Invocations, ops, random.NewDeterministic(1460)); err != nil {
		t.Fatal(err)
	}
	// CurrentAT100×1.7 returns76..101 damage despite target HP1. Recovery
	// uses floor50% of that return, then its own .9..1.2 roll:34..60HP.
	if loaded.Actors[1].HP != 0 || loaded.Actors[0].HP < 534 || loaded.Actors[0].HP > 560 || loaded.Actors[0].MP != 80 || c.DamageCalls != 1 || c.HealCalls != 1 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
		t.Fatal(loaded, c)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("intrinsic effect wrote persistent character", chars.characters[char.ID])
	}
}
