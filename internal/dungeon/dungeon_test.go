package dungeon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
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

func (m *mockDungeonRepo) DeleteActiveExpedition(ctx context.Context, characterID string) error {
	delete(m.active, characterID)
	return nil
}

func (m *mockDungeonRepo) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	record dungeon.CharacterDungeonRecord,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
) error {
	m.records[record.CharacterID] = record
	m.histories[record.CharacterID] = append([]dungeon.DungeonExpeditionHistory{history}, m.histories[record.CharacterID]...)
	if character != nil {
		m.savedChars[character.ID] = *character
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
