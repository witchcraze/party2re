package dungeon_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreprogression "github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type mockDungeonRepo struct {
	records      map[string]dungeon.CharacterDungeonRecord
	active       map[string]*dungeon.ActiveExpedition
	histories    map[string][]dungeon.DungeonExpeditionHistory
	savedChars   map[string]corecharacter.Character
	awardedItems map[string][]coreitem.Instance
}

func newMockDungeonRepo() *mockDungeonRepo {
	return &mockDungeonRepo{
		records:      make(map[string]dungeon.CharacterDungeonRecord),
		active:       make(map[string]*dungeon.ActiveExpedition),
		histories:    make(map[string][]dungeon.DungeonExpeditionHistory),
		savedChars:   make(map[string]corecharacter.Character),
		awardedItems: make(map[string][]coreitem.Instance),
	}
}

func (m *mockDungeonRepo) GetRecord(ctx context.Context, characterID string) (dungeon.CharacterDungeonRecord, error) {
	rec, ok := m.records[characterID]
	if !ok {
		now := time.Now().UTC()
		rec = dungeon.CharacterDungeonRecord{
			CharacterID: characterID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		m.records[characterID] = rec
	}
	return rec, nil
}

func (m *mockDungeonRepo) GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
	return m.active[characterID], nil
}

func (m *mockDungeonRepo) SaveActiveExpedition(ctx context.Context, exp dungeon.ActiveExpedition) error {
	m.active[exp.CharacterID] = &exp
	return nil
}

func (m *mockDungeonRepo) DeleteActiveExpedition(ctx context.Context, characterID, expeditionID string) error {
	if run := m.active[characterID]; run != nil && run.ID == expeditionID {
		delete(m.active, characterID)
	}
	return nil
}

func (m *mockDungeonRepo) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	record dungeon.CharacterDungeonRecord,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
	members ...dungeon.ExpeditionMember,
) error {
	for _, h := range m.histories[record.CharacterID] {
		if history.ID != "" && h.ID == history.ID {
			return dungeon.ErrExpeditionAlreadyFinished
		}
	}
	m.records[record.CharacterID] = record
	m.histories[record.CharacterID] = append([]dungeon.DungeonExpeditionHistory{history}, m.histories[record.CharacterID]...)
	if character != nil {
		m.savedChars[character.ID] = *character
	}
	for _, mem := range members {
		if character != nil && mem.CharacterID == character.ID {
			continue
		}
		c, ok := m.savedChars[mem.CharacterID]
		if !ok {
			c = corecharacter.Character{ID: mem.CharacterID}
		}
		c.Stats.HP = mem.CurrentHP
		c.Stats.MP = mem.CurrentMP
		if history.Outcome == dungeon.StatusCleared || history.Outcome == dungeon.StatusEscaped {
			earnedExp := mem.AccumulatedExp
			earnedGold := mem.AccumulatedGold
			if earnedExp == 0 && earnedGold == 0 && len(members) <= 1 {
				earnedExp = history.ExpReward
				earnedGold = history.GoldReward
			}
			if earnedExp > 0 {
				_, _ = coreprogression.ApplyExperience(&c, earnedExp)
			}
			if earnedGold > 0 {
				_ = c.AddMoney(earnedGold)
			}
		}
		m.savedChars[mem.CharacterID] = c
	}
	if len(rewardItems) > 0 {
		m.awardedItems[record.CharacterID] = append(m.awardedItems[record.CharacterID], rewardItems...)
	}
	return nil
}

func (m *mockDungeonRepo) GetHistory(ctx context.Context, characterID string, limit int) ([]dungeon.DungeonExpeditionHistory, error) {
	list := m.histories[characterID]
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, dungeon.ErrCharacterNotFound
	}
	return c, nil
}

func createTestChar(id string, level, hp, attack, defense int) corecharacter.Character {
	return corecharacter.Character{
		ID:    id,
		Name:  "Hero_" + id,
		Level: level,
		Stats: corecharacter.Stats{
			HP:      hp,
			MaxHP:   hp,
			Attack:  attack,
			Defense: defense,
			Agility: 40,
		},
		Money: 500,
	}
}

