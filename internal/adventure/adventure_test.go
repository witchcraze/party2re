package adventure

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type repositoryStub struct{ value Adventure }

func (r *repositoryStub) Save(_ context.Context, value Adventure) error { r.value = value; return nil }
func (r *repositoryStub) FindByID(_ context.Context, _ string) (Adventure, error) {
	if r.value.ID == "" {
		return Adventure{}, ErrNotFound
	}
	return r.value, nil
}

type characterRepositoryStub struct{ value corecharacter.Character }

func (r *characterRepositoryStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	if r.value.ID == "" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return r.value, nil
}
func (r *characterRepositoryStub) Update(_ context.Context, value corecharacter.Character) error {
	r.value = value
	return nil
}

func newTestService(t *testing.T) (*Service, *testClock, *repositoryStub, *characterRepositoryStub) {
	t.Helper()
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{value: character}
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	service, err := NewServiceWithClock(adventures, characters, corebattle.Engine{}, clock)
	if err != nil {
		t.Fatal(err)
	}
	return service, clock, adventures, characters
}

func TestAdventureRequiresCompletionTime(t *testing.T) {
	service, _, _, characters := newTestService(t)
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Claim(context.Background(), value.ID); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Claim() error = %v, want %v", err, ErrNotReady)
	}
}

func TestAdventureClaimsBattleResultAndAwardsRewardOnce(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	got, err := service.Claim(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Claimed || !got.Resolved || got.BattleResult.WinnerID != characters.value.ID ||
		characters.value.Experience != AdventureReward {
		t.Fatalf("Claim() = %#v, character = %#v", got, characters.value)
	}
	if repository.value.BattleResult != got.BattleResult {
		t.Fatalf("saved result = %#v, want %#v", repository.value.BattleResult, got.BattleResult)
	}
	if _, err := service.Claim(context.Background(), value.ID); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("second Claim() error = %v, want %v", err, ErrAlreadyClaimed)
	}
}
