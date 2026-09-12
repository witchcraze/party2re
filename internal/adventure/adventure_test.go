package adventure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
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

func (r *repositoryStub) FindByID(_ context.Context, id string) (Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.ID == "" || r.value.ID != id {
		return Adventure{}, ErrNotFound
	}
	return r.value, nil
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

func newTestService(t *testing.T) (*Service, *testClock, *repositoryStub, *characterRepositoryStub) {
	t.Helper()
	character, err := corecharacter.New("Alice")
	if err != nil {
		t.Fatal(err)
	}
	character.Stats.HP = 200
	character.Stats.MaxHP = 200
	character.Stats.Attack = 50
	character.Stats.Defense = 20
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{value: character}
	adventures.characters = characters
	clock := &testClock{now: time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)}
	logger := nopLogger{}
	inventories := newInventoryRepositoryStub()
	stages, _ := InitialStageCatalog()
	monsters, _ := InitialMonsterCatalog()
	service, err := NewServiceWithCatalogs(adventures, characters, inventories, stages, monsters, corebattle.Engine{}, nil, logger, clock)
	if err != nil {
		t.Fatal(err)
	}
	return service, clock, adventures, characters
}

func TestStartStageSuccess(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	characters.value.Level = 10

	adv, err := service.StartStage(context.Background(), characters.value.ID, "stage-01")
	if err != nil {
		t.Fatalf("StartStage() error = %v", err)
	}
	if adv.StageID != "stage-01" {
		t.Errorf("StageID = %s, want stage-01", adv.StageID)
	}
	if !adv.Resolved {
		t.Errorf("Resolved = false, want true (immediate resolution)")
	}
	if adv.FloorsCleared == 0 {
		t.Errorf("FloorsCleared = 0, want > 0")
	}
	if adv.StartedAt != clock.now {
		t.Errorf("StartedAt = %v, want %v", adv.StartedAt, clock.now)
	}
	if repository.value.ID != adv.ID {
		t.Fatalf("saved adventure ID = %s, want %s", repository.value.ID, adv.ID)
	}
}

func TestStartDefaultStage(t *testing.T) {
	service, _, repository, characters := newTestService(t)

	adv, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if adv.StageID != StarterAdventure {
		t.Errorf("StageID = %s, want %s", adv.StageID, StarterAdventure)
	}
	if !adv.Resolved {
		t.Errorf("Resolved = false, want true")
	}
	if repository.value.ID != adv.ID {
		t.Fatalf("saved adventure ID = %s, want %s", repository.value.ID, adv.ID)
	}
}

func TestStartStageLevelRequirementNotMet(t *testing.T) {
	service, _, _, characters := newTestService(t)
	characters.value.Level = 1

	_, err := service.StartStage(context.Background(), characters.value.ID, "stage-05")
	if !errors.Is(err, ErrLevelRequirementNotMet) && !errors.Is(err, ErrJobLevelRequirementNotMet) {
		t.Fatalf("StartStage() error = %v, want level requirement error", err)
	}
}

func TestStartStageNotFound(t *testing.T) {
	service, _, _, characters := newTestService(t)

	_, err := service.StartStage(context.Background(), characters.value.ID, "nonexistent-stage")
	if !errors.Is(err, ErrStageNotFound) {
		t.Fatalf("StartStage() error = %v, want %v", err, ErrStageNotFound)
	}
}

func TestStartInvalidCharacter(t *testing.T) {
	service, _, _, _ := newTestService(t)

	if _, err := service.Start(context.Background(), ""); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("Start(\"\") error = %v, want %v", err, corecharacter.ErrNotFound)
	}
	if _, err := service.Start(context.Background(), "nonexistent_char"); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("Start(nonexistent) error = %v, want %v", err, corecharacter.ErrNotFound)
	}
}

func TestExecuteCrawl_ParticipantsValidation(t *testing.T) {
	service, _, _, characters := newTestService(t)

	// No participants
	_, err := service.ExecuteCrawl(context.Background(), DungeonCrawlRequest{})
	if !errors.Is(err, ErrNoParticipants) {
		t.Fatalf("ExecuteCrawl(empty) error = %v, want ErrNoParticipants", err)
	}

	// Too many participants (>4)
	_, err = service.ExecuteCrawl(context.Background(), DungeonCrawlRequest{
		CharacterIDs: []string{"c1", "c2", "c3", "c4", "c5"},
	})
	if !errors.Is(err, ErrTooManyParticipants) {
		t.Fatalf("ExecuteCrawl(5 chars) error = %v, want ErrTooManyParticipants", err)
	}

	// Unconscious character (HP <= 0)
	characters.value.Stats.HP = 0
	_, err = service.ExecuteCrawl(context.Background(), DungeonCrawlRequest{
		CharacterIDs: []string{characters.value.ID},
		StageID:      "stage-01",
	})
	if !errors.Is(err, ErrCharacterUnconscious) {
		t.Fatalf("ExecuteCrawl(unconscious) error = %v, want ErrCharacterUnconscious", err)
	}

	// Exhausted character (Tired >= 100)
	characters.value.Stats.HP = 50
	characters.value.Tired = 100
	_, err = service.ExecuteCrawl(context.Background(), DungeonCrawlRequest{
		CharacterIDs: []string{characters.value.ID},
		StageID:      "stage-01",
	})
	if !errors.Is(err, ErrCharacterExhausted) {
		t.Fatalf("ExecuteCrawl(exhausted) error = %v, want ErrCharacterExhausted", err)
	}
}