func TestListDungeons_LevelAndPrereqGate(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"novice":  createTestChar("novice", 3, 50, 10, 5),
			"adept":   createTestChar("adept", 25, 200, 50, 40),
			"veteran": createTestChar("veteran", 65, 800, 200, 150),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Novice (Lv 3) - Dungeon 1 requires Lv 5 -> locked
	dungeons, err := service.ListDungeons(ctx, "novice")
	if err != nil {
		t.Fatal(err)
	}
	if dungeons[0].IsUnlocked {
		t.Errorf("expected dungeon 1 to be locked for level 3 novice")
	}

	// 2. Adept (Lv 25) - Dungeon 1 unlocked, Dungeon 2 locked (has not cleared Dungeon 1)
	dungeons, err = service.ListDungeons(ctx, "adept")
	if err != nil {
		t.Fatal(err)
	}
	if !dungeons[0].IsUnlocked {
		t.Errorf("expected dungeon 1 to be unlocked for level 25 adept")
	}
	if dungeons[1].IsUnlocked {
		t.Errorf("expected dungeon 2 to be locked by prerequisite tier")
	}
}

func TestStartExpedition_SuccessAndGuards(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"novice": createTestChar("novice", 2, 40, 10, 5),
			"advent": createTestChar("advent", 10, 150, 30, 20),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Level Requirement Not Met
	_, err = service.StartExpedition(ctx, "novice", "dungeon-01")
	if !errors.Is(err, dungeon.ErrLevelRequirementNotMet) {
		t.Errorf("expected ErrLevelRequirementNotMet, got %v", err)
	}

	// 2. Start Success
	exp, err := service.StartExpedition(ctx, "advent", "dungeon-01")
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}
	if exp.CurrentFloor != 1 || exp.PosX != 0 || exp.PosY != 0 || exp.TurnsRemaining != 25 {
		t.Errorf("unexpected initial expedition state: %#v", exp)
	}

	// 3. Reject Starting when already in progress
	_, err = service.StartExpedition(ctx, "advent", "dungeon-01")
	if !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
		t.Errorf("expected ErrActiveExpeditionExists, got %v", err)
	}
}

func TestMove_WallAndOutOfBounds(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"advent": createTestChar("advent", 10, 150, 30, 20),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.StartExpedition(ctx, "advent", "dungeon-01")
	if err != nil {
		t.Fatal(err)
	}

	// North from (0, 0) is out of bounds
	_, err = service.Move(ctx, "advent", dungeon.DirectionNorth)
	if !errors.Is(err, dungeon.ErrImpassableWall) {
		t.Errorf("expected ErrImpassableWall out of bounds, got %v", err)
	}

	// South from (0, 0) is wall '1' in dungeon-01 floor 1 ("1101")
	_, err = service.Move(ctx, "advent", dungeon.DirectionSouth)
	if !errors.Is(err, dungeon.ErrImpassableWall) {
		t.Errorf("expected ErrImpassableWall for wall tile, got %v", err)
	}
}

func TestMove_TreasureAndTraps(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"advent": createTestChar("advent", 10, 200, 40, 30),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.StartExpedition(ctx, "advent", "dungeon-01")
	if err != nil {
		t.Fatal(err)
	}

	// Move East to (1, 0) - path '0'
	res, err := service.Move(ctx, "advent", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move east failed: %v", err)
	}

	// Move East to (2, 0) - path '0'
	res, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move east failed: %v", err)
	}

	// Move East to (3, 0) - treasure 'T'
	res, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move east to treasure failed: %v", err)
	}
	if res.EventType != dungeon.EventTreasure || res.GoldFound <= 0 {
		t.Errorf("expected treasure event, got %#v", res)
	}
	if res.MedalsFound != 1 || res.Expedition.AccumulatedMedals != 1 {
		t.Errorf("expected 1 medal found in chest, got found=%d, accumulated=%d", res.MedalsFound, res.Expedition.AccumulatedMedals)
	}
	if res.Expedition.AccumulatedGold < res.GoldFound {
		t.Errorf("expected accumulated gold >= %d, got %d", res.GoldFound, res.Expedition.AccumulatedGold)
	}
}

