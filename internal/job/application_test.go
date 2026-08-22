package job

import (
	"context"
	"testing"

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
