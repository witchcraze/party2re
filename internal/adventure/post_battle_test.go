package adventure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
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
	return setupTestAdventureServiceWithBattle(t, char, settler, corebattle.Engine{})
}

func setupTestAdventureServiceWithBattle(t *testing.T, char corecharacter.Character, settler adventure.PostBattleSettler, resolver corebattle.Resolver) (*adventure.Service, *memoryCharRepo) {
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
		resolver,
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
	char, err := corecharacter.New("TrsSeek")
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
	if settler.lastReq.Habitat == "" {
		t.Errorf("expected Habitat to be populated, got empty")
	}
}

func TestAdventure_Floor11Treasure_DepotFallbackWhenInventoryFull(t *testing.T) {
	char, err := corecharacter.New("DptSeek")
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
	char, err := corecharacter.New("LstSeek")
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
		StageID:      "stage-00",
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

func TestAdventure_StageEX_UnsealDemonKing(t *testing.T) {
	// Strong character capable of clearing stage-20 (封印の地)
	char, err := corecharacter.New("MaoSlay")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 99
	char.JobLevel = 20
	char.Stats.HP = 50000
	char.Stats.MaxHP = 50000
	char.Stats.Attack = 20000
	char.Stats.Defense = 20000
	char.Stats.Agility = 20000

	settler := &mockPostBattleSettler{}
	svc, _ := setupTestAdventureService(t, char, settler)

	// 1. Clear Stage-20 (封印の地 / Stage EX)
	crawlRes, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-20",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl stage-20 failed: %v", err)
	}
	if !crawlRes.StageCleared {
		t.Fatalf("expected stage-20 to be cleared, outcome: %v", crawlRes.Outcome)
	}
	if !settler.lastReq.UnsealDemonKing {
		t.Errorf("expected settler.lastReq.UnsealDemonKing = true for stage-20, got false")
	}

	// 2. Clear Stage-01 (Normal starter stage) -> UnsealDemonKing must be false
	crawlResNormal, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-01",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl stage-01 failed: %v", err)
	}
	if !crawlResNormal.StageCleared {
		t.Fatalf("expected stage-01 to be cleared, outcome: %v", crawlResNormal.Outcome)
	}
	if settler.lastReq.UnsealDemonKing {
		t.Errorf("expected settler.lastReq.UnsealDemonKing = false for stage-01, got true")
	}
}

type txKey struct{}

type spyTxProvider struct {
	txCalls int
}

func (p *spyTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	p.txCalls++
	return fn(context.WithValue(ctx, txKey{}, true))
}

func TestAdventure_SettlementAndHistoryInSameTxBoundary(t *testing.T) {
	c1, _ := corecharacter.New("Hero1")
	c1.Level, c1.Stats.HP, c1.Stats.MaxHP, c1.Stats.Attack, c1.Stats.Defense = 50, 500, 500, 150, 100
	c2, _ := corecharacter.New("Hero2")
	c2.Level, c2.Stats.HP, c2.Stats.MaxHP, c2.Stats.Attack, c2.Stats.Defense = 50, 500, 500, 150, 100

	charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{c1.ID: c1, c2.ID: c2}}
	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()
	clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
	txProv := &spyTxProvider{}

	var settlerInTx bool
	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			return battle.ApplyPostBattleResponse{}, nil
		},
	}
	// Wrap settler to inspect ctx
	txCheckingSettler := &ctxCheckSettler{
		inner: settler,
		onCall: func(ctx context.Context) {
			settlerInTx = ctx.Value(txKey{}) == true
		},
	}

	var saveInTxCount int
	advRepo := &ctxCheckAdvRepo{
		memoryAdvRepo: &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)},
		onSave: func(ctx context.Context) {
			if ctx.Value(txKey{}) == true {
				saveInTxCount++
			}
		},
	}

	svc, err := adventure.NewServiceWithCatalogs(
		advRepo, charRepo, nil, stages, monsters, corebattle.Engine{}, nil, nil, clock,
		adventure.WithPostBattleSettler(txCheckingSettler),
		adventure.WithTransactionProvider(txProv),
	)
	if err != nil {
		t.Fatal(err)
	}

	var hookInTx bool
	var hooksCalled int
	svc.SetVictoryHook(func(ctx context.Context, _ string, _ int, _ int) error {
		hooksCalled++
		if ctx.Value(txKey{}) == true {
			hookInTx = true
		}
		return nil
	})

	_, err = svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{c1.ID, c2.ID},
		StageID:      "stage-01",
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	if txProv.txCalls != 1 {
		t.Fatalf("expected 1 RunInTx call, got %d", txProv.txCalls)
	}
	if !settlerInTx {
		t.Fatal("expected PostBattleSettler to run inside txCtx")
	}
	if saveInTxCount != 2 {
		t.Fatalf("expected 2 adventure Save calls inside txCtx, got %d", saveInTxCount)
	}
	if hooksCalled != 2 || hookInTx {
		t.Fatalf("expected 2 post-commit hooks outside txCtx, got called=%d inTx=%v", hooksCalled, hookInTx)
	}
}