func TestEscape_LocksInLedgerRewards(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"advent": createTestChar("advent", 10, 200, 40, 30),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.StartExpedition(ctx, "advent", "dungeon-01")
	if err != nil {
		t.Fatal(err)
	}

	// Move to treasure (0,0) -> (1,0) -> (2,0) -> (3,0)
	_, _ = service.Move(ctx, "advent", dungeon.DirectionEast)
	_, _ = service.Move(ctx, "advent", dungeon.DirectionEast)
	res, err := service.Move(ctx, "advent", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}

	accumulatedGold := res.Expedition.AccumulatedGold
	accumulatedExp := res.Expedition.AccumulatedExp
	accumulatedMedals := res.Expedition.AccumulatedMedals

	// Escape from dungeon
	escRes, err := service.Escape(ctx, "advent")
	if err != nil {
		t.Fatalf("Escape failed: %v", err)
	}
	if escRes.EventType != dungeon.EventEscape || !escRes.IsFinished {
		t.Errorf("expected finished escape event, got %#v", escRes)
	}
	if escRes.MedalsFound != accumulatedMedals {
		t.Errorf("expected %d medals on escape, got %d", accumulatedMedals, escRes.MedalsFound)
	}

	// Verify active expedition is cleaned up
	active, _ := service.GetActiveExpedition(ctx, "advent")
	if active != nil {
		t.Errorf("expected active expedition to be removed")
	}

	// Verify rewards committed to character
	savedChar := repo.savedChars["advent"]
	if savedChar.Money != 500+accumulatedGold {
		t.Errorf("expected character money %d, got %d", 500+accumulatedGold, savedChar.Money)
	}
	if savedChar.SmallMedals != accumulatedMedals {
		t.Errorf("expected character medals %d, got %d", accumulatedMedals, savedChar.SmallMedals)
	}

	// Verify history
	history, err := service.GetHistory(ctx, "advent", 5)
	if err != nil || len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d (err: %v)", len(history), err)
	}
	if history[0].Outcome != dungeon.StatusEscaped || history[0].GoldReward != accumulatedGold || history[0].ExpReward != accumulatedExp || history[0].MedalsReward != accumulatedMedals {
		t.Errorf("unexpected history record: %#v", history[0])
	}
}

func TestWipeout_ForfeitsLedgerRewards(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"weak": createTestChar("weak", 5, 12, 1, 1),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.StartExpedition(ctx, "weak", "dungeon-01")
	if err != nil {
		t.Fatal(err)
	}

	// Move east to (1, 0)
	res, err := service.Move(ctx, "weak", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}

	if res.EventType == dungeon.EventWipeout {
		if !res.IsFinished {
			t.Errorf("expected isFinished on wipeout")
		}
		// Verify history records wipeout with 0 rewards
		history, _ := service.GetHistory(ctx, "weak", 5)
		if len(history) != 1 || history[0].Outcome != dungeon.StatusWipedOut || history[0].GoldReward != 0 {
			t.Errorf("unexpected wipeout history: %#v", history)
		}
	}
}

func TestService_MonsterDefeatedHook(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"strong": createTestChar("strong", 20, 1000, 500, 500),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(repo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	var hookCalled bool
	var hookCharID string
	var hookCount int
	service.SetMonsterDefeatedHook(func(ctx context.Context, characterID string, count int) error {
		hookCalled = true
		hookCharID = characterID
		hookCount = count
		return nil
	})

	_, err = service.StartExpedition(ctx, "strong", "dungeon-01")
	if err != nil {
		t.Fatal(err)
	}

	// Move east to (1, 0) which is tile '0' triggering monster combat
	res, err := service.Move(ctx, "strong", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}

	if res.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res.EventType)
	}
	if !hookCalled {
		t.Errorf("expected monsterDefeatedHook to be called")
	}
	if hookCharID != "strong" {
		t.Errorf("expected character ID 'strong', got '%s'", hookCharID)
	}
	if hookCount != 1 {
		t.Errorf("expected count 1, got %d", hookCount)
	}
}

type mockCustomActiveStore struct {
	dungeon.ActiveExpeditionStore
	getActiveFn func(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error)
	stepFn      func(ctx context.Context, characterID string, params dungeon.StepParams) (dungeon.StepOutcome, error)
}

func (m *mockCustomActiveStore) GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
	if m.getActiveFn != nil {
		return m.getActiveFn(ctx, characterID)
	}
	return m.ActiveExpeditionStore.GetActiveExpedition(ctx, characterID)
}

func (m *mockCustomActiveStore) Step(ctx context.Context, characterID string, params dungeon.StepParams) (dungeon.StepOutcome, error) {
	if m.stepFn != nil {
		return m.stepFn(ctx, characterID, params)
	}
	return m.ActiveExpeditionStore.Step(ctx, characterID, params)
}