func TestAdventure_VictoryHook(t *testing.T) {
	service, _, _, characters := newTestService(t)
	// Give high level so character easily wins starter stage
	characters.value.Level = 50
	characters.value.Stats.HP = 500
	characters.value.Stats.MaxHP = 500
	characters.value.Stats.Attack = 100
	characters.value.Stats.Defense = 50

	var hookedCharID string
	var hookedMonsters int
	var hookedGold int
	service.SetVictoryHook(func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error {
		hookedCharID = characterID
		hookedMonsters = monstersDefeated
		hookedGold = goldEarned
		return nil
	})

	adv, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}

	if adv.BattleResult.Outcome == corebattle.OutcomeWin {
		if hookedCharID != characters.value.ID {
			t.Errorf("expected hookedCharID %s, got %s", characters.value.ID, hookedCharID)
		}
		if hookedMonsters == 0 {
			t.Errorf("expected hookedMonsters > 0, got %d", hookedMonsters)
		}
		if hookedGold != adv.BattleResult.Reward.Currency {
			t.Errorf("expected hookedGold %d, got %d", adv.BattleResult.Reward.Currency, hookedGold)
		}
	}
}

func TestAdventure_PostAdventureHook(t *testing.T) {
	service, _, _, characters := newTestService(t)

	var postAdvCharID string
	service.SetPostAdventureHook(func(ctx context.Context, characterID string) error {
		postAdvCharID = characterID
		return nil
	})

	_, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}

	if postAdvCharID != characters.value.ID {
		t.Errorf("expected postAdvCharID %s, got %s", characters.value.ID, postAdvCharID)
	}
}

func TestAdventureNewServiceNilDependencies(t *testing.T) {
	adventures := &repositoryStub{}
	characters := &characterRepositoryStub{}
	battle := corebattle.Engine{}
	clock := &testClock{}
	logger := nopLogger{}

	if _, err := NewService(nil, characters, battle, nil, logger); err == nil {
		t.Fatal("NewService(nil, ...) expected error, got nil")
	}
	if _, err := NewService(adventures, nil, battle, nil, logger); err == nil {
		t.Fatal("NewService(..., nil, ...) expected error, got nil")
	}
	if _, err := NewService(adventures, characters, nil, nil, logger); err == nil {
		t.Fatal("NewService(..., nil, ...) expected error, got nil")
	}
	if _, err := NewServiceWithClock(adventures, characters, battle, nil, logger, nil); err == nil {
		t.Fatal("NewServiceWithClock(..., nil) expected error, got nil")
	}
	if _, err := NewServiceWithClock(adventures, characters, battle, nil, logger, clock); err != nil {
		t.Fatalf("NewServiceWithClock(...) error = %v", err)
	}

	svc, err := NewService(adventures, characters, battle, nil, logger)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
}

func TestAdventureRealClock(t *testing.T) {
	clock := RealClock{}
	now := clock.Now()
	if now.IsZero() {
		t.Fatal("RealClock.Now() returned zero time")
	}
}

func TestValidateCombatItem(t *testing.T) {
	tests := []struct {
		name    string
		def     item.Definition
		wantErr bool
	}{
		{
			name: "combat only item allowed",
			def: item.Definition{
				ID:            "item-001",
				Name:          "薬草",
				UsageCategory: item.UsageCategoryCombatOnly,
			},
			wantErr: false,
		},
		{
			name: "anytime seed item rejected",
			def: item.Definition{
				ID:            "item-060",
				Name:          "力の種",
				UsageCategory: item.UsageCategoryAnytime,
			},
			wantErr: true,
		},
		{
			name: "small medal rejected",
			def: item.Definition{
				ID:            "item-100",
				Name:          "小さなメダル",
				UsageCategory: item.UsageCategoryAnytime,
			},
			wantErr: true,
		},
		{
			name: "passive item rejected",
			def: item.Definition{
				ID:            "item-037",
				Name:          "闘気の盾",
				UsageCategory: item.UsageCategoryCombatPassive,
			},
			wantErr: true,
		},
		{
			name: "material item rejected",
			def: item.Definition{
				ID:            "item-200",
				Name:          "鉄のインゴット",
				UsageCategory: item.UsageCategoryNone,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCombatItem(tt.def)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for item %s, got nil", tt.def.Name)
				} else if !errors.Is(err, ErrCannotUseInCombat) {
					t.Errorf("expected ErrCannotUseInCombat, got %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error for item %s, got %v", tt.def.Name, err)
				}
			}
		})
	}
}
