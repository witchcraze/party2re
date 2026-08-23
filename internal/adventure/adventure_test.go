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

func (r *characterRepositoryStub) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" || r.value.ID != id {
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

func TestAdventureNewServiceNilDependencies(t *testing.T) {
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{}
	battle := corebattle.Engine{}
	clock := &testClock{}

	if _, err := NewService(nil, characters, battle); err == nil {
		t.Fatal("NewService(nil, characters, battle) expected error, got nil")
	}
	if _, err := NewService(adventures, nil, battle); err == nil {
		t.Fatal("NewService(adventures, nil, battle) expected error, got nil")
	}
	if _, err := NewService(adventures, characters, nil); err == nil {
		t.Fatal("NewService(adventures, characters, nil) expected error, got nil")
	}
	if _, err := NewServiceWithClock(adventures, characters, battle, nil); err == nil {
		t.Fatal("NewServiceWithClock(adventures, characters, battle, nil) expected error, got nil")
	}
	if _, err := NewServiceWithClock(adventures, characters, battle, clock); err != nil {
		t.Fatalf("NewServiceWithClock(adventures, characters, battle, clock) error = %v", err)
	}

	svc, err := NewService(adventures, characters, battle)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
}

func TestAdventureStartInvalidCharacter(t *testing.T) {
	service, _, _, _ := newTestService(t)

	if _, err := service.Start(context.Background(), ""); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("Start(\"\") error = %v, want %v", err, corecharacter.ErrNotFound)
	}
	if _, err := service.Start(context.Background(), "nonexistent_char"); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("Start(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
}

func TestAdventureClaimNotFound(t *testing.T) {
	service, _, _, _ := newTestService(t)

	if _, err := service.Claim(context.Background(), "nonexistent_adventure"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Claim(nonexistent) error = %v, want %v", err, ErrNotFound)
	}
}

func TestAdventureClaimDefeatOutcome(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	service.battle = battleResolverStub{result: corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: AdventureEnemyID,
		LoserID:  characters.value.ID,
		Turns:    1,
		Reward:   corebattle.Reward{},
	}}
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	initialExp := characters.value.Experience
	initialMoney := characters.value.Money

	got, err := service.Claim(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !got.Claimed || !got.Resolved || got.BattleResult.WinnerID != AdventureEnemyID {
		t.Fatalf("Claim() result = %#v, want WinnerID == AdventureEnemyID", got)
	}
	if characters.value.Experience != initialExp || characters.value.Money != initialMoney {
		t.Fatalf("character modified on loss: exp=%d, money=%d", characters.value.Experience, characters.value.Money)
	}
	if repository.value.BattleResult.WinnerID != AdventureEnemyID {
		t.Fatalf("saved battle result = %#v, want WinnerID == AdventureEnemyID", repository.value.BattleResult)
	}
}

func TestAdventureClaimDrawOutcome(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	service.battle = battleResolverStub{result: corebattle.Result{
		Outcome: corebattle.OutcomeDraw,
		Turns:   10,
		Reward:  corebattle.Reward{},
	}}
	value, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = value.AvailableAt

	initialExp := characters.value.Experience
	initialMoney := characters.value.Money

	got, err := service.Claim(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !got.Claimed || !got.Resolved || got.BattleResult.Outcome != corebattle.OutcomeDraw {
		t.Fatalf("Claim() result = %#v, want OutcomeDraw", got)
	}
	if characters.value.Experience != initialExp || characters.value.Money != initialMoney {
		t.Fatalf("character modified on draw: exp=%d, money=%d", characters.value.Experience, characters.value.Money)
	}
	if repository.value.BattleResult.Outcome != corebattle.OutcomeDraw {
		t.Fatalf("saved battle result = %#v, want OutcomeDraw", repository.value.BattleResult)
	}
}

func TestAdventureRealClock(t *testing.T) {
	clock := realClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Fatal("realClock.Now() returned zero time")
	}
}