type ctxCheckSettler struct {
	inner  adventure.PostBattleSettler
	onCall func(ctx context.Context)
}

func (s *ctxCheckSettler) ApplyPostBattleResult(ctx context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	if s.onCall != nil {
		s.onCall(ctx)
	}
	return s.inner.ApplyPostBattleResult(ctx, req)
}

type ctxCheckAdvRepo struct {
	*memoryAdvRepo
	onSave func(ctx context.Context)
}

func (r *ctxCheckAdvRepo) Save(ctx context.Context, a adventure.Adventure) error {
	if r.onSave != nil {
		r.onSave(ctx)
	}
	return r.memoryAdvRepo.Save(ctx, a)
}

type staticBlessingProvider struct {
	blessing string
}

func (p staticBlessingProvider) GetActiveBlessing(_ context.Context, _ string) (string, error) {
	return p.blessing, nil
}

type mapBlessingProvider map[string]string

func (m mapBlessingProvider) GetActiveBlessing(_ context.Context, charID string) (string, error) {
	return m[charID], nil
}

type fixedBattleRand struct{ val int }

func (r fixedBattleRand) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	return r.val % n, nil
}

func (r fixedBattleRand) Float64() float64 { return 0 }

type memoryInvRepo struct {
	invs map[string]coreinventory.Inventory
}

func (r *memoryInvRepo) FindByCharacterID(_ context.Context, id string) (coreinventory.Inventory, error) {
	inv, ok := r.invs[id]
	if !ok {
		return coreinventory.New(id)
	}
	return inv, nil
}

func (r *memoryInvRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, id)
}

func (r *memoryInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.invs[inv.CharacterID] = inv
	return nil
}

func (r *memoryInvRepo) Update(ctx context.Context, inv coreinventory.Inventory) error {
	return r.Save(ctx, inv)
}

func (r *memoryInvRepo) AddInstance(ctx context.Context, charID string, inst coreitem.Instance) error {
	inv, _ := r.FindByCharacterID(ctx, charID)
	_ = inv.Add(inst)
	return r.Save(ctx, inv)
}

func (r *memoryInvRepo) RemoveInstance(_ context.Context, _ string, _ string, _ int) error {
	return nil
}

func (r *memoryInvRepo) CountInstances(ctx context.Context, charID string) (int, error) {
	inv, _ := r.FindByCharacterID(ctx, charID)
	return len(inv.Items), nil
}

func (c *memoryCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return c.FindByID(ctx, id)
}

