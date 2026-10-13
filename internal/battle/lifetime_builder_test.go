package battle_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
)

type historyReadStub struct {
	state corejob.CharacterJob
	err   error
}

func (r *historyReadStub) FindByCharacterID(context.Context, string) (corejob.CharacterJob, error) {
	return r.state, r.err
}

func TestBuildLifetimeActorHistoryIsReadOnly(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-82", SP: 111, OldJobID: "job-36", OldSP: 100, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 10, Defense: 10, Agility: 10}}
	chars.characters[char.ID] = char
	history := &historyReadStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID, History: []corejob.Change{{FromJobID: "job-01", ToJobID: "job-02"}}, MasteredJobs: []string{"job-03"}, MasteredJobSP: map[string]int{"job-03": 80}, RecordedJobSP: map[string]int{"job-01": 17, "job-03": 20}}}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(history))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "native.svg")
	if err != nil || a.Jobs.JobID != "job-82" || a.Jobs.SP != 111 || a.Jobs.OldSP != 100 || a.OriginalJobs != a.Jobs || a.Icon != "native.svg" || a.NPC || a.Hit != 95 || a.Tension != 1 || a.JobHistory["job-02"] != nil || *a.JobHistory["job-01"] != 17 || *a.JobHistory["job-03"] != 20 {
		t.Fatal(a, err)
	}
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a}}
	c := corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: char.ID, PartyIDs: []string{char.ID}}}, RequesterGender: "m"}
	if _, err := s.ApplyTransformation(&c, "tool-168-memory", "job-01", random.Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTransformation(&c, "job-82-form", "", random.Default()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) || history.state.RecordedJobSP["job-01"] != 17 || history.state.CurrentJobID != "job-82" || len(history.state.History) != 1 {
		t.Fatal("temporary identity escaped to permanent state", history.state, chars.characters[char.ID])
	}
	// Snapshot pointers never alias a repository-owned map value.
	*s.Actors[0].JobHistory["job-01"] = 99
	if history.state.RecordedJobSP["job-01"] != 17 {
		t.Fatal(history.state)
	}
	history.err = errors.New("history read failure")
	if _, err := svc.BuildLifetimeActor(context.Background(), char.ID, ""); !errors.Is(err, history.err) {
		t.Fatal(err)
	}
	history.err = corejob.ErrCharacterJobNotFound
	chars.characters[char.ID] = char
	char.Stats.HP = 0
	chars.characters[char.ID] = char
	missing, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil || missing.JobHistory != nil || missing.HP != 0 {
		t.Fatal(missing, err)
	}
}