func TestMove_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"advent": createTestChar("advent", 10, 150, 30, 20),
		},
	}
	memStore := dungeon.NewMemoryExpeditionRepository()
	var getActiveErr error
	var stepErr error
	customStore := &mockCustomActiveStore{
		ActiveExpeditionStore: memStore,
		getActiveFn: func(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
			if getActiveErr != nil {
				return nil, getActiveErr
			}
			return memStore.GetActiveExpedition(ctx, characterID)
		},
		stepFn: func(ctx context.Context, characterID string, params dungeon.StepParams) (dungeon.StepOutcome, error) {
			if stepErr != nil {
				return dungeon.StepOutcome{}, stepErr
			}
			return memStore.Step(ctx, characterID, params)
		},
	}

	service, err := dungeon.NewService(repo, charRepo, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(customStore))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Empty characterID
	_, err = service.Move(ctx, "", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound for empty characterID, got %v", err)
	}

	// 2. Active store GetActiveExpedition error
	getActiveErr = errors.New("valkey connection failure")
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if err == nil || !strings.Contains(err.Error(), "valkey connection failure") {
		t.Errorf("expected valkey connection failure, got %v", err)
	}
	getActiveErr = nil

	// 3. No active expedition
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Errorf("expected ErrNoActiveExpedition when none started, got %v", err)
	}

	// 4. Expedition exists but Status != StatusExploring (e.g. StatusCleared)
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:          "exp-done",
		CharacterID: "advent",
		DungeonID:   "dungeon-01",
		Status:      dungeon.StatusCleared,
	})
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Errorf("expected ErrNoActiveExpedition when expedition is cleared, got %v", err)
	}

	// 5. Unknown dungeon ID
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:          "exp-unknown-dungeon",
		CharacterID: "advent",
		DungeonID:   "non-existent-dungeon",
		Status:      dungeon.StatusExploring,
	})
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Errorf("expected ErrDungeonNotFound, got %v", err)
	}

	// 6. Character not found in charRepo
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:          "exp-ghost",
		CharacterID: "ghost-character",
		DungeonID:   "dungeon-01",
		Status:      dungeon.StatusExploring,
	})
	_, err = service.Move(ctx, "ghost-character", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound for ghost character, got %v", err)
	}

	// 7. Invalid floor index: Floor 0 (< 1) and Floor 99 (> Floors)
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:           "exp-bad-floor-0",
		CharacterID:  "advent",
		DungeonID:    "dungeon-01",
		CurrentFloor: 0,
		Status:       dungeon.StatusExploring,
	})
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if err == nil || !strings.Contains(err.Error(), "invalid floor index") {
		t.Errorf("expected invalid floor index for floor 0, got %v", err)
	}

	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:           "exp-bad-floor-99",
		CharacterID:  "advent",
		DungeonID:    "dungeon-01",
		CurrentFloor: 99,
		Status:       dungeon.StatusExploring,
	})
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if err == nil || !strings.Contains(err.Error(), "invalid floor index") {
		t.Errorf("expected invalid floor index for floor 99, got %v", err)
	}

	// 8. Invalid Direction
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:             "exp-valid",
		CharacterID:    "advent",
		DungeonID:      "dungeon-01",
		CurrentFloor:   1,
		PosX:           0,
		PosY:           0,
		CurrentHP:      100,
		TurnsRemaining: 20,
		Status:         dungeon.StatusExploring,
	})
	_, err = service.Move(ctx, "advent", dungeon.Direction("INVALID_DIR"))
	if !errors.Is(err, dungeon.ErrInvalidDirection) {
		t.Errorf("expected ErrInvalidDirection, got %v", err)
	}

	// 9. Impassable bounds (West from 0,0)
	_, err = service.Move(ctx, "advent", dungeon.DirectionWest)
	if !errors.Is(err, dungeon.ErrImpassableWall) {
		t.Errorf("expected ErrImpassableWall for west out of bounds, got %v", err)
	}

	// 10. Active store Step error
	stepErr = errors.New("step transaction failed")
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if err == nil || !strings.Contains(err.Error(), "step transaction failed") {
		t.Errorf("expected step transaction failed, got %v", err)
	}
	stepErr = nil
}

