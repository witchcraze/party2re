package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

func TestApplyPostBattleResult_SingleCharacter_FullDepot_LostDrops(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()

	char := corecharacter.Character{
		ID:         "char-full-depot",
		Name:       "満杯勇者",
		JobID:      "job-warrior",
		Level:      1,
		Experience: 0,
		JobLevel:   1,
		Stats: corecharacter.Stats{
			HP:    100,
			MaxHP: 100,
		},
	}
	_ = charRepo.Update(ctx, char)

	// Inventory full with 2 items (capacity = 2)
	inv, _ := coreinventory.New("char-full-depot")
	item1, _ := coreitem.NewInstance("item-dummy-1", 1)
	item2, _ := coreitem.NewInstance("item-dummy-2", 1)
	_ = inv.Add(item1)
	_ = inv.Add(item2)
	_ = invRepo.Save(ctx, inv)

	// Depot full with 1 item (capacity = 1)
	dep, _ := depot.NewDepot("char-full-depot")
	dep.Capacity = 1
	depotItem, _ := coreitem.NewInstance("item-existing-in-depot", 1)
	_ = dep.AddItem(depotItem)
	_ = depotRepo.Save(ctx, dep)

	txProv := &mockTxProvider{}
	txRunner, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionRunner(txRunner),
		battle.WithMaxInventoryCapacity(2),
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-full-depot": 100,
		},
		TotalReward: corebattle.Reward{
			Experience:       10,
			Currency:         10,
			ItemDefinitionID: "item-rare-drop",
			ItemQuantity:     1,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-full-depot"},
		BattleResult: battleRes,
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// Inventory was full -> 0 inventory drops
	if len(resp.InventoryDrops["char-full-depot"]) != 0 {
		t.Errorf("expected 0 inventory drops, got %d", len(resp.InventoryDrops["char-full-depot"]))
	}
	// Depot was full -> 0 depot deliveries (must NOT falsely report item-rare-drop was delivered)
	if len(resp.DepotDeliveries["char-full-depot"]) != 0 {
		t.Errorf("expected 0 depot deliveries on full depot, got %d", len(resp.DepotDeliveries["char-full-depot"]))
	}
	// LostDrops must contain item-rare-drop
	lost := resp.LostDrops["char-full-depot"]
	if len(lost) != 1 || lost[0].DefinitionID != "item-rare-drop" {
		t.Errorf("expected 1 lost drop (item-rare-drop), got %+v", lost)
	}

	// Verify depot was not modified or corrupted
	savedDepot, _ := depotRepo.FindByCharacterID(ctx, "char-full-depot")
	if len(savedDepot.Items) != 1 || savedDepot.Items[0].DefinitionID != "item-existing-in-depot" {
		t.Errorf("depot items modified unexpectedly: %+v", savedDepot.Items)
	}
}

