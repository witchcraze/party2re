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

func (r *repositoryStub) FindByPlayerID(_ context.Context, playerID string) ([]corecharacter.Character, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.saved.PlayerID == playerID {
		return []corecharacter.Character{r.saved}, nil
	}
	return nil, nil
}

func TestServiceCreateSavesCharacter(t *testing.T) {
	repository := &repositoryStub{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Create(context.Background(), "player-1", "Alice")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID != repository.saved.ID || got.PlayerID != "player-1" || repository.saved.PlayerID != "player-1" {
		t.Fatalf("Create() did not save returned character: got %#v saved %#v", got, repository.saved)
	}
}

func TestServiceCreateWithOptionsSavesInitialIdentity(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)

	got, err := service.CreateWithOptions(context.Background(), "player-1", "Alice", CreationOptions{
		JobID:  "starter-2",
		Gender: "female",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions() error = %v", err)
	}
	if got.JobID != "starter-2" || got.Gender != "female" || got.PlayerID != "player-1" || repository.saved.Stats != got.Stats {
		t.Fatalf("CreateWithOptions() = %#v, saved %#v", got, repository.saved)
	}
}

func TestServiceCreateRejectsInvalidInputWithoutSaving(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)

	if _, err := service.Create(context.Background(), "player-1", ""); err == nil {
		t.Fatal("Create() error = nil, want validation error for empty name")
	}
	if _, err := service.Create(context.Background(), "", "Alice"); !errors.Is(err, ErrInvalidPlayer) {
		t.Fatalf("Create() error = %v, want ErrInvalidPlayer for empty playerID", err)
	}
	if repository.saved.ID != "" {
		t.Fatal("Create() saved invalid character")
	}
}

func TestServiceCreateReturnsRepositoryError(t *testing.T) {
	want := errors.New("database unavailable")
	service, _ := NewService(&repositoryStub{err: want})

	if _, err := service.Create(context.Background(), "player-1", "Alice"); !errors.Is(err, want) {
		t.Fatalf("Create() error = %v, want %v", err, want)
	}
}

func (r *repositoryStub) Update(_ context.Context, value corecharacter.Character) error {
	r.saved = value
	return r.err
}

func TestServiceGetReturnsSavedCharacter(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)
	created, err := service.Create(context.Background(), "player-1", "Alice")
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

func TestServiceListByPlayer(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)
	created, err := service.Create(context.Background(), "player-1", "Alice")
	if err != nil {
		t.Fatal(err)
	}

	list, err := service.ListByPlayer(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("ListByPlayer error: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("ListByPlayer = %#v, want 1 character", list)
	}

	if _, err := service.ListByPlayer(context.Background(), ""); !errors.Is(err, ErrInvalidPlayer) {
		t.Fatalf("ListByPlayer(\"\") error = %v, want ErrInvalidPlayer", err)
	}
}

func TestServiceRebirth(t *testing.T) {
	repository := &repositoryStub{}
	service, _ := NewService(repository)
	created, err := service.Create(context.Background(), "player-1", "Rebirth Candidate")
	if err != nil {
		t.Fatal(err)
	}

	// Under-leveled character cannot rebirth
	if _, err := service.Rebirth(context.Background(), created.ID); err == nil {
		t.Fatal("expected error for level 1 character rebirth, got nil")
	}

	// Set character to level 99
	repository.saved.Level = 99
	rebirthed, err := service.Rebirth(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Rebirth error: %v", err)
	}
	if rebirthed.Level != 1 || rebirthed.RebirthCount != 1 {
		t.Errorf("rebirthed = %#v, want Level 1 and RebirthCount 1", rebirthed)
	}
}
