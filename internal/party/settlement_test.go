package party

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	battleadapter "github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type mockBattleSettler struct {
	err  error
	resp battleadapter.ApplyPostBattleResponse
}

func (m *mockBattleSettler) ApplyPostBattleResult(_ context.Context, _ battleadapter.ApplyPostBattleRequest) (battleadapter.ApplyPostBattleResponse, error) {
	if m.err != nil {
		return battleadapter.ApplyPostBattleResponse{}, m.err
	}
	return m.resp, nil
}

func TestSettleFallback_AddMoneyError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(newMockPartyRepository(), charRepo, invRepo, stageProv, monsterProv, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-1"
	c := corecharacter.Character{
		ID:    charID,
		Name:  "Hero",
		Money: 100,
	}
	charRepo.chars[charID] = c

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalGold: -50, // Invalid negative gold triggers ErrInvalidAmount in AddMoney
		Outcome:   corebattle.OutcomeWin,
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on negative gold in settleFallback, got nil")
	}
	if !strings.Contains(err.Error(), "adding gold for character") {
		t.Errorf("expected error message to contain 'adding gold for character', got: %v", err)
	}
}

func TestSettleFallback_AddCrystalError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(newMockPartyRepository(), charRepo, invRepo, stageProv, monsterProv, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-2"
	c := corecharacter.Character{
		ID:      charID,
		Name:    "Mage",
		Crystal: 10,
	}
	charRepo.chars[charID] = c

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalCrystals: -10, // Invalid negative crystals triggers ErrInvalidAmount in AddCrystal
		Outcome:       corebattle.OutcomeWin,
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on negative crystals in settleFallback, got nil")
	}
	if !strings.Contains(err.Error(), "adding crystals for character") {
		t.Errorf("expected error message to contain 'adding crystals for character', got: %v", err)
	}
}

func TestSettleFallback_CharacterUpdateError(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(newMockPartyRepository(), charRepo, invRepo, stageProv, monsterProv, nil)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-3"
	c := corecharacter.Character{
		ID:   charID,
		Name: "Cleric",
	}
	charRepo.chars[charID] = c

	injectedErr := errors.New("db error updating character")
	charRepo.updateErr = injectedErr

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalGold: 100,
		Outcome:   corebattle.OutcomeWin,
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on character repo update failure, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error %v, got %v", injectedErr, err)
	}
}

func TestSettleWithBattleSettler_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	injectedErr := errors.New("battle adapter settlement failure")
	settler := &mockBattleSettler{err: injectedErr}

	svc, err := NewService(
		newMockPartyRepository(),
		charRepo,
		invRepo,
		stageProv,
		monsterProv,
		nil,
		WithPostBattleSettler(settler),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charID := "char-settle-4"
	c := corecharacter.Character{
		ID:   charID,
		Name: "Warrior",
	}
	charRepo.chars[charID] = c

	members := []Member{{CharacterID: charID, PartyID: "party-1"}}
	charMap := map[string]corecharacter.Character{charID: c}
	crawlResult := &adventure.DungeonCrawlResult{
		TotalGold: 50,
		Outcome:   corebattle.OutcomeWin,
	}

	_, _, err = svc.settlePostBattle(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error from battle settler, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected injected error %v, got %v", injectedErr, err)
	}
}

type capturingBattleSettler struct {
	req battleadapter.ApplyPostBattleRequest
}

func (c *capturingBattleSettler) ApplyPostBattleResult(_ context.Context, req battleadapter.ApplyPostBattleRequest) (battleadapter.ApplyPostBattleResponse, error) {
	c.req = req
	return battleadapter.ApplyPostBattleResponse{}, nil
}

func TestSettleWithBattleSettler_UsesAccumulatedConsumedItems(t *testing.T) {
	settler := &capturingBattleSettler{}
	svc, err := NewService(
		newMockPartyRepository(),
		newMockCharacterRepository(),
		&mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)},
		&mockStageProvider{stages: make(map[string]adventure.Stage)},
		&mockMonsterProvider{monsters: make(map[string]adventure.Monster)},
		nil,
		WithPostBattleSettler(settler),
	)
	if err != nil {
		t.Fatal(err)
	}
	members := []Member{{CharacterID: "a", PartyID: "p"}, {CharacterID: "b", PartyID: "p"}}
	charMap := map[string]corecharacter.Character{"a": {ID: "a"}, "b": {ID: "b"}}
	accumulated := map[string][]corebattle.ConsumedItem{
		"a": {{ID: "item-001", InstanceID: "herb-1", Quantity: 1}, {ID: "item-002", InstanceID: "herb-2", Quantity: 1}},
		"b": {{ID: "item-001", InstanceID: "herb-3", Quantity: 1}},
	}
	crawlResult := &adventure.DungeonCrawlResult{Outcome: corebattle.OutcomeDefeat, ConsumedItems: accumulated}
	// The last floor only consumed one herb; earlier floors must not be lost.
	last := corebattle.PartyBattleResult{ConsumedItems: map[string][]corebattle.ConsumedItem{"a": {{ID: "item-002", InstanceID: "herb-2", Quantity: 1}}}}

	if _, _, err := svc.settlePostBattle(context.Background(), members, charMap, crawlResult, last); err != nil {
		t.Fatal(err)
	}
	if got := settler.req.BattleResult.ConsumedItems; !reflect.DeepEqual(got, accumulated) {
		t.Fatalf("settlement ConsumedItems = %v, want %v", got, accumulated)
	}
}

