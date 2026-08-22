package adventure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

type repositoryStub struct {
	mu         sync.Mutex
	value      Adventure
	characters *characterRepositoryStub
}

func (r *repositoryStub) Save(_ context.Context, value Adventure) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = value
	return nil
}
func (r *repositoryStub) FindByID(_ context.Context, _ string) (Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" {
		return Adventure{}, ErrNotFound
	}
	return r.value, nil
}
func (r *repositoryStub) ClaimAndApply(_ context.Context, value Adventure, character corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID != value.ID {
		return ErrNotFound
	}
	if r.value.Claimed {
		return ErrAlreadyClaimed
	}
	r.value = value
	r.characters.mu.Lock()
	defer r.characters.mu.Unlock()
	r.characters.value = character
	return nil
}

type characterRepositoryStub struct {
	mu    sync.Mutex
	value corecharacter.Character
}

func (r *characterRepositoryStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return r.value, nil
}
func (r *characterRepositoryStub) Update(_ context.Context, value corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = value
	return nil
}

type battleResolverStub struct {
	result corebattle.Result
}

func (b battleResolverStub) Resolve(corebattle.Request) (corebattle.Result, error) {
	return b.result, nil
}

func newTestService(t *testing.T) (*Service, *testClock, *repositoryStub, *characterRepositoryStub) {
	t.Helper()
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{value: character}
	adventures.characters = characters
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

func TestAdventureAppliesSelectedBattleCurrencyReward(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	service.battle = battleResolverStub{result: corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: characters.value.ID,
		Reward:   corebattle.Reward{Currency: 15},
	}}
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	if _, err := service.Claim(context.Background(), value.ID); err != nil {
		t.Fatal(err)
	}
	if characters.value.Money != 215 {
		t.Fatalf("Money = %d, want 215", characters.value.Money)
	}
	if repository.value.BattleResult.Reward.Currency != 15 {
		t.Fatalf("saved reward = %#v, want currency 15", repository.value.BattleResult.Reward)
	}
}

func TestAdventureRejectsUnsupportedItemReward(t *testing.T) {
	service, clock, _, characters := newTestService(t)
	service.battle = battleResolverStub{result: corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: characters.value.ID,
		Reward:   corebattle.Reward{ItemDefinitionID: "potion", ItemQuantity: 1},
	}}
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	if _, err := service.Claim(context.Background(), value.ID); !errors.Is(err, ErrUnsupportedReward) {
		t.Fatalf("Claim() error = %v, want %v", err, ErrUnsupportedReward)
	}
}

func TestAdventureClaimsRewardAtMostOnceConcurrently(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, claimErr := service.Claim(context.Background(), value.ID)
			errs <- claimErr
		}()
	}
	wg.Wait()
	close(errs)

	var successes, alreadyClaimed int
	for claimErr := range errs {
		switch {
		case claimErr == nil:
			successes++
		case errors.Is(claimErr, ErrAlreadyClaimed):
			alreadyClaimed++
		default:
			t.Fatalf("Claim() error = %v", claimErr)
		}
	}
	if successes != 1 || alreadyClaimed != 1 {
		t.Fatalf("concurrent claims: successes = %d, already claimed = %d", successes, alreadyClaimed)
	}
	if characters.value.Experience != AdventureReward || !repository.value.Claimed {
		t.Fatalf("concurrent claim state = adventure %#v, character %#v", repository.value, characters.value)
	}
}
