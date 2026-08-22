package character

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type repositoryStub struct {
	saved corecharacter.Character
	err   error
}

func (r *repositoryStub) Save(_ context.Context, value corecharacter.Character) error {
	r.saved = value
	return r.err
}

func (r *repositoryStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	return r.saved, r.err
}

func TestServiceCreateSavesCharacter(t *testing.T) {
	repository := &repositoryStub{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Create(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != repository.saved.ID {
		t.Fatalf("Create() did not save returned character: got %#v saved %#v", got, repository.saved)
	}
}

func TestServiceCreateRejectsInvalidInputWithoutSaving(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)

	if _, err := service.Create(context.Background(), ""); err == nil {
		t.Fatal("Create() error = nil, want validation error")
	}
	if repository.saved.ID != "" {
		t.Fatal("Create() saved invalid character")
	}
}

func TestServiceCreateReturnsRepositoryError(t *testing.T) {
	want := errors.New("database unavailable")
	service, _ := NewService(&repositoryStub{err: want})

	if _, err := service.Create(context.Background(), "Alice"); !errors.Is(err, want) {
		t.Fatalf("Create() error = %v, want %v", err, want)
	}
}

func TestServiceGetReturnsSavedCharacter(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)
	created, err := service.Create(context.Background(), "Alice")
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != created {
		t.Fatalf("Get() = %#v, want %#v", got, created)
	}
}