func TestSettleWithBattleSettler_ConveysUnsealDemonKing(t *testing.T) {
	for _, unseal := range []bool{false, true} {
		t.Run(fmt.Sprintf("unseal=%v", unseal), func(t *testing.T) {
			settler := &capturingBattleSettler{}
			svc, err := NewService(
				newMockPartyRepository(),
				newMockCharacterRepository(),
				&mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)},
				&mockStageProvider{stages: make(map[string]adventure.Stage)},
				&mockMonsterProvider{monsters: make(map[string]adventure.Monster)},
				nil,
				WithPostBattleSettler(settler),
			)
			if err != nil {
				t.Fatal(err)
			}
			members := []Member{{CharacterID: "c1", PartyID: "p1"}}
			charMap := map[string]corecharacter.Character{"c1": {ID: "c1"}}
			crawlResult := &adventure.DungeonCrawlResult{
				Outcome:         corebattle.OutcomeWin,
				UnsealDemonKing: unseal,
			}
			if _, _, err := svc.settlePostBattle(context.Background(), members, charMap, crawlResult, corebattle.PartyBattleResult{}); err != nil {
				t.Fatal(err)
			}
			if settler.req.UnsealDemonKing != unseal {
				t.Fatalf("expected settler.req.UnsealDemonKing = %v, got %v", unseal, settler.req.UnsealDemonKing)
			}
		})
	}
}