func TestAdventure_TreasureBlessing_SingleChestGenerationNoItemDuplication(t *testing.T) {
	aliases := []string{"DROP", "4", "treasure_wish", "宝箱がほしい"}
	for _, blessing := range aliases {
		t.Run("blessing_"+blessing, func(t *testing.T) {
			ctx := context.Background()
			char, err := corecharacter.New("BlesSolo")
			if err != nil {
				t.Fatal(err)
			}
			char.Level = 50
			char.Stats = corecharacter.Stats{HP: 500, MaxHP: 500, MP: 100, MaxMP: 100, Attack: 150, Defense: 100}

			charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
			invRepo := &memoryInvRepo{invs: make(map[string]coreinventory.Inventory)}
			advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}
			clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
			stages, _ := adventure.InitialStageCatalog()
			monsters, _ := adventure.InitialMonsterCatalog()

			blessingProv := staticBlessingProvider{blessing: blessing}
			battleSvc := battle.NewService(
				battle.WithCharacterRepository(charRepo),
				battle.WithInventoryRepository(invRepo),
				battle.WithBlessingProvider(blessingProv),
				battle.WithRandomSource(fixedBattleRand{val: 0}), // Lucky roll (0 % 5 == 0)
				battle.WithMaxInventoryCapacity(10),
			)

			svc, err := adventure.NewServiceWithCatalogs(
				advRepo, charRepo, nil, stages, monsters, corebattle.Engine{}, nil, nil, clock,
				adventure.WithPostBattleSettler(battleSvc),
				adventure.WithBlessingProvider(blessingProv),
			)
			if err != nil {
				t.Fatal(err)
			}

			// Lucky roll (rng=0): 1 base chest + 1 blessing chest = 2 chests spawned.
			// Without Master Key, solo character examines only 1 chest (1 opened, 1 unopened).
			// Settlement must deliver 1 item and NOT duplicate the acquired item.
			res, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
				CharacterIDs: []string{char.ID},
				StageID:      "stage-01",
				Rng:          func(n int) int { return 0 },
			})
			if err != nil {
				t.Fatalf("ExecuteCrawl failed: %v", err)
			}
			if len(res.TreasureBoxes) != 2 {
				t.Fatalf("expected 2 treasure boxes with lucky blessing roll, got %d", len(res.TreasureBoxes))
			}
			if res.TreasureBoxes[0].OpenedBy != char.ID || res.TreasureBoxes[1].OpenedBy != "" {
				t.Fatalf("expected only 1 box opened by solo character without Master Key, got %+v", res.TreasureBoxes)
			}
			inv, _ := invRepo.FindByCharacterID(ctx, char.ID)
			if len(inv.Items) != 1 {
				t.Fatalf("expected exactly 1 item in inventory (no duplication on unopened extra chest), got %d", len(inv.Items))
			}
		})
	}
}

func TestAdventure_TreasureBlessing_ActorScope(t *testing.T) {
	tests := []struct {
		name        string
		leaderBless string
		memberBless string
		wantChests  int
	}{
		{
			name:        "none",
			leaderBless: "",
			memberBless: "",
			wantChests:  2,
		},
		{
			name:        "leader_only",
			leaderBless: "4",
			memberBless: "",
			wantChests:  3,
		},
		{
			name:        "member_only",
			leaderBless: "",
			memberBless: "4",
			wantChests:  2,
		},
		{
			name:        "both",
			leaderBless: "4",
			memberBless: "4",
			wantChests:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			leader, err := corecharacter.New("Leader")
			if err != nil {
				t.Fatal(err)
			}
			leader.Level = 50
			leader.JobID = "job-01"
			leader.Stats = corecharacter.Stats{HP: 500, MaxHP: 500, MP: 100, MaxMP: 100, Attack: 150, Defense: 100}

			member, err := corecharacter.New("Member")
			if err != nil {
				t.Fatal(err)
			}
			member.Level = 50
			member.JobID = "job-01"
			member.Stats = corecharacter.Stats{HP: 500, MaxHP: 500, MP: 100, MaxMP: 100, Attack: 150, Defense: 100}

			charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{
				leader.ID: leader,
				member.ID: member,
			}}
			invRepo := &memoryInvRepo{invs: make(map[string]coreinventory.Inventory)}
			advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}
			clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
			stages, _ := adventure.InitialStageCatalog()
			monsters, _ := adventure.InitialMonsterCatalog()

			blessingProv := mapBlessingProvider{
				leader.ID: tt.leaderBless,
				member.ID: tt.memberBless,
			}
			battleSvc := battle.NewService(
				battle.WithCharacterRepository(charRepo),
				battle.WithInventoryRepository(invRepo),
				battle.WithBlessingProvider(blessingProv),
				battle.WithRandomSource(fixedBattleRand{val: 0}), // Lucky roll (0 % 5 == 0)
				battle.WithMaxInventoryCapacity(10),
			)

			svc, err := adventure.NewServiceWithCatalogs(
				advRepo, charRepo, nil, stages, monsters, corebattle.Engine{}, nil, nil, clock,
				adventure.WithPostBattleSettler(battleSvc),
				adventure.WithBlessingProvider(blessingProv),
			)
			if err != nil {
				t.Fatal(err)
			}

			res, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
				CharacterIDs: []string{leader.ID, member.ID},
				StageID:      "stage-00",
				Rng:          func(n int) int { return 0 },
			})
			if err != nil {
				t.Fatalf("ExecuteCrawl failed: %v", err)
			}
			if len(res.TreasureBoxes) != tt.wantChests {
				t.Fatalf("expected %d treasure boxes, got %d", tt.wantChests, len(res.TreasureBoxes))
			}
		})
	}
}

