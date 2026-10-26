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

func TestBeastCloneUsesReloadedRequesterAndDistinctActor(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "requester", Level: 39, JobID: "job-26", SP: 110, Stats: corecharacter.Stats{HP: 1000, MaxHP: 1000, MP: 80, MaxMP: 100, Attack: 100, Defense: 80, Agility: 40}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a,
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 1000, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1},
		{Participant: corebattle.Participant{ID: "helper", TeamID: "party", HP: 20, MaxHP: 20, Status: corebattle.StatusSleep, Gear: &corebattle.BattleGearStats{}}, NPC: true, Tension: 1},
	}}
	type savedFacts struct {
		State           corebattle.LifetimeState
		RequesterLevel  int
		RequesterItemID string
	}
	data, err := json.Marshal(savedFacts{state, char.Level, "item-165"})
	if err != nil {
		t.Fatal(err)
	}
	var loaded savedFacts
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.State.Actors[0].Jobs.JobID != "job-26" || loaded.State.Actors[0].Jobs.SP != 110 || loaded.State.Actors[0].Gear == nil || loaded.State.Actors[0].BaseStats.Defense != 80 {
		t.Fatal(loaded)
	}
	c := corebattle.BeastCloneContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{
		TargetContext: corebattle.TargetContext{ActorID: "helper", PartyIDs: []string{"requester", "helper"}, EnemyIDs: []string{"enemy"}}, RequesterLevel: &loaded.RequesterLevel, ActorNPC: true,
	}}, RequesterItemID: loaded.RequesterItemID}
	var definitions skill.BattleDefinitions
	definitions.RegisterBeastClone(&c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := definitions.InvocationAction(&loaded.State, &f, []string{"job-26-entry-8"}, random.NewDeterministic(17))
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.State.ApplyInvocations(&f, definitions.Invocations, ops, random.NewDeterministic(17)); err != nil {
		t.Fatal(err)
	}
	h := loaded.State.Actors[3]
	if h.SummonForm != 0 || !h.NPC || h.Status != corebattle.StatusSleep || h.ExperienceReward != 19 || h.GoldReward != 9 || loaded.State.Actors[2].Status != "" || loaded.State.Actors[2].Temporary != corebattle.TemporaryCover || loaded.State.Actors[0].Temporary != "" || c.ActorID != "helper" || h.HP < 110 || h.HP > 146 {
		t.Fatal(loaded, c)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal("intrinsic callback wrote persistent character", chars.characters[char.ID])
	}
}
