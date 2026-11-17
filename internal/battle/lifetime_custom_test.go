package battle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/customskill"
)

type customReadError struct{ err error }

func (r customReadError) FindCustomSkill(context.Context, string) (*customskill.CustomSkill, error) {
	return nil, r.err
}

// system.cgi:73–78 initializes CMP from capped level and both job tiers.
func TestLifetimeCustomFreezesSlotsCostAndInitialCMP(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", Level: 150, JobID: "job-01", OldJobID: "job-02", Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
	chars.characters[char.ID] = char
	custom := &customskill.CustomSkill{Name: "custom", CMP: 18, Gems: [3]string{"gem_metronome", "gem_metronome", "gem_metronome"}}
	repo := mockCustomSkillRepo{char.ID: custom}
	service := battle.NewService(
		battle.WithCharacterRepository(chars),
		battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}),
		battle.WithJobDefinitionProvider(mockJobProvider{"job-01": {CMPTier: 4}, "job-02": {CMPTier: 2}}),
		battle.WithCustomSkillRepository(repo),
	)
	actor, err := service.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if actor.CMP != 227 || actor.MaxCMP != 227 || actor.Custom == nil || actor.Custom.Name != custom.Name || actor.Custom.Gems != custom.Gems || actor.Custom.Cost != 18 {
		t.Fatal(actor)
	}
	copy := actor.Clone()
	copy.Custom.Gems[0] = "changed"
	custom.Gems[1] = "changed"
	if actor.Custom.Gems != [3]string{"gem_metronome", "gem_metronome", "gem_metronome"} {
		t.Fatal("custom snapshot aliases mutable data", actor.Custom)
	}
	delete(repo, char.ID)
	actor, err = service.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil || actor.Custom != nil || actor.CMP != 227 {
		t.Fatal(actor, err)
	}
	readErr := errors.New("custom read failed")
	battle.WithCustomSkillRepository(customReadError{readErr})(service)
	if _, err := service.BuildLifetimeActor(context.Background(), char.ID, ""); !errors.Is(err, readErr) {
		t.Fatal(err)
	}
}