func TestAdventure_ExitFatigue_NormalAndCalm_SettlerAndFallback(t *testing.T) {
	ctx := context.Background()
	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()
	clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}

	makeStrongHero := func(id string, initialTired int) corecharacter.Character {
		return corecharacter.Character{
			ID:    id,
			Name:  id,
			Level: 50,
			Tired: initialTired,
			Stats: corecharacter.Stats{HP: 500, MaxHP: 500, MP: 100, MaxMP: 100, Attack: 200, Defense: 100, Agility: 100},
		}
	}
	makeWeakHero := func(id string, initialTired int) corecharacter.Character {
		return corecharacter.Character{
			ID:    id,
			Name:  id,
			Level: 1,
			Tired: initialTired,
			Stats: corecharacter.Stats{HP: 1, MaxHP: 1, MP: 0, MaxMP: 0, Attack: 1, Defense: 1},
		}
	}

	t.Run("battleSettler normal win (Round 11 -> +34) and calm win (Round 11 -> +17)", func(t *testing.T) {
		c1 := makeStrongHero("hero-norm", 10)
		c2 := makeStrongHero("hero-calm", 10)
		charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{c1.ID: c1, c2.ID: c2}}
		invRepo := &memoryInvRepo{invs: make(map[string]coreinventory.Inventory)}
		advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}

		battleSvc := battle.NewService(
			battle.WithCharacterRepository(charRepo),
			battle.WithInventoryRepository(invRepo),
		)
		svc, err := adventure.NewServiceWithCatalogs(
			advRepo, charRepo, nil, stages, monsters, corebattle.Engine{}, nil, nil, clock,
			adventure.WithPostBattleSettler(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		isFull := map[string]bool{c1.ID: true, c2.ID: true}
		svc.SetPostAdventureHook(func(_ context.Context, characterID string) error {
			isFull[characterID] = false
			return nil
		})

		resNorm, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
			CharacterIDs: []string{c1.ID},
			StageID:      "stage-00",
			Mode:         adventure.CrawlModeNormal,
		})
		if err != nil {
			t.Fatalf("normal crawl failed: %v", err)
		}
		if resNorm.Round != 11 || resNorm.ExitFatigue != 34 {
			t.Fatalf("normal crawl Round=%d ExitFatigue=%d, want 11 and 34", resNorm.Round, resNorm.ExitFatigue)
		}
		if got := charRepo.chars[c1.ID].Tired; got != 44 {
			t.Errorf("c1 Tired = %d, want 44 (10 + 34)", got)
		}
		if isFull[c1.ID] {
			t.Errorf("expected c1 meal flag reset after adventure")
		}

		resCalm, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
			CharacterIDs: []string{c2.ID},
			StageID:      "stage-00",
			Mode:         adventure.CrawlModeCalm,
		})
		if err != nil {
			t.Fatalf("calm crawl failed: %v", err)
		}
		if resCalm.Round != 11 || resCalm.ExitFatigue != 17 {
			t.Fatalf("calm crawl Round=%d ExitFatigue=%d, want 11 and 17", resCalm.Round, resCalm.ExitFatigue)
		}
		if got := charRepo.chars[c2.ID].Tired; got != 27 {
			t.Errorf("c2 Tired = %d, want 27 (10 + 17)", got)
		}
		if isFull[c2.ID] {
			t.Errorf("expected c2 meal flag reset after calm adventure")
		}
	})

	t.Run("battleSettler defeat on Floor 1 (Round 1, FloorsCleared 0 -> +4 normal, +2 calm) and resets meal flag", func(t *testing.T) {
		w1 := makeWeakHero("weak-norm", 5)
		w2 := makeWeakHero("weak-calm", 5)
		charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{w1.ID: w1, w2.ID: w2}}
		invRepo := &memoryInvRepo{invs: make(map[string]coreinventory.Inventory)}
		advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}

		battleSvc := battle.NewService(
			battle.WithCharacterRepository(charRepo),
			battle.WithInventoryRepository(invRepo),
		)
		svc, err := adventure.NewServiceWithCatalogs(
			advRepo, charRepo, nil, stages, monsters, corebattle.Engine{}, nil, nil, clock,
			adventure.WithPostBattleSettler(battleSvc),
		)
		if err != nil {
			t.Fatal(err)
		}

		isFull := map[string]bool{w1.ID: true, w2.ID: true}
		svc.SetPostAdventureHook(func(_ context.Context, characterID string) error {
			isFull[characterID] = false
			return nil
		})

		res1, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
			CharacterIDs: []string{w1.ID},
			StageID:      "stage-00",
			Mode:         adventure.CrawlModeNormal,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res1.Outcome != corebattle.OutcomeDefeat || res1.FloorsCleared != 0 || res1.Round != 1 || res1.ExitFatigue != 4 {
			t.Fatalf("got Outcome=%s FloorsCleared=%d Round=%d ExitFatigue=%d, want defeat, 0, 1, 4",
				res1.Outcome, res1.FloorsCleared, res1.Round, res1.ExitFatigue)
		}
		if got := charRepo.chars[w1.ID].Tired; got != 9 {
			t.Errorf("w1 Tired = %d, want 9 (5 + 4)", got)
		}
		if isFull[w1.ID] {
			t.Errorf("expected w1 meal flag reset on defeat")
		}

		res2, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
			CharacterIDs: []string{w2.ID},
			StageID:      "stage-00",
			Mode:         adventure.CrawlModeCalm,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res2.Outcome != corebattle.OutcomeDefeat || res2.FloorsCleared != 0 || res2.Round != 1 || res2.ExitFatigue != 2 {
			t.Fatalf("got Outcome=%s FloorsCleared=%d Round=%d ExitFatigue=%d, want defeat, 0, 1, 2",
				res2.Outcome, res2.FloorsCleared, res2.Round, res2.ExitFatigue)
		}
		if got := charRepo.chars[w2.ID].Tired; got != 7 {
			t.Errorf("w2 Tired = %d, want 7 (5 + 2)", got)
		}
		if isFull[w2.ID] {
			t.Errorf("expected w2 meal flag reset on calm defeat")
		}
	})

	t.Run("fallback settlement path applies exit fatigue for party and clamps at 100", func(t *testing.T) {
		p1 := makeStrongHero("fb-1", 15)
		p2 := makeStrongHero("fb-2", 80)
		charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{p1.ID: p1, p2.ID: p2}}
		advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}

		svc, err := adventure.NewServiceWithCatalogs(
			advRepo, charRepo, nil, stages, monsters, corebattle.Engine{}, nil, nil, clock,
		)
		if err != nil {
			t.Fatal(err)
		}

		res, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
			CharacterIDs: []string{p1.ID, p2.ID},
			StageID:      "stage-00",
			Mode:         adventure.CrawlModeNormal,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitFatigue != 34 {
			t.Fatalf("ExitFatigue = %d, want 34", res.ExitFatigue)
		}
		if got := charRepo.chars[p1.ID].Tired; got != 49 {
			t.Errorf("p1 Tired = %d, want 49 (15 + 34)", got)
		}
		if got := charRepo.chars[p2.ID].Tired; got != 100 {
			t.Errorf("p2 Tired = %d, want 100 (80 + 34 clamped to 100)", got)
		}
	})
}

