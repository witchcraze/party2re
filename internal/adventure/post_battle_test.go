package adventure_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type mockPostBattleSettler struct {
	lastReq battle.ApplyPostBattleRequest
	respFn  func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error)
}

func (m *mockPostBattleSettler) ApplyPostBattleResult(_ context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	m.lastReq = req
	if m.respFn != nil {
		return m.respFn(req)
	}
	return battle.ApplyPostBattleResponse{}, nil
}

type fixedAdvClock struct{ now time.Time }

func (c *fixedAdvClock) Now() time.Time { return c.now }

type memoryCharRepo struct {
	chars map[string]corecharacter.Character
}

func (r *memoryCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *memoryCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.chars[c.ID] = c
	return nil
}

type memoryAdvRepo struct {
	adventures map[string]adventure.Adventure
}

func (r *memoryAdvRepo) Save(_ context.Context, a adventure.Adventure) error {
	r.adventures[a.ID] = a
	return nil
}

func (r *memoryAdvRepo) FindByID(_ context.Context, id string) (adventure.Adventure, error) {
	a, ok := r.adventures[id]
	if !ok {
		return adventure.Adventure{}, adventure.ErrNotFound
	}
	return a, nil
}

func (r *memoryAdvRepo) ListByCharacterID(_ context.Context, characterID string, limit, offset int) ([]adventure.Adventure, int, error) {
	return nil, 0, nil
}

func (r *memoryAdvRepo) ListByCharacterIDByCursor(_ context.Context, characterID string, limit int, beforeTime time.Time, beforeID string) ([]adventure.Adventure, error) {
	return nil, nil
}

func (r *memoryAdvRepo) GetAggregatedStats(_ context.Context, characterID string) (adventure.AggregatedStats, error) {
	return adventure.AggregatedStats{}, nil
}

func setupTestAdventureService(t *testing.T, char corecharacter.Character, settler adventure.PostBattleSettler) (*adventure.Service, *memoryCharRepo) {
	t.Helper()
	charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}
	clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}

	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}

	opts := []adventure.Option{}
	if settler != nil {
		opts = append(opts, adventure.WithPostBattleSettler(settler))
	}

	svc, err := adventure.NewServiceWithCatalogs(
		advRepo,
		charRepo,
		nil,
		stages,
		monsters,
		corebattle.Engine{},
		nil,
		nil,
		clock,
		opts...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc, charRepo
}

func TestAdventure_Floor11Treasure_PersistedToInventory(t *testing.T) {
	char, err := corecharacter.New("TreasureSeeker")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 50
	char.Stats.HP = 500
	char.Stats.MaxHP = 500
	char.Stats.MP = 100
	char.Stats.MaxMP = 100
	char.Stats.Attack = 150
	char.Stats.Defense = 100

	settlerCalled := false
	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			settlerCalled = true
			if len(req.RecipientDrops[char.ID]) == 0 {
				t.Errorf("expected RecipientDrops for %s, got none", char.ID)
			}
			inst, _ := coreitem.NewInstance(req.RecipientDrops[char.ID][0], 1)
			return battle.ApplyPostBattleResponse{
				InventoryDrops: map[string][]coreitem.Instance{
					char.ID: {inst},
				},
			}, nil
		},
	}

	svc, _ := setupTestAdventureService(t, char, settler)

	crawlRes, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-01",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	if !settlerCalled {
		t.Fatal("expected PostBattleSettler to be called, but it was not")
	}
	if !crawlRes.StageCleared {
		t.Fatal("expected stage to be cleared")
	}
	if len(crawlRes.TreasureBoxes) == 0 {
		t.Fatal("expected Floor 11 treasure boxes to spawn")
	}
	if crawlRes.TreasureBoxes[0].DeliveredTo != "inventory" {
		t.Errorf("expected treasure box delivered to inventory, got %s", crawlRes.TreasureBoxes[0].DeliveredTo)
	}
}

func TestAdventure_Floor11Treasure_DepotFallbackWhenInventoryFull(t *testing.T) {
	char, err := corecharacter.New("DepotSeeker")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 50
	char.Stats.HP = 500
	char.Stats.MaxHP = 500
	char.Stats.Attack = 150
	char.Stats.Defense = 100

	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			inst, _ := coreitem.NewInstance(req.RecipientDrops[char.ID][0], 1)
			return battle.ApplyPostBattleResponse{
				DepotDeliveries: map[string][]coreitem.Instance{
					char.ID: {inst},
				},
			}, nil
		},
	}

	svc, _ := setupTestAdventureService(t, char, settler)

	crawlRes, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-01",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	if len(crawlRes.TreasureBoxes) == 0 {
		t.Fatal("expected treasure boxes")
	}
	if crawlRes.TreasureBoxes[0].DeliveredTo != "depot" {
		t.Errorf("expected treasure box delivered to depot, got %s", crawlRes.TreasureBoxes[0].DeliveredTo)
	}
}

func TestAdventure_Floor11Treasure_LostDropsWhenDepotFull(t *testing.T) {
	char, err := corecharacter.New("LostSeeker")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 50
	char.Stats.HP = 500
	char.Stats.MaxHP = 500
	char.Stats.Attack = 150
	char.Stats.Defense = 100

	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			inst, _ := coreitem.NewInstance(req.RecipientDrops[char.ID][0], 1)
			return battle.ApplyPostBattleResponse{
				LostDrops: map[string][]coreitem.Instance{
					char.ID: {inst},
				},
			}, nil
		},
	}

	svc, _ := setupTestAdventureService(t, char, settler)

	crawlRes, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-01",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	if len(crawlRes.TreasureBoxes) == 0 {
		t.Fatal("expected treasure boxes")
	}
	if crawlRes.TreasureBoxes[0].DeliveredTo != "lost" {
		t.Errorf("expected treasure box delivered to lost, got %s", crawlRes.TreasureBoxes[0].DeliveredTo)
	}
	if len(crawlRes.LostDrops[char.ID]) == 0 {
		t.Errorf("expected crawlRes.LostDrops to contain item for %s", char.ID)
	}
}

func TestAdventure_Defeat_HPSetToOneAndPersisted(t *testing.T) {
	// Weak character that will be defeated
	char, err := corecharacter.New("Weakling")
	if err != nil {
		t.Fatal(err)
	}
	char.Stats.HP = 10
	char.Stats.MaxHP = 10
	char.Stats.Attack = 1
	char.Stats.Defense = 1

	settlerCalled := false
	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			settlerCalled = true
			if req.BattleResult.Outcome != corebattle.OutcomeDefeat {
				t.Errorf("expected battle outcome defeat, got %v", req.BattleResult.Outcome)
			}
			remHP := req.BattleResult.RemainingHP[char.ID]
			if remHP > 0 {
				t.Errorf("expected defeated participant remaining HP <= 0, got %d", remHP)
			}
			return battle.ApplyPostBattleResponse{}, nil
		},
	}

	svc, _ := setupTestAdventureService(t, char, settler)

	crawlRes, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-01",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	if !settlerCalled {
		t.Fatal("expected settler to be called on defeat")
	}
	if crawlRes.Outcome != corebattle.OutcomeDefeat {
		t.Fatalf("expected outcome defeat, got %v", crawlRes.Outcome)
	}
}
