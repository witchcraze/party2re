package adventure

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

func (r *repositoryStub) ListByCharacterID(_ context.Context, characterID string, limit, offset int) ([]Adventure, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.CharacterID == characterID {
		return []Adventure{r.value}, 1, nil
	}
	return nil, 0, nil
}

func (r *repositoryStub) ListByCharacterIDByCursor(_ context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.CharacterID == characterID {
		return []Adventure{r.value}, nil
	}
	return nil, nil
}

func (r *repositoryStub) GetAggregatedStats(_ context.Context, characterID string) (AggregatedStats, error) {
	return AggregatedStats{}, nil
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

type testScheduler struct {
	mu           sync.Mutex
	scheduledIDs []string
	err          error
}

func (s *testScheduler) Schedule(ctx context.Context, actionType, actorID string, params map[string]string, executeAt time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return "", s.err
	}
	s.scheduledIDs = append(s.scheduledIDs, "mock-id")
	return "mock-id", nil
}

type testLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *testLogger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
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
	scheduler := &testScheduler{}
	logger := nopLogger{}
	inventories := newInventoryRepositoryStub()
	stages, _ := InitialStageCatalog()
	monsters, _ := InitialMonsterCatalog()
	service, err := NewServiceWithCatalogs(adventures, characters, inventories, stages, monsters, corebattle.Engine{}, scheduler, logger, clock)
	if err != nil {
		t.Fatal(err)
	}
	return service, clock, adventures, characters
}

func TestStartSchedulesAdventure(t *testing.T) {
	service, clock, repository, characters := newTestService(t)

	got, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got.Type != StarterAdventure || got.StartedAt != clock.now || got.AvailableAt != clock.now.Add(AdventureDuration) {
		t.Fatalf("Start() = %#v", got)
	}
	if repository.value.ID != got.ID {
		t.Fatal("adventure was not saved")
	}
	scheduler := service.scheduler.(*testScheduler)
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if len(scheduler.scheduledIDs) != 1 {
		t.Fatal("scheduler was not called")
	}
}

func TestStartSchedulerFailsLogsWarningAndSucceeds(t *testing.T) {
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{value: character}
	adventures.characters = characters
	scheduler := &testScheduler{err: errors.New("valkey connection error")}
	logger := &testLogger{}
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	service, err := NewServiceWithClock(adventures, characters, corebattle.Engine{}, scheduler, logger, clock)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("Start() expected success even when scheduler fails, got error: %v", err)
	}
	if got.ID == "" {
		t.Fatal("adventure ID is empty")
	}

	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.warns) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(logger.warns))
	}
}

func TestStartNilSchedulerSucceeds(t *testing.T) {
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{value: character}
	adventures.characters = characters
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	service, err := NewServiceWithClock(adventures, characters, corebattle.Engine{}, nil, nil, clock)
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("Start() expected success with nil scheduler, got error: %v", err)
	}
	if got.ID == "" {
		t.Fatal("adventure ID is empty")
	}
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
		characters.value.Experience != value.ExperienceReward {
		t.Fatalf("Claim() = %#v, character = %#v", got, characters.value)
	}
	if !reflect.DeepEqual(repository.value.BattleResult, got.BattleResult) {
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
	service.inventories = nil
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
	if characters.value.Experience != value.ExperienceReward || !repository.value.Claimed {
		t.Fatalf("concurrent claim state = adventure %#v, character %#v", repository.value, characters.value)
	}
}

func TestAdventureNewServiceNilDependencies(t *testing.T) {
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{}
	battle := corebattle.Engine{}
	clock := &testClock{}
	scheduler := &testScheduler{}
	logger := nopLogger{}

	if _, err := NewService(nil, characters, battle, scheduler, logger); err == nil {
		t.Fatal("NewService(nil, ...) expected error, got nil")
	}
	if _, err := NewService(adventures, nil, battle, scheduler, logger); err == nil {
		t.Fatal("NewService(..., nil, ...) expected error, got nil")
	}
	if _, err := NewService(adventures, characters, nil, scheduler, logger); err == nil {
		t.Fatal("NewService(..., nil, ...) expected error, got nil")
	}
	if _, err := NewServiceWithClock(adventures, characters, battle, scheduler, logger, nil); err == nil {
		t.Fatal("NewServiceWithClock(..., nil) expected error, got nil")
	}
	if _, err := NewServiceWithClock(adventures, characters, battle, scheduler, logger, clock); err != nil {
		t.Fatalf("NewServiceWithClock(...) error = %v", err)
	}

	svc, err := NewService(adventures, characters, battle, scheduler, logger)
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
	clock := RealClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Fatal("RealClock.Now() returned zero time")
	}
}

