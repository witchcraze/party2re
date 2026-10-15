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

func TestArithmeticUsesAdapterSnapshotWithoutPersistentWrites(t *testing.T) {
	chars := newMockCharRepo()
	chars.characters["self"] = corecharacter.Character{ID: "self", JobID: "job-31", SP: 155, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 24, MaxMP: 100, Defense: 999}}
	chars.characters["other"] = corecharacter.Character{ID: "other", JobID: "job-72", SP: 180, Stats: corecharacter.Stats{HP: 100, MaxHP: 1000, MP: 24, MaxMP: 100, Defense: 1000}}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	s := corebattle.LifetimeState{}
	for _, id := range []string{"self", "other"} {
		a, err := svc.BuildLifetimeActor(context.Background(), id, "")
		if err != nil {
			t.Fatal(err)
		}
		a.TeamID = id
		s.Actors = append(s.Actors, a)
	}
	// Current defense deliberately crosses the opposite boundary: arithmetic
	// checks the builder's saved base defense, without recapturing current gear.
	s.Actors[0].Defense, s.Actors[1].Defense = 1000, 999
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded corebattle.LifetimeState
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	s = reloaded
	c := corebattle.ArithmeticContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self"}, EnemyIDs: []string{"other"}}, Divisor: 4, Defeat: func(string) error { return nil }}
	d := skill.BattleDefinitions{ArithmeticSkills: map[string]bool{"job-72-damage": true}}
	if _, err := d.ArithmeticAction(&s, &c, "job-72-damage", "", random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	before := map[string]corecharacter.Character{"self": chars.characters["self"], "other": chars.characters["other"]}
	if _, err := s.ApplyArithmetic(&c, "job-72-damage", "", random.NewDeterministic(1)); err != nil || s.Actors[0].HP != 0 || s.Actors[1].HP < 98 || s.Actors[1].HP > 99 {
		t.Fatal(s, err)
	}
	if !reflect.DeepEqual(before, chars.characters) {
		t.Fatal("pure battle effects wrote permanent characters")
	}
}
