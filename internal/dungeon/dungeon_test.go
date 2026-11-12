package dungeon_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreprogression "github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type mockDungeonRepo struct {
	active       map[string]*dungeon.ActiveExpedition
	histories    map[string][]dungeon.DungeonExpeditionHistory
	savedChars   map[string]corecharacter.Character
	awardedItems map[string][]coreitem.Instance
}

func newMockDungeonRepo() *mockDungeonRepo {
	return &mockDungeonRepo{
		active:       make(map[string]*dungeon.ActiveExpedition),
		histories:    make(map[string][]dungeon.DungeonExpeditionHistory),
		savedChars:   make(map[string]corecharacter.Character),
		awardedItems: make(map[string][]coreitem.Instance),
	}
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
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
	members ...dungeon.ExpeditionMember,
) error {
	owner := history.CharacterID
	for _, h := range m.histories[owner] {
		if history.ID != "" && h.ID == history.ID {
			return dungeon.ErrExpeditionAlreadyFinished
		}
	}
	m.histories[owner] = append([]dungeon.DungeonExpeditionHistory{history}, m.histories[owner]...)
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
		if mem.HasCurrentMP() {
			c.Stats.MP = mem.CurrentMP
		}
		if history.Outcome == dungeon.StatusEscaped {
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
		m.awardedItems[owner] = append(m.awardedItems[owner], rewardItems...)
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

func TestStartExpedition_SuccessAndGuards(t *testing.T) {
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

	// Retired catalog IDs no longer resolve (#1958).
	if _, err = service.StartExpedition(ctx, "advent", "dungeon-01"); !errors.Is(err, dungeon.ErrDungeonNotFound) {
		t.Errorf("expected ErrDungeonNotFound for retired catalog, got %v", err)
	}

	if _, err := service.StartExpedition(ctx, "advent", dungeon.NativeMap0ID); err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}

	// Reject Starting when already in progress
	_, err = service.StartExpedition(ctx, "advent", dungeon.NativeMap0ID)
	if !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
		t.Errorf("expected ErrActiveExpeditionExists, got %v", err)
	}
}

func TestService_MonsterDefeatedHook(t *testing.T) {
	f := newNativeFixture(t, createTestChar("strong", 20, 1000, 500, 500))
	var hookCharID string
	var hookCount int
	f.svc.SetMonsterDefeatedHook(func(ctx context.Context, characterID string, count int) error {
		hookCharID, hookCount = characterID, count
		return nil
	})
	f.place(t, "strong", 0, 0, 1)
	f.rng.floats = []float64{0.1} // passage spawn
	res, err := f.svc.Move(context.Background(), "strong", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	if res.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle, got %v", res.EventType)
	}
	if hookCharID != "strong" || hookCount != 1 {
		t.Errorf("hook = (%q, %d), want (strong, 1)", hookCharID, hookCount)
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

	// 4. Expedition exists but Status != StatusExploring
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:          "exp-done",
		CharacterID: "advent",
		DungeonID:   dungeon.NativeMap0ID,
		Status:      dungeon.StatusEscaped,
	})
	_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Errorf("expected ErrNoActiveExpedition when expedition is settled, got %v", err)
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
		DungeonID:   dungeon.NativeMap0ID,
		Status:      dungeon.StatusExploring,
	})
	_, err = service.Move(ctx, "ghost-character", dungeon.DirectionEast)
	if !errors.Is(err, dungeon.ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound for ghost character, got %v", err)
	}

	// 7. Invalid layout index: variant 0 (< 1) and 99 (> variants)
	for _, variant := range []int{0, 99} {
		_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
			ID:             "exp-bad-variant",
			CharacterID:    "advent",
			DungeonID:      dungeon.NativeMap0ID,
			MapVariant:     variant,
			TurnsRemaining: 10,
			Status:         dungeon.StatusExploring,
		})
		_, err = service.Move(ctx, "advent", dungeon.DirectionEast)
		if err == nil || !strings.Contains(err.Error(), "invalid floor index") {
			t.Errorf("expected invalid floor index for variant %d, got %v", variant, err)
		}
	}

	// 8. Invalid Direction
	_ = customStore.SaveActiveExpedition(ctx, dungeon.ActiveExpedition{
		ID:             "exp-valid",
		CharacterID:    "advent",
		DungeonID:      dungeon.NativeMap0ID,
		MapVariant:     1,
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

func TestService_ItemEncoding(t *testing.T) {
	encoded := dungeon.EncodeItems([]string{"item-1", "item-2"})
	decoded := dungeon.DecodeItems(encoded)
	if len(decoded) != 2 || decoded[0] != "item-1" || decoded[1] != "item-2" {
		t.Errorf("unexpected decoded items: %v", decoded)
	}
}