func TestMove_TileEvents_Comprehensive(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": createTestChar("hero", 10, 200, 50, 40),
			"weak": createTestChar("weak", 10, 10, 5, 5),
		},
	}

	customDungeons := []dungeon.Dungeon{
		{
			ID:               "test-dg-01",
			Name:             "実験迷宮",
			MinLevel:         1,
			Tier:             1,
			MaxTurnsPerFloor: 25,
			ClearExpBonus:    100,
			ClearGoldBonus:   200,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       6,
					Height:      2,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"S0TXE?",
						"D11111",
					},
					Monsters: nil,
				},
				{
					FloorNumber: 2,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"SBD",
					},
					Boss: &dungeon.DungeonMonster{
						ID:         "floor-boss",
						Name:       "階層ボス",
						HP:         40,
						Attack:     10,
						Defense:    5,
						ExpReward:  150,
						GoldReward: 300,
						DropItemID: "boss-orb",
					},
				},
			},
		},
		{
			ID:               "test-dg-bossless",
			Name:             "ボス不在迷宮",
			MinLevel:         1,
			Tier:             2,
			MaxTurnsPerFloor: 10,
			ClearExpBonus:    50,
			ClearGoldBonus:   50,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"SBD",
					},
					Boss:     nil,
					Monsters: nil,
				},
			},
		},
	}

	activeStore := dungeon.NewMemoryExpeditionRepository()
	service, err := dungeon.NewService(
		repo,
		charRepo,
		corebattle.Engine{},
		dungeon.WithCustomDungeons(customDungeons),
		dungeon.WithActiveExpeditionStore(activeStore),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start on 1F (0, 0)
	exp, err := service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}

	// Move East to (1, 0) - tile '0' (without monsters)
	res, err := service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to path failed: %v", err)
	}
	if res.EventType != dungeon.EventMove || res.Message != "静かな通路を進んだ。" {
		t.Errorf("unexpected path result: %+v", res)
	}

	// Move West back to (0, 0) - tile 'S'
	res, err = service.Move(ctx, "hero", dungeon.DirectionWest)
	if err != nil {
		t.Fatalf("move to start failed: %v", err)
	}
	if res.EventType != dungeon.EventMove || res.Message != "静かな通路を進んだ。" {
		t.Errorf("unexpected start tile result: %+v", res)
	}

	// Move East to (1, 0) again
	_, _ = service.Move(ctx, "hero", dungeon.DirectionEast)

	// Move East to (2, 0) - tile 'T' (treasure)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to treasure failed: %v", err)
	}
	if res.EventType != dungeon.EventTreasure || res.GoldFound != 100 || res.MedalsFound != 1 || res.ItemFound != "potion" {
		t.Errorf("unexpected treasure result: %+v", res)
	}

	// Move East to (3, 0) - tile 'X' (trap survival: max(10, 200*0.15) = 30 dmg)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to trap failed: %v", err)
	}
	if res.EventType != dungeon.EventTrap || res.DamageTaken != 30 || res.Expedition.CurrentHP != 170 {
		t.Errorf("unexpected trap result: %+v", res)
	}

	// Move East to (4, 0) - tile 'E' (safe escape portal)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to escape failed: %v", err)
	}
	if res.EventType != dungeon.EventEscape || !res.IsFinished {
		t.Errorf("unexpected escape result: %+v", res)
	}
	// Active expedition should be cleaned up
	active, _ := service.GetActiveExpedition(ctx, "hero")
	if active != nil {
		t.Errorf("expected active expedition to be removed after escape")
	}

	// 2. Test Default Tile ('?')
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	exp.PosX = 4
	exp.PosY = 0
	_ = activeStore.SaveActiveExpedition(ctx, *exp)
	// Move East to (5, 0) '?'
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to default tile failed: %v", err)
	}
	if res.EventType != dungeon.EventMove || res.Message != "通路を進んだ。" {
		t.Errorf("unexpected default tile result: %+v", res)
	}
	_, _ = service.Escape(ctx, "hero")

	// 3. Test Stairs descent to 2F, Boss Victory and Dungeon Clear
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	// (0, 0) South to (0, 1) 'D' (Down Stairs)
	res, err = service.Move(ctx, "hero", dungeon.DirectionSouth)
	if err != nil {
		t.Fatalf("move down stairs failed: %v", err)
	}
	if res.EventType != dungeon.EventStairs || res.Expedition.CurrentFloor != 2 || res.Expedition.PosX != 0 || res.Expedition.PosY != 0 || res.Expedition.TurnsRemaining != 25 {
		t.Errorf("unexpected stairs result: %+v", res)
	}

	// On 2F, move East to (1, 0) 'B' (Boss Battle, hero is strong, defeats boss)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to boss failed: %v", err)
	}
	if res.EventType != dungeon.EventBoss || !res.IsFinished || res.Expedition.Status != dungeon.StatusCleared {
		t.Errorf("unexpected boss victory clear: %+v", res)
	}
	if res.ExpEarned < 250 || res.GoldFound < 500 {
		t.Errorf("expected boss rewards added, got exp=%d gold=%d", res.ExpEarned, res.GoldFound)
	}

	// 4. Test Boss Defeat / Wipeout
	exp, err = service.StartExpedition(ctx, "weak", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	// Move down stairs
	_, _ = service.Move(ctx, "weak", dungeon.DirectionSouth)
	// Weak character (HP 10, Atk 5, Def 5) fights boss (HP 40, Atk 10, Def 5)
	res, err = service.Move(ctx, "weak", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("boss combat failed: %v", err)
	}
	if res.EventType != dungeon.EventWipeout || !res.IsFinished || !strings.Contains(res.Message, "フロアボス") {
		t.Errorf("expected wipeout by boss, got %+v", res)
	}

	// 5. Test Boss Tile without Boss or Monsters clears dungeon
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-bossless")
	if err != nil {
		t.Fatal(err)
	}
	// (0, 0) East to (1, 0) 'B' (no boss configured)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to bossless B failed: %v", err)
	}
	if res.EventType != dungeon.EventBoss || !res.IsFinished || res.Expedition.Status != dungeon.StatusCleared {
		t.Errorf("expected bossless clear, got %+v", res)
	}

	// 6. Test Stairs on last floor clears dungeon
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-bossless")
	if err != nil {
		t.Fatal(err)
	}
	// Set position to (1, 0) so next East is (2, 0) 'D' (Down stairs on last floor)
	exp.PosX = 1
	_ = activeStore.SaveActiveExpedition(ctx, *exp)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("move to stairs on last floor failed: %v", err)
	}
	if res.EventType != dungeon.EventBoss || !res.IsFinished || res.Expedition.Status != dungeon.StatusCleared {
		t.Errorf("expected last floor stairs clear, got %+v", res)
	}

	// 7. Test Trap Fatal Wipeout
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	// Set HP low (e.g. 5) and position to (2,0) so trap (30 dmg) at (3,0) wipes out
	exp.PosX = 2
	exp.PosY = 0
	exp.CurrentHP = 5
	if len(exp.Members) > 0 {
		exp.Members[0].CurrentHP = 5
	}
	_ = activeStore.SaveActiveExpedition(ctx, *exp)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast) // moves to (3, 0) 'X'
	if err != nil {
		t.Fatalf("move to fatal trap failed: %v", err)
	}
	if res.EventType != dungeon.EventWipeout || !res.IsFinished || !strings.Contains(res.Message, "罠が作動し") {
		t.Errorf("expected trap wipeout, got %+v", res)
	}

	// 8. Test Turns Depleted Wipeout on Normal Path
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	exp.TurnsRemaining = 1
	_ = activeStore.SaveActiveExpedition(ctx, *exp)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast) // moves to (1, 0) '0'
	if err != nil {
		t.Fatalf("move with 1 turn remaining failed: %v", err)
	}
	if res.EventType != dungeon.EventWipeout || !res.IsFinished || !strings.Contains(res.Message, "行動限界") {
		t.Errorf("expected turns exhaustion wipeout, got %+v", res)
	}

	// 9. Test Turns Depleted Wipeout on Treasure Tile
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	exp.PosX = 1
	exp.TurnsRemaining = 1
	_ = activeStore.SaveActiveExpedition(ctx, *exp)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast) // moves to (2, 0) 'T'
	if err != nil {
		t.Fatalf("move to treasure with 1 turn remaining failed: %v", err)
	}
	if res.EventType != dungeon.EventWipeout || !res.IsFinished || !strings.Contains(res.Message, "行動限界") {
		t.Errorf("expected turns exhaustion wipeout on treasure tile, got %+v", res)
	}

	// 10. Test Turns Depleted Wipeout on Default Tile
	exp, err = service.StartExpedition(ctx, "hero", "test-dg-01")
	if err != nil {
		t.Fatal(err)
	}
	exp.PosX = 4
	exp.TurnsRemaining = 1
	_ = activeStore.SaveActiveExpedition(ctx, *exp)
	res, err = service.Move(ctx, "hero", dungeon.DirectionEast) // moves to (5, 0) '?'
	if err != nil {
		t.Fatalf("move to default tile with 1 turn remaining failed: %v", err)
	}
	if res.EventType != dungeon.EventWipeout || !res.IsFinished || !strings.Contains(res.Message, "行動限界") {
		t.Errorf("expected turns exhaustion wipeout on default tile, got %+v", res)
	}
}

