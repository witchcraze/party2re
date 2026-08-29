package job

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

type repositoryStub struct{ value corejob.CharacterJob }

func (r *repositoryStub) Save(_ context.Context, value corejob.CharacterJob) error {
	r.value = value
	return nil
}
func (r *repositoryStub) FindByCharacterID(_ context.Context, _ string) (corejob.CharacterJob, error) {
	return r.value, nil
}

func TestServiceChangePersistsJobHistory(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "starter")
	target, _ := corejob.NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	repository := &repositoryStub{value: state}
	service, _ := NewService(repository)
	got, err := service.Change(context.Background(), "character-1", target, 1, "unspecified")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentJobID != "vanguard" || len(repository.value.History) != 1 {
		t.Fatalf("got = %#v, saved = %#v", got, repository.value)
	}
}

func TestServiceMaster(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "starter")
	repository := &repositoryStub{value: state}
	service, _ := NewService(repository)

	got, err := service.Master(context.Background(), "character-1", "warrior")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMastered("warrior") || !repository.value.IsMastered("warrior") {
		t.Errorf("expected warrior to be mastered: %#v", got)
	}
}

func TestServiceCheckAndApplyMastery(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "mage")
	repository := &repositoryStub{value: state}
	service, _ := NewService(repository)

	// Level 50 does not trigger mastery
	applied, err := service.CheckAndApplyMastery(context.Background(), "character-1", 50)
	if err != nil || applied {
		t.Errorf("level 50 should not apply mastery: applied=%v, err=%v", applied, err)
	}

	// Level 99 triggers mastery
	applied, err = service.CheckAndApplyMastery(context.Background(), "character-1", 99)
	if err != nil || !applied {
		t.Errorf("level 99 should apply mastery: applied=%v, err=%v", applied, err)
	}
	if !repository.value.IsMastered("mage") {
		t.Errorf("expected mage to be mastered in repository")
	}

	// Re-checking does not re-apply
	applied, err = service.CheckAndApplyMastery(context.Background(), "character-1", 99)
	if err != nil || applied {
		t.Errorf("already mastered should not re-apply: applied=%v", applied)
	}
}

type charRepoStub struct {
	char corecharacter.Character
}

func (c *charRepoStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	return c.char, nil
}

func (c *charRepoStub) Update(_ context.Context, char corecharacter.Character) error {
	c.char = char
	return nil
}

func TestServiceListAndChangeJob(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "starter")
	repo := &repositoryStub{value: state}
	char := corecharacter.Character{
		ID:     "character-1",
		JobID:  "starter",
		Level:  10,
		Gender: "male",
	}
	charRepo := &charRepoStub{char: char}
	svc, err := NewService(repo, WithCharacterRepository(charRepo))
	if err != nil {
		t.Fatal(err)
	}

	defs := svc.ListDefinitions()
	if len(defs) == 0 {
		t.Fatal("expected non-empty job definitions list")
	}

	updatedChar, updatedJob, err := svc.ChangeJob(context.Background(), "character-1", "job-01")
	if err != nil {
		t.Fatal(err)
	}
	if updatedChar.JobID != "job-01" || updatedJob.CurrentJobID != "job-01" {
		t.Fatalf("expected job job-01, got char=%s, job=%s", updatedChar.JobID, updatedJob.CurrentJobID)
	}
}