type mockCountingPartyBattleResolver struct {
	calls   int
	outcome corebattle.Outcome
}

func (m *mockCountingPartyBattleResolver) Resolve(request corebattle.Request) (corebattle.Result, error) {
	m.calls++
	return corebattle.Result{Outcome: corebattle.OutcomeWin}, nil
}

func (m *mockCountingPartyBattleResolver) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	m.calls++
	outcome := m.outcome
	if outcome == "" {
		outcome = corebattle.OutcomeWin
	}
	remHP := make(map[string]int)
	remMP := make(map[string]int)
	for _, a := range req.Allies {
		remHP[a.ID] = a.HP
		remMP[a.ID] = a.MP
	}
	return corebattle.PartyBattleResult{
		Outcome:     outcome,
		Turns:       1,
		RemainingHP: remHP,
		RemainingMP: remMP,
	}, nil
}

type mutableBlessingProvider struct {
	blessings map[string]string
	err       error
}

func (m *mutableBlessingProvider) GetActiveBlessing(_ context.Context, charID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.blessings == nil {
		return "", nil
	}
	return m.blessings[charID], nil
}

func TestAdventure_TreasureBlessing_ReadFailure(t *testing.T) {
	ctx := context.Background()
	char, err := corecharacter.New("BlesSolo")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 80
	char.JobLevel = 50
	char.Stats = corecharacter.Stats{HP: 100, MaxHP: 100, MP: 30, MaxMP: 30, Attack: 50, Defense: 30}
	initialChar := char

	charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	invRepo := &memoryInvRepo{invs: make(map[string]coreinventory.Inventory)}
	advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}
	clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()

	battleResolver := &mockCountingPartyBattleResolver{}
	blessingProv := &mutableBlessingProvider{}

	battleSvc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithBlessingProvider(blessingProv),
		battle.WithRandomSource(fixedBattleRand{val: 0}),
		battle.WithMaxInventoryCapacity(10),
	)

	svc, err := adventure.NewServiceWithCatalogs(
		advRepo, charRepo, nil, stages, monsters, battleResolver, nil, nil, clock,
		adventure.WithPostBattleSettler(battleSvc),
		adventure.WithBlessingProvider(blessingProv),
	)
	if err != nil {
		t.Fatal(err)
	}

	timerSvc := timer.NewService(nil)
	svc.SetTimerService(timerSvc)

	req := adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-26",
		Rng:          func(n int) int { return 0 },
	}

	// 1. Storage failure: error should propagate before battle execution and settlement
	injectedErr := errors.New("blessing storage unavailable")
	blessingProv.err = injectedErr
	battleResolver.calls = 0

	_, err = svc.ExecuteCrawl(ctx, req)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected error wrapping %v, got %v", injectedErr, err)
	}
	if battleResolver.calls != 0 {
		t.Fatalf("expected 0 battle calls on blessing read failure, got %d", battleResolver.calls)
	}
	// Verify stage-26 daily lock was NOT acquired on failure
	locked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatalf("expected stage-26 lock NOT set on blessing failure")
	}
	savedChar, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if savedChar.Stats.HP != initialChar.Stats.HP || savedChar.Money != initialChar.Money || savedChar.Experience != initialChar.Experience {
		t.Fatalf("character mutated on read failure: %+v vs %+v", savedChar, initialChar)
	}
	advList, _, err := advRepo.ListByCharacterID(ctx, char.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(advList) != 0 {
		t.Fatalf("expected 0 adventure logs on failure, got %d", len(advList))
	}

	// 2. Recovery: clearing the injected read failure permits a successful crawl (absent blessing: 1 chest)
	blessingProv.err = nil
	blessingProv.blessings = nil
	battleResolver.calls = 0

	res, err := svc.ExecuteCrawl(ctx, req)
	if err != nil {
		t.Fatalf("expected successful crawl after clearing error, got: %v", err)
	}
	if battleResolver.calls == 0 {
		t.Fatal("expected battle calls on successful crawl")
	}
	if len(res.TreasureBoxes) != 1 {
		t.Fatalf("expected 1 chest for absent blessing, got %d", len(res.TreasureBoxes))
	}
	// Now stage-26 daily lock was set
	lockedAfter, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, char.ID)
	if err != nil || !lockedAfter {
		t.Fatalf("expected stage-26 lock set after successful crawl")
	}

	// 3. Active blessing on normal stage: yields 2 chests (1 base + 1 blessing bonus)
	reqNormal := adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-00",
		Rng:          func(n int) int { return 0 },
	}
	blessingProv.blessings = map[string]string{char.ID: "4"}
	battleResolver.calls = 0

	resActive, err := svc.ExecuteCrawl(ctx, reqNormal)
	if err != nil {
		t.Fatalf("expected successful crawl with active blessing, got: %v", err)
	}
	if len(resActive.TreasureBoxes) != 2 {
		t.Fatalf("expected 2 chests for active blessing, got %d", len(resActive.TreasureBoxes))
	}
}