func TestService_GetRecordAndItemEncoding(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{}
	service, err := dungeon.NewService(repo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	rec, err := service.GetRecord(ctx, "hero")
	if err != nil {
		t.Fatalf("GetRecord failed: %v", err)
	}
	if rec.CharacterID != "hero" {
		t.Errorf("expected record for hero, got %+v", rec)
	}

	encoded := dungeon.EncodeItems([]string{"item-1", "item-2"})
	decoded := dungeon.DecodeItems(encoded)
	if len(decoded) != 2 || decoded[0] != "item-1" || decoded[1] != "item-2" {
		t.Errorf("unexpected decoded items: %v", decoded)
	}
}

func TestMove_TreasureConsumedOncePerExpedition(t *testing.T) {
	ctx := context.Background()
	repo := newMockDungeonRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": createTestChar("hero", 10, 200, 50, 40),
		},
	}

	customDungeons := []dungeon.Dungeon{
		{
			ID:               "treasure-dg",
			Name:             "宝箱検証迷宮",
			MinLevel:         1,
			Tier:             1,
			MaxTurnsPerFloor: 30,
			Floors: []dungeon.Floor{
				{
					FloorNumber: 1,
					Width:       4,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"STTD",
					},
				},
				{
					FloorNumber: 2,
					Width:       3,
					Height:      1,
					StartX:      0,
					StartY:      0,
					Grid: []string{
						"STE",
					},
				},
			},
		},
	}

	t.Run("round-trip revisit and different floor treasure", func(t *testing.T) {
		activeStore := dungeon.NewMemoryExpeditionRepository()
		service, err := dungeon.NewService(
			repo,
			charRepo,
			corebattle.Engine{},
			dungeon.WithCustomDungeons(customDungeons),
			dungeon.WithActiveExpeditionStore(activeStore),
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := service.StartExpedition(ctx, "hero", "treasure-dg"); err != nil {
			t.Fatal(err)
		}

		// 1. First visit to 1F (1,0) 'T': grants reward
		res1, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("first move to T failed: %v", err)
		}
		if res1.EventType != dungeon.EventTreasure || res1.GoldFound != 100 || res1.MedalsFound != 1 || res1.ItemFound != "potion" {
			t.Fatalf("expected treasure reward on first visit, got %+v", res1)
		}
		if res1.Expedition.AccumulatedGold != 100 || res1.Expedition.AccumulatedMedals != 1 || len(res1.Expedition.AccumulatedItems) != 1 {
			t.Fatalf("unexpected accumulated rewards after first T: %+v", res1.Expedition)
		}

		// 2. Step back West to (0,0) 'S', then revisit East to (1,0) 'T': move only, no extra reward
		if _, err := service.Move(ctx, "hero", dungeon.DirectionWest); err != nil {
			t.Fatalf("move back to S failed: %v", err)
		}
		resRevisit, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("revisit move to T failed: %v", err)
		}
		if resRevisit.EventType != dungeon.EventMove || resRevisit.GoldFound != 0 || resRevisit.MedalsFound != 0 || resRevisit.ItemFound != "" || resRevisit.ChestsOpened != 0 {
			t.Fatalf("expected EventMove with 0 reward on revisit, got %+v", resRevisit)
		}
		if resRevisit.Expedition.AccumulatedGold != 100 || resRevisit.Expedition.AccumulatedMedals != 1 || len(resRevisit.Expedition.AccumulatedItems) != 1 {
			t.Fatalf("expected accumulated rewards unchanged on revisit, got %+v", resRevisit.Expedition)
		}

		// 3. Save/load (round-trip through store) then revisit (1,0) 'T' again
		loaded, err := activeStore.GetActiveExpedition(ctx, "hero")
		if err != nil || loaded == nil {
			t.Fatalf("GetActiveExpedition failed: %v", err)
		}
		if err := activeStore.SaveActiveExpedition(ctx, *loaded); err != nil {
			t.Fatalf("SaveActiveExpedition failed: %v", err)
		}
		if _, err := service.Move(ctx, "hero", dungeon.DirectionWest); err != nil {
			t.Fatalf("move west before post-save revisit failed: %v", err)
		}
		resAfterSaveLoad, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("revisit after save/load failed: %v", err)
		}
		if resAfterSaveLoad.EventType != dungeon.EventMove || resAfterSaveLoad.Expedition.AccumulatedGold != 100 {
			t.Fatalf("expected consumed state preserved across save/load, got %+v", resAfterSaveLoad)
		}

		// 4. Distinct treasure tile on same floor (2,0) 'T' still grants reward once
		resSecondChest, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("move to second T on 1F failed: %v", err)
		}
		if resSecondChest.EventType != dungeon.EventTreasure || resSecondChest.GoldFound != 100 || resSecondChest.Expedition.AccumulatedGold != 200 {
			t.Fatalf("expected second distinct chest on 1F to grant reward, got %+v", resSecondChest)
		}

		// 5. Descend stairs at (3,0) 'D' to 2F, then visit 2F (1,0) 'T' (same X/Y as 1F chest, different floor)
		resStairs, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil || resStairs.EventType != dungeon.EventStairs || resStairs.Expedition.CurrentFloor != 2 {
			t.Fatalf("expected stairs to 2F, got %+v, err=%v", resStairs, err)
		}
		resFloor2Chest, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("move to 2F T failed: %v", err)
		}
		if resFloor2Chest.EventType != dungeon.EventTreasure || resFloor2Chest.GoldFound != 200 || resFloor2Chest.Expedition.AccumulatedGold != 400 {
			t.Fatalf("expected 2F treasure at (1,0) to grant 200G, got %+v", resFloor2Chest)
		}
	})

	t.Run("retry after step save failure still grants treasure once", func(t *testing.T) {
		memStore := dungeon.NewMemoryExpeditionRepository()
		failNextStep := false
		flakyStore := &mockCustomActiveStore{
			ActiveExpeditionStore: memStore,
			stepFn: func(ctx context.Context, characterID string, params dungeon.StepParams) (dungeon.StepOutcome, error) {
				if failNextStep {
					return dungeon.StepOutcome{}, errors.New("simulated transient step write failure")
				}
				return memStore.Step(ctx, characterID, params)
			},
		}

		service, err := dungeon.NewService(
			repo,
			charRepo,
			corebattle.Engine{},
			dungeon.WithCustomDungeons(customDungeons),
			dungeon.WithActiveExpeditionStore(flakyStore),
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := service.StartExpedition(ctx, "hero", "treasure-dg"); err != nil {
			t.Fatal(err)
		}

		// First attempt fails during Step persistence
		failNextStep = true
		if _, err := service.Move(ctx, "hero", dungeon.DirectionEast); err == nil {
			t.Fatal("expected error when Step fails")
		}

		// Retry succeeds and grants the treasure
		failNextStep = false
		resRetry, err := service.Move(ctx, "hero", dungeon.DirectionEast)
		if err != nil {
			t.Fatalf("retry Move to T failed: %v", err)
		}
		if resRetry.EventType != dungeon.EventTreasure || resRetry.GoldFound != 100 || resRetry.Expedition.AccumulatedGold != 100 {
			t.Fatalf("expected treasure granted on retry after failed step, got %+v", resRetry)
		}
	})

	t.Run("concurrent arrival at same treasure tile awards reward once", func(t *testing.T) {
		singleTreasureDungeons := []dungeon.Dungeon{
			{
				ID:               "single-treasure-dg",
				Name:             "単一宝箱迷宮",
				MinLevel:         1,
				Tier:             1,
				MaxTurnsPerFloor: 30,
				Floors: []dungeon.Floor{
					{
						FloorNumber: 1,
						Width:       3,
						Height:      1,
						StartX:      0,
						StartY:      0,
						Grid: []string{
							"ST1",
						},
					},
				},
			},
		}
		memStore := dungeon.NewMemoryExpeditionRepository()
		service, err := dungeon.NewService(
			repo,
			charRepo,
			corebattle.Engine{},
			dungeon.WithCustomDungeons(singleTreasureDungeons),
			dungeon.WithActiveExpeditionStore(memStore),
		)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := service.StartExpedition(ctx, "hero", "single-treasure-dg"); err != nil {
			t.Fatal(err)
		}

		const workers = 8
		results := make([]dungeon.ExpeditionStepResult, workers)
		errs := make([]error, workers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			idx := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[idx], errs[idx] = service.Move(ctx, "hero", dungeon.DirectionEast)
			}()
		}
		close(start)
		wg.Wait()

		treasureCount := 0
		for i := 0; i < workers; i++ {
			if errs[i] != nil {
				continue
			}
			if results[i].EventType == dungeon.EventTreasure {
				treasureCount++
			}
		}
		if treasureCount != 1 {
			t.Fatalf("expected exactly 1 concurrent Move to claim EventTreasure, got %d", treasureCount)
		}

		exp, err := service.GetActiveExpedition(ctx, "hero")
		if err != nil || exp == nil {
			t.Fatalf("GetActiveExpedition failed: %v", err)
		}
		if exp.AccumulatedGold != 100 || exp.AccumulatedMedals != 1 || len(exp.AccumulatedItems) != 1 {
			t.Fatalf("expected treasure reward accumulated exactly once, got gold=%d medals=%d items=%v",
				exp.AccumulatedGold, exp.AccumulatedMedals, exp.AccumulatedItems)
		}
	})
}