func TestSettleFallback_TreasureBoxes_Routing(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	depotRepo := newMockDepotRepo()
	stageProv := &mockStageProvider{stages: make(map[string]adventure.Stage)}
	monsterProv := &mockMonsterProvider{monsters: make(map[string]adventure.Monster)}

	svc, err := NewService(
		newMockPartyRepository(),
		charRepo,
		invRepo,
		stageProv,
		monsterProv,
		nil,
		WithDepotRepository(depotRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// char1: holds a tool, empty weapon/armor hands. Depot full (5/5).
	// char2: holds a tool, empty weapon/armor hands. Depot has space (0/5).
	// char3: empty inventory. Depot full (5/5).
	c1 := corecharacter.Character{ID: "c1", Name: "C1", JobLevel: 0}
	c2 := corecharacter.Character{ID: "c2", Name: "C2", JobLevel: 0}
	c3 := corecharacter.Character{ID: "c3", Name: "C3", JobLevel: 0}
	charRepo.chars["c1"] = c1
	charRepo.chars["c2"] = c2
	charRepo.chars["c3"] = c3

	tool1, _ := coreitem.NewInstance("item-001", 1)
	inv1, _ := coreinventory.New("c1")
	_ = inv1.Add(tool1)
	invRepo.inventories["c1"] = inv1

	tool2, _ := coreitem.NewInstance("item-001", 1)
	inv2, _ := coreinventory.New("c2")
	_ = inv2.Add(tool2)
	invRepo.inventories["c2"] = inv2

	inv3, _ := coreinventory.New("c3")
	invRepo.inventories["c3"] = inv3

	// c1 full depot (5 items)
	dep1, _ := depot.NewDepotWithCapacity("c1", 0, 0, 0)
	for i := 0; i < 5; i++ {
		fill, _ := coreitem.NewInstance(fmt.Sprintf("weapon-fill-%d", i), 1)
		_ = dep1.AddItem(fill)
	}
	_ = depotRepo.Save(ctx, dep1)

	// c2 empty depot (capacity 5)
	dep2, _ := depot.NewDepotWithCapacity("c2", 0, 0, 0)
	_ = depotRepo.Save(ctx, dep2)

	// c3 full depot (5 items)
	dep3, _ := depot.NewDepotWithCapacity("c3", 0, 0, 0)
	for i := 0; i < 5; i++ {
		fill, _ := coreitem.NewInstance(fmt.Sprintf("weapon-fill-%d", i), 1)
		_ = dep3.AddItem(fill)
	}
	_ = depotRepo.Save(ctx, dep3)

	members := []Member{
		{CharacterID: "c1", PartyID: "p"},
		{CharacterID: "c2", PartyID: "p"},
		{CharacterID: "c3", PartyID: "p"},
	}
	charMap := map[string]corecharacter.Character{"c1": c1, "c2": c2, "c3": c3}

	crawlResult := &adventure.DungeonCrawlResult{
		Outcome: corebattle.OutcomeWin,
		TreasureBoxes: []adventure.TreasureBox{
			{OpenedBy: "c1", ItemID: "weapon-01"}, // c1: full depot -> lost
			{OpenedBy: "c2", ItemID: "weapon-01"}, // c2: space in depot -> depot
			{OpenedBy: "c3", ItemID: "item-001"},  // c3: empty tool hand -> inventory
			{OpenedBy: "c1", ItemID: "item-002"},  // c1: tool hand occupied, full depot -> lost
		},
	}

	summaries, lostDrops, err := svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err != nil {
		t.Fatalf("settleFallback failed: %v", err)
	}

	if crawlResult.TreasureBoxes[0].DeliveredTo != "lost" {
		t.Errorf("box 0 delivered_to = %s, want lost", crawlResult.TreasureBoxes[0].DeliveredTo)
	}
	if crawlResult.TreasureBoxes[1].DeliveredTo != "depot" {
		t.Errorf("box 1 delivered_to = %s, want depot", crawlResult.TreasureBoxes[1].DeliveredTo)
	}
	if crawlResult.TreasureBoxes[2].DeliveredTo != "inventory" {
		t.Errorf("box 2 delivered_to = %s, want inventory", crawlResult.TreasureBoxes[2].DeliveredTo)
	}
	if crawlResult.TreasureBoxes[3].DeliveredTo != "lost" {
		t.Errorf("box 3 delivered_to = %s, want lost", crawlResult.TreasureBoxes[3].DeliveredTo)
	}

	// c1 lost drops: weapon-01 and item-002
	if len(lostDrops["c1"]) != 2 || lostDrops["c1"][0] != "weapon-01" || lostDrops["c1"][1] != "item-002" {
		t.Errorf("c1 lostDrops mismatch: %v", lostDrops["c1"])
	}
	// c2 drops: weapon-01 in depot
	var c2Summary, c3Summary *MemberRewardSummary
	for i := range summaries {
		if summaries[i].CharacterID == "c2" {
			c2Summary = &summaries[i]
		}
		if summaries[i].CharacterID == "c3" {
			c3Summary = &summaries[i]
		}
	}
	if c2Summary == nil || len(c2Summary.Drops) != 1 || c2Summary.Drops[0].DefinitionID != "weapon-01" {
		t.Errorf("c2 drops mismatch: %+v", c2Summary)
	}
	if c3Summary == nil || len(c3Summary.Drops) != 1 || c3Summary.Drops[0].DefinitionID != "item-001" {
		t.Errorf("c3 drops mismatch: %+v", c3Summary)
	}
}

type failingDepotSaveRepo struct {
	depot.Depot
	saveErr error
}

func (r *failingDepotSaveRepo) FindByCharacterIDForUpdate(_ context.Context, _ string) (depot.Depot, error) {
	return r.Depot, nil
}

func (r *failingDepotSaveRepo) Save(_ context.Context, _ depot.Depot) error {
	return r.saveErr
}

func TestSettleFallback_TreasureDeliveryError_Propagated(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepository()
	invRepo := &mockInventoryRepository{inventories: make(map[string]coreinventory.Inventory)}
	dep, _ := depot.NewDepotWithCapacity("c1", 0, 0, 0)
	injectedErr := errors.New("simulated depot storage failure")
	failingDepot := &failingDepotSaveRepo{Depot: dep, saveErr: injectedErr}

	svc, err := NewService(
		newMockPartyRepository(),
		charRepo,
		invRepo,
		&mockStageProvider{stages: make(map[string]adventure.Stage)},
		&mockMonsterProvider{monsters: make(map[string]adventure.Monster)},
		nil,
		WithDepotRepository(failingDepot),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	c1 := corecharacter.Character{ID: "c1", Name: "C1", JobLevel: 0}
	charRepo.chars["c1"] = c1
	inv1, _ := coreinventory.New("c1")
	invRepo.inventories["c1"] = inv1

	members := []Member{{CharacterID: "c1", PartyID: "p"}}
	charMap := map[string]corecharacter.Character{"c1": c1}
	crawlResult := &adventure.DungeonCrawlResult{
		Outcome: corebattle.OutcomeWin,
		TreasureBoxes: []adventure.TreasureBox{
			{OpenedBy: "c1", ItemID: "weapon-01"}, // weapon -> routes to depot -> fails on save
		},
	}

	_, _, err = svc.settleFallback(ctx, members, charMap, crawlResult, corebattle.PartyBattleResult{})
	if err == nil {
		t.Fatal("expected error on storage failure during treasure delivery, got nil")
	}
	if !errors.Is(err, injectedErr) {
		t.Errorf("expected %v, got %v", injectedErr, err)
	}
}
