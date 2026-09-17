package battle_test

import (
	"context"
	"fmt"
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

	// Depot full with items matching refreshed capacity for JobLevel
	depCap := depot.CalculateCapacity(char.JobLevel, 0, 0)
	dep, _ := depot.NewDepotWithCapacity("char-full-depot", char.JobLevel, 0, 0)
	for i := 0; i < depCap; i++ {
		depotItem, _ := coreitem.NewInstance(fmt.Sprintf("item-existing-in-depot-%d", i), 1)
		if err := dep.AddItem(depotItem); err != nil {
			t.Fatalf("failed to add item %d: %v", i, err)
		}
	}
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
			Experience:       0,
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
	if len(savedDepot.Items) != depCap {
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
		Level:    1,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
		JobLevel: 1,
	}
	c2 := corecharacter.Character{
		ID:       "char-multi-2",
		Name:     "PartyMember",
		JobID:    "job-mage",
		Level:    1,
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

	depCap1 := depot.CalculateCapacity(c1.JobLevel, 0, 0)
	dep1, _ := depot.NewDepotWithCapacity("char-multi-1", c1.JobLevel, 0, 0)
	for i := 0; i < depCap1; i++ {
		d, _ := coreitem.NewInstance(fmt.Sprintf("existing-depot-item-%d", i), 1)
		if err := dep1.AddItem(d); err != nil {
			t.Fatalf("failed to add item %d to dep1: %v", i, err)
		}
	}
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
			Experience: 0,
			Currency:   20,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-multi-1", "char-multi-2"},
		BattleResult: battleRes,
		RecipientDrops: map[string][]string{
			"char-multi-1": {"drop-item-1"},
			"char-multi-2": {"drop-item-2"},
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// c1: full depot -> delivery must be 0, lost must be 1
	if len(resp.DepotDeliveries["char-multi-1"]) != 0 {
		t.Errorf("c1 expected 0 depot deliveries, got %d", len(resp.DepotDeliveries["char-multi-1"]))
	}
	if len(resp.LostDrops["char-multi-1"]) != 1 || resp.LostDrops["char-multi-1"][0].DefinitionID != "drop-item-1" {
		t.Errorf("c1 expected 1 lost drop (drop-item-1), got %+v", resp.LostDrops["char-multi-1"])
	}

	// c2: depot has space -> delivery must be 1, lost must be 0
	if len(resp.DepotDeliveries["char-multi-2"]) != 1 || resp.DepotDeliveries["char-multi-2"][0].DefinitionID != "drop-item-2" {
		t.Errorf("c2 expected 1 depot delivery (drop-item-2), got %+v", resp.DepotDeliveries["char-multi-2"])
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

func TestApplyPostBattleResult_MultiCharacter_SingleRecipientDrop(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()

	charIDs := []string{"char-p1", "char-p2", "char-p3", "char-p4"}
	for _, id := range charIDs {
		c := corecharacter.Character{
			ID:       id,
			Name:     id,
			JobID:    "job-warrior",
			Level:    1,
			JobLevel: 1,
			Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
		}
		_ = charRepo.Update(ctx, c)
		inv, _ := coreinventory.New(id)
		_ = invRepo.Save(ctx, inv)
	}

	txProv := &mockTxProvider{}
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithMaxInventoryCapacity(5),
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-p1": 100,
			"char-p2": 100,
			"char-p3": 100,
			"char-p4": 100,
		},
		TotalReward: corebattle.Reward{
			Experience:       50,
			Currency:         100,
			ItemDefinitionID: "item-boss-sword",
			ItemQuantity:     1,
		},
	}

	// 1. Without RecipientCharacterID or RecipientDrops: default designated recipient is CharacterIDs[0] (char-p1)
	req1 := battle.ApplyPostBattleRequest{
		CharacterIDs: charIDs,
		BattleResult: battleRes,
	}
	resp1, err := svc.ApplyPostBattleResult(ctx, req1)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}
	if len(resp1.InventoryDrops["char-p1"]) != 1 || resp1.InventoryDrops["char-p1"][0].DefinitionID != "item-boss-sword" {
		t.Errorf("expected char-p1 to receive 1 item-boss-sword, got %+v", resp1.InventoryDrops["char-p1"])
	}
	for _, id := range []string{"char-p2", "char-p3", "char-p4"} {
		if len(resp1.InventoryDrops[id]) != 0 {
			t.Errorf("expected %s to receive 0 drops, got %d", id, len(resp1.InventoryDrops[id]))
		}
	}

	// 2. With RecipientCharacterID explicitly specified (char-p3)
	req2 := battle.ApplyPostBattleRequest{
		CharacterIDs:         charIDs,
		BattleResult:         battleRes,
		RecipientCharacterID: "char-p3",
	}
	resp2, err := svc.ApplyPostBattleResult(ctx, req2)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}
	if len(resp2.InventoryDrops["char-p3"]) != 1 || resp2.InventoryDrops["char-p3"][0].DefinitionID != "item-boss-sword" {
		t.Errorf("expected char-p3 to receive 1 item-boss-sword, got %+v", resp2.InventoryDrops["char-p3"])
	}
	for _, id := range []string{"char-p1", "char-p2", "char-p4"} {
		if len(resp2.InventoryDrops[id]) != 0 {
			t.Errorf("expected %s to receive 0 drops, got %d", id, len(resp2.InventoryDrops[id]))
		}
	}
}

func TestApplyPostBattleResult_ProgressionError_Propagated(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	// Level = 0 is invalid for progression (progression.ErrInvalidCharacterLevel)
	char := corecharacter.Character{
		ID:       "char-invalid-level",
		Name:     "無効勇者",
		JobID:    "job-warrior",
		Level:    0, // invalid level triggers progression error
		JobLevel: 1,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)

	txProv := &mockTxProvider{}
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionProvider(txProv),
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-invalid-level": 100,
		},
		TotalReward: corebattle.Reward{
			Experience: 100,
			Currency:   50,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-invalid-level"},
		BattleResult: battleRes,
	}

	_, err := svc.ApplyPostBattleResult(ctx, req)
	if err == nil {
		t.Fatalf("expected progression error to be propagated, but got nil")
	}
}

func TestApplyPostBattleResult_RemainingStatus(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()

	char := corecharacter.Character{
		ID:       "char-status",
		Name:     "状態異常勇者",
		Level:    1,
		JobLevel: 1,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)

	txProv := &mockTxProvider{}
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionProvider(txProv),
	)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			"char-status": 50,
		},
		RemainingStatus: map[string]string{
			"char-status": "poison",
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"char-status"},
		BattleResult: battleRes,
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	if resp.RemainingStatus == nil || resp.RemainingStatus["char-status"] != "poison" {
		t.Errorf("expected RemainingStatus to contain poison, got: %+v", resp.RemainingStatus)
	}
}