type mockParticipantBuilderWithItems struct {
	items map[string][]string
}

func (m *mockParticipantBuilderWithItems) BuildParticipant(_ context.Context, charID string) (corebattle.Participant, error) {
	p := corebattle.MustNewParticipant(charID, 50000, 10000, 10000)
	p.ItemDefinitionIDs = m.items[charID]
	return p, nil
}

func TestAdventure_Floor11Treasure_ConsumesMasterKeyForExtraBox(t *testing.T) {
	char, err := corecharacter.New("KeySeek")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 50
	char.JobLevel = 50
	char.Stats = corecharacter.Stats{HP: 50000, MaxHP: 50000, MP: 1000, MaxMP: 1000, Attack: 10000, Defense: 10000}

	var capturedReq battle.ApplyPostBattleRequest
	settlerCalled := false
	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			settlerCalled = true
			capturedReq = req
			invDrops := make(map[string][]coreitem.Instance)
			for cID, drops := range req.RecipientDrops {
				for _, drop := range drops {
					inst, _ := coreitem.NewInstance(drop, 1)
					invDrops[cID] = append(invDrops[cID], inst)
				}
			}
			return battle.ApplyPostBattleResponse{
				InventoryDrops: invDrops,
			}, nil
		},
	}

	charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}
	clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()

	mockBuilder := &mockParticipantBuilderWithItems{
		items: map[string][]string{
			char.ID: {adventure.ItemMasterKey},
		},
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
		adventure.WithPostBattleSettler(settler),
		adventure.WithParticipantBuilder(mockBuilder),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Stage-17 spawns 3 boxes for 1 player (totalMembers * 3 = 3)
	crawlRes, err := svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-17",
		Rng:          func(n int) int { return 0 },
	})
	if err != nil {
		t.Fatalf("ExecuteCrawl failed: %v", err)
	}

	if !settlerCalled {
		t.Fatal("expected PostBattleSettler to be called")
	}
	if !crawlRes.StageCleared {
		t.Fatal("expected stage-17 to be cleared")
	}
	// 4 treasure boxes were spawned on Floor 11 (1*3 + 1)
	if len(crawlRes.TreasureBoxes) != 4 {
		t.Fatalf("expected 4 treasure boxes on stage-17, got %d", len(crawlRes.TreasureBoxes))
	}
	// Character opened 2 boxes: 1 base allowance + 1 with master key
	drops := capturedReq.RecipientDrops[char.ID]
	if len(drops) != 2 {
		t.Fatalf("expected 2 recipient drops, got %d: %v", len(drops), drops)
	}
	consumed := capturedReq.BattleResult.ConsumedItems[char.ID]
	if len(consumed) != 1 || consumed[0].ID != adventure.ItemMasterKey || consumed[0].Quantity != 1 {
		t.Fatalf("expected 1 consumed master key, got: %v", consumed)
	}
}