func TestApplyPostBattleResult_MultiCharacter_FullDepot_LostDrops(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()

	c1 := corecharacter.Character{
		ID:       "char-multi-1",
		Name:     "PartyLeader",
		JobID:    "job-warrior",
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
		JobLevel: 1,
	}
	c2 := corecharacter.Character{
		ID:       "char-multi-2",
		Name:     "PartyMember",
		JobID:    "job-mage",
		Stats:    corecharacter.Stats{HP: 80, MaxHP: 80},
		JobLevel: 1,
	}
	_ = charRepo.Update(ctx, c1)
	_ = charRepo.Update(ctx, c2)

	// c1 inventory full (2/2); depot full (1/1)
	inv1, _ := coreinventory.New("char-multi-1")
	i1, _ := coreitem.NewInstance("dummy-1", 1)
	i2, _ := coreitem.NewInstance("dummy-2", 1)
	_ = inv1.Add(i1)
	_ = inv1.Add(i2)
	_ = invRepo.Save(ctx, inv1)

	dep1, _ := depot.NewDepot("char-multi-1")
	dep1.Capacity = 1
	d1, _ := coreitem.NewInstance("existing-depot-item", 1)
	_ = dep1.AddItem(d1)
	_ = depotRepo.Save(ctx, dep1)

	// c2 inventory full (2/2); depot has space (capacity 2, 0 items)
	inv2, _ := coreinventory.New("char-multi-2")
	i3, _ := coreitem.NewInstance("dummy-3", 1)
	i4, _ := coreitem.NewInstance("dummy-4", 1)
	_ = inv2.Add(i3)
	_ = inv2.Add(i4)
	_ = invRepo.Save(ctx, inv2)

	dep2, _ := depot.NewDepot("char-multi-2")
	dep2.Capacity = 2
	_ = depotRepo.Save(ctx, dep2)

	txProv := &mockTxProvider{}
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithMaxInventoryCapacity(2),
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-multi-1": 100,
			"char-multi-2": 80,
		},
		TotalReward: corebattle.Reward{
			Experience: 20,
			Currency:   20,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-multi-1", "char-multi-2"},
		BattleResult: battleRes,
		DropItems:    []string{"shared-drop-item"},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// c1: full depot -> delivery must be 0, lost must be 1
	if len(resp.DepotDeliveries["char-multi-1"]) != 0 {
		t.Errorf("c1 expected 0 depot deliveries, got %d", len(resp.DepotDeliveries["char-multi-1"]))
	}
	if len(resp.LostDrops["char-multi-1"]) != 1 || resp.LostDrops["char-multi-1"][0].DefinitionID != "shared-drop-item" {
		t.Errorf("c1 expected 1 lost drop (shared-drop-item), got %+v", resp.LostDrops["char-multi-1"])
	}

	// c2: depot has space -> delivery must be 1, lost must be 0
	if len(resp.DepotDeliveries["char-multi-2"]) != 1 || resp.DepotDeliveries["char-multi-2"][0].DefinitionID != "shared-drop-item" {
		t.Errorf("c2 expected 1 depot delivery (shared-drop-item), got %+v", resp.DepotDeliveries["char-multi-2"])
	}
	if len(resp.LostDrops["char-multi-2"]) != 0 {
		t.Errorf("c2 expected 0 lost drops, got %d", len(resp.LostDrops["char-multi-2"]))
	}
}

func TestApplyPostBattleResult_NilDepotRepo_LostDrops(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	char := corecharacter.Character{
		ID:         "char-no-depot-svc",
		Name:       "倉庫無し勇者",
		JobID:      "job-warrior",
		Level:      1,
		Experience: 0,
		JobLevel:   1,
		Stats:      corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)

	// Inventory full with 1 item (capacity = 1)
	inv, _ := coreinventory.New("char-no-depot-svc")
	item1, _ := coreitem.NewInstance("item-dummy-1", 1)
	_ = inv.Add(item1)
	_ = invRepo.Save(ctx, inv)

	txProv := &mockTxProvider{}
	txRunner, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	// Service WITHOUT DepotRepository
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionRunner(txRunner),
		battle.WithMaxInventoryCapacity(1),
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-no-depot-svc": 100,
		},
		TotalReward: corebattle.Reward{
			Experience: 10,
			Currency:   10,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-no-depot-svc"},
		BattleResult: battleRes,
		DropItems:    []string{"item-overflow-drop"},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// With nil depotRepo, overflow drop must NOT be reported as delivered to depot
	if len(resp.DepotDeliveries["char-no-depot-svc"]) != 0 {
		t.Errorf("expected 0 depot deliveries when depotRepo is nil, got %d", len(resp.DepotDeliveries["char-no-depot-svc"]))
	}
	// Must be recorded as LostDrops
	lost := resp.LostDrops["char-no-depot-svc"]
	if len(lost) != 1 || lost[0].DefinitionID != "item-overflow-drop" {
		t.Errorf("expected 1 lost drop (item-overflow-drop), got %+v", lost)
	}
}
