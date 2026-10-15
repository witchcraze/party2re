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

func TestRosterUsesAdapterSnapshotWithoutChangingPermanentJobs(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", Level: 70, JobID: "job-59", SP: 200, OldJobID: "job-60", OldSP: 150, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 100, MaxMP: 100, Attack: 100, Defense: 80, Agility: 60}}
	chars.characters[char.ID] = char
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a}}
	c := corebattle.RosterContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: a.ID, PartyIDs: []string{a.ID}}, RequesterLevel: &char.Level}}, EnemyTeamID: "npc"}
	d := skill.BattleDefinitions{RosterSkills: map[string]bool{"job-59-summon": true, "job-59-merge": true}}
	for _, id := range []string{"job-59-summon", "job-59-merge"} {
		if err := d.RosterAction(&s, &c, id, "", random.NewDeterministic(1)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplyRoster(&c, id, "", random.NewDeterministic(1)); err != nil {
			t.Fatal(err)
		}
	}
	s.PruneRoster(&c.TargetContext)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Actors) != 1 || loaded.Actors[0].Jobs != a.Jobs || loaded.Actors[0].Gear == nil || !reflect.DeepEqual(chars.characters[char.ID], char) {
		t.Fatal(loaded, chars.characters[char.ID])
	}
}