func TestAdventure_Floor11Treasure_DeliveryFailureRollback(t *testing.T) {
	char, err := corecharacter.New("FailDlvr")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 50
	char.JobLevel = 50
	char.Stats = corecharacter.Stats{HP: 500, MaxHP: 500, MP: 100, MaxMP: 100, Attack: 150, Defense: 100}

	settlerErr := errors.New("simulated delivery settlement failure")
	settler := &mockPostBattleSettler{
		respFn: func(req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
			return battle.ApplyPostBattleResponse{}, settlerErr
		},
	}

	charRepo := &memoryCharRepo{chars: map[string]corecharacter.Character{char.ID: char}}
	advRepo := &memoryAdvRepo{adventures: make(map[string]adventure.Adventure)}
	clock := &fixedAdvClock{now: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
	stages, _ := adventure.InitialStageCatalog()
	monsters, _ := adventure.InitialMonsterCatalog()

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
		adventure.WithPostBattleSettler(settler),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ExecuteCrawl(context.Background(), adventure.DungeonCrawlRequest{
		CharacterIDs: []string{char.ID},
		StageID:      "stage-01",
	})
	if !errors.Is(err, settlerErr) {
		t.Fatalf("expected error %v, got: %v", settlerErr, err)
	}

	if len(advRepo.adventures) != 0 {
		t.Fatalf("expected 0 adventures persisted on delivery rollback, got %d", len(advRepo.adventures))
	}
}