type inventoryRepositoryStub struct {
	mu          sync.Mutex
	inventories map[string]coreinventory.Inventory
}

func newInventoryRepositoryStub() *inventoryRepositoryStub {
	return &inventoryRepositoryStub{inventories: make(map[string]coreinventory.Inventory)}
}

func (r *inventoryRepositoryStub) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.inventories[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (r *inventoryRepositoryStub) Save(_ context.Context, value coreinventory.Inventory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventories[value.CharacterID] = value
	return nil
}

func TestAdventureStartStageSuccess(t *testing.T) {
	service, clock, _, characters := newTestService(t)
	// Elevate character to level 10 to access stage-02
	characters.value.Level = 10

	adv, err := service.StartStage(context.Background(), characters.value.ID, "stage-02")
	if err != nil {
		t.Fatalf("StartStage() error = %v", err)
	}
	if adv.StageID != "stage-02" || adv.MonsterID == "" {
		t.Fatalf("StartStage() = %#v", adv)
	}
	if adv.AvailableAt != clock.now.Add(time.Hour) {
		t.Errorf("AvailableAt = %v, want %v", adv.AvailableAt, clock.now.Add(time.Hour))
	}
}

func TestAdventureStartStageLevelRequirementNotMet(t *testing.T) {
	service, _, _, characters := newTestService(t)
	characters.value.Level = 1 // MinLevel for stage-05 is 35

	_, err := service.StartStage(context.Background(), characters.value.ID, "stage-05")
	if !errors.Is(err, ErrLevelRequirementNotMet) {
		t.Fatalf("StartStage() error = %v, want %v", err, ErrLevelRequirementNotMet)
	}
}

func TestAdventureStartStageNotFound(t *testing.T) {
	service, _, _, characters := newTestService(t)

	_, err := service.StartStage(context.Background(), characters.value.ID, "nonexistent-stage")
	if !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("StartStage() error = %v, want %v", err, ErrStageNotFound)
	}
}

func TestAdventureClaimAwardsGoldAndItemDrops(t *testing.T) {
	character, _ := corecharacter.New("Loot Collector")
	character.Level = 10
	character.Money = 50

	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{value: character}
	adventures.characters = characters
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	scheduler := &testScheduler{}
	logger := nopLogger{}
	inventories := newInventoryRepositoryStub()

	stages, _ := InitialStageCatalog()
	monsters, _ := InitialMonsterCatalog()

	service, err := NewServiceWithCatalogs(adventures, characters, inventories, stages, monsters, corebattle.Engine{}, scheduler, logger, clock)
	if err != nil {
		t.Fatal(err)
	}

	adv, err := service.StartStage(context.Background(), character.ID, "stage-01")
	if err != nil {
		t.Fatal(err)
	}

	clock.now = adv.AvailableAt

	claimed, err := service.Claim(context.Background(), adv.ID)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}

	if !claimed.Claimed || !claimed.Resolved {
		t.Fatalf("Claim() not resolved or claimed: %#v", claimed)
	}

	// Character money should have increased by monster gold reward
	if characters.value.Money <= 50 {
		t.Errorf("character money was not awarded: %d", characters.value.Money)
	}

	// Inventory should contain dropped item instance if dropped
	if claimed.BattleResult.Reward.ItemDefinitionID != "" {
		inv, _ := inventories.FindByCharacterID(context.Background(), character.ID)
		if len(inv.Items) != 1 || inv.Items[0].DefinitionID != claimed.BattleResult.Reward.ItemDefinitionID {
			t.Errorf("inventory item was not granted: %#v", inv)
		}
	}
}
