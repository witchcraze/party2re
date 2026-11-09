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
)

func setupRewardTestService(blessing string, rngVal int) (*battle.Service, *mockCharRepo, *mockInvRepo) {
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	txProv := &mockTxProvider{}
	blessingProv := &mockBlessingProvider{blessing: blessing}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithBlessingProvider(blessingProv),
		battle.WithRandomSource(fixedRNG{val: rngVal}),
		battle.WithMaxInventoryCapacity(10),
	)
	return svc, charRepo, invRepo
}

func TestApplyPostBattleResult_GoldBlessing_LuckyRoll(t *testing.T) {
	ctx := context.Background()
	// fixedRNG{val: 0}: 0 % 4 == 0 -> lucky roll (< 1 in 4)
	svc, charRepo, invRepo := setupRewardTestService("GOLD", 0)

	char := corecharacter.Character{
		ID:       "char-gold-blessed",
		Name:     "ゴールド信者",
		Level:    1,
		JobLevel: 1,
		Money:    100,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)
	inv, _ := coreinventory.New(char.ID)
	_ = invRepo.Save(ctx, inv)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			char.ID: 100,
		},
		TotalReward: corebattle.Reward{
			Currency:   100,
			Experience: 0,
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: battleRes,
	})
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// 100 * 1.5 = 150
	if got := resp.GainedGold[char.ID]; got != 150 {
		t.Errorf("expected 150 gained gold with lucky blessing roll, got %d", got)
	}

	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	// Initial 100 + 150 = 250
	if updatedChar.Money != 250 {
		t.Errorf("expected character money 250, got %d", updatedChar.Money)
	}
}

func TestApplyPostBattleResult_GoldBlessing_UnluckyRoll(t *testing.T) {
	ctx := context.Background()
	// fixedRNG{val: 1}: 1 % 4 == 1 != 0 -> unlucky roll
	svc, charRepo, invRepo := setupRewardTestService("GOLD", 1)

	char := corecharacter.Character{
		ID:       "char-gold-unlucky",
		Name:     "不運なゴールド信者",
		Level:    1,
		JobLevel: 1,
		Money:    100,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)
	inv, _ := coreinventory.New(char.ID)
	_ = invRepo.Save(ctx, inv)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			char.ID: 100,
		},
		TotalReward: corebattle.Reward{
			Currency:   100,
			Experience: 0,
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: battleRes,
	})
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// Unlucky roll -> 1.0x (100)
	if got := resp.GainedGold[char.ID]; got != 100 {
		t.Errorf("expected 100 gained gold with unlucky roll, got %d", got)
	}

	updatedChar, _ := charRepo.FindByID(ctx, char.ID)
	// Initial 100 + 100 = 200
	if updatedChar.Money != 200 {
		t.Errorf("expected character money 200, got %d", updatedChar.Money)
	}
}

func TestApplyPostBattleResult_ExpBlessing_LuckyRoll(t *testing.T) {
	ctx := context.Background()
	// fixedRNG{val: 0}: 0 % 4 == 0 -> lucky roll
	svc, charRepo, invRepo := setupRewardTestService("EXP", 0)

	char := corecharacter.Character{
		ID:         "char-exp-blessed",
		Name:       "EXP信者",
		Level:      1,
		JobLevel:   1,
		Experience: 0,
		Stats:      corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)
	inv, _ := coreinventory.New(char.ID)
	_ = invRepo.Save(ctx, inv)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			char.ID: 100,
		},
		TotalReward: corebattle.Reward{
			Currency:   0,
			Experience: 100,
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: battleRes,
	})
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// 100 * 1.5 = 150
	if got := resp.GainedExperience[char.ID]; got != 150 {
		t.Errorf("expected 150 gained exp with lucky blessing roll, got %d", got)
	}
}

func TestApplyPostBattleResult_ExpBlessing_UnluckyRoll(t *testing.T) {
	ctx := context.Background()
	// fixedRNG{val: 2}: 2 % 4 == 2 != 0 -> unlucky roll
	svc, charRepo, invRepo := setupRewardTestService("EXP", 2)

	char := corecharacter.Character{
		ID:         "char-exp-unlucky",
		Name:       "不運なEXP信者",
		Level:      1,
		JobLevel:   1,
		Experience: 0,
		Stats:      corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)
	inv, _ := coreinventory.New(char.ID)
	_ = invRepo.Save(ctx, inv)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			char.ID: 100,
		},
		TotalReward: corebattle.Reward{
			Currency:   0,
			Experience: 100,
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: battleRes,
	})
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// Unlucky roll -> 100
	if got := resp.GainedExperience[char.ID]; got != 100 {
		t.Errorf("expected 100 gained exp with unlucky roll, got %d", got)
	}
}

func TestApplyPostBattleResult_DropBlessing_DoesNotDuplicateDrops(t *testing.T) {
	ctx := context.Background()
	for _, blessing := range []string{"DROP", "4", "treasure_wish", "宝箱がほしい"} {
		t.Run(blessing, func(t *testing.T) {
			// Even with lucky RNG roll (0 % 5 == 0), post-battle settlement must NOT duplicate acquired items;
			// Chapel Blessing 4 ("宝箱がほしい") applies only at the chest-generation boundary.
			svc, charRepo, invRepo := setupRewardTestService(blessing, 0)

			char := corecharacter.Character{
				ID:       "char-drop-" + blessing,
				Name:     "ドロップ信者",
				Level:    1,
				JobLevel: 1,
				Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
			}
			_ = charRepo.Update(ctx, char)
			inv, _ := coreinventory.New(char.ID)
			_ = invRepo.Save(ctx, inv)

			battleRes := corebattle.PartyBattleResult{
				Outcome:    corebattle.OutcomeWin,
				WinnerSide: "allies",
				RemainingHP: map[string]int{
					char.ID: 100,
				},
				TotalReward: corebattle.Reward{
					ItemDefinitionID: "item-001",
					ItemQuantity:     1,
				},
			}

			resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
				CharacterIDs: []string{char.ID},
				BattleResult: battleRes,
			})
			if err != nil {
				t.Fatalf("ApplyPostBattleResult failed: %v", err)
			}

			drops := resp.InventoryDrops[char.ID]
			if len(drops) != 1 {
				t.Errorf("blessing %q: expected 1 inventory drop without settlement duplication, got %d", blessing, len(drops))
			}
		})
	}
}

func TestApplyPostBattleResult_JapaneseBlessingNames(t *testing.T) {
	ctx := context.Background()
	// Test authentic Japanese wish name "お金がほしい"
	svc, charRepo, invRepo := setupRewardTestService("お金がほしい", 0)

	char := corecharacter.Character{
		ID:       "char-jp-blessed",
		Name:     "和風信者",
		Level:    1,
		JobLevel: 1,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)
	inv, _ := coreinventory.New(char.ID)
	_ = invRepo.Save(ctx, inv)

	battleRes := corebattle.PartyBattleResult{
		Outcome:    corebattle.OutcomeWin,
		WinnerSide: "allies",
		RemainingHP: map[string]int{
			char.ID: 100,
		},
		TotalReward: corebattle.Reward{
			Currency: 200,
		},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: battleRes,
	})
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// 200 * 1.5 = 300
	if got := resp.GainedGold[char.ID]; got != 300 {
		t.Errorf("expected 300 gained gold with 'お金がほしい', got %d", got)
	}
}

func TestApplyPostBattleResult_TreasureRoutingByDropKind(t *testing.T) {
	// Tests legacy treasure delivery routing through ApplyPostBattleResult (cmd/party2 production wiring default, maxInvCap == 0).
	// Under _npc_action.cgi:66–78, treasure kind 3 enters an empty tool hand ($m{ite}), while weapons/armor route to depot-or-loss.
	for _, tc := range []struct {
		name       string
		heldIDs    []string
		depotItems int
		treasureID string
		wantInv    int
		wantDepot  int
		wantLost   int
	}{
		{
			name:       "treasure weapon, full depot, empty hands -> lost",
			heldIDs:    nil,
			depotItems: 5,
			treasureID: "weapon-01",
			wantInv:    0,
			wantDepot:  5,
			wantLost:   1,
		},
		{
			name:       "treasure armor, full depot, empty hands -> lost",
			heldIDs:    nil,
			depotItems: 5,
			treasureID: "armor-01",
			wantInv:    0,
			wantDepot:  5,
			wantLost:   1,
		},
		{
			name:       "treasure weapon, depot space, empty hands -> depot",
			heldIDs:    nil,
			depotItems: 0,
			treasureID: "weapon-01",
			wantInv:    0,
			wantDepot:  1,
			wantLost:   0,
		},
		{
			name:       "treasure tool, full depot, empty tool hand -> inventory",
			heldIDs:    nil,
			depotItems: 5,
			treasureID: "item-001",
			wantInv:    1,
			wantDepot:  5,
			wantLost:   0,
		},
		{
			name:       "treasure tool, full depot, occupied tool hand -> lost",
			heldIDs:    []string{"item-002"},
			depotItems: 5,
			treasureID: "item-001",
			wantInv:    1,
			wantDepot:  5,
			wantLost:   1,
		},
		{
			name:       "treasure tool, depot space, occupied tool hand -> depot",
			heldIDs:    []string{"item-002"},
			depotItems: 0,
			treasureID: "item-001",
			wantInv:    1,
			wantDepot:  1,
			wantLost:   0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			charRepo := newMockCharRepo()
			invRepo := newMockInvRepo()
			equipRepo := newMockEquipRepo()
			depotRepo := newMockDepotRepo()
			txProv := &mockTxProvider{}

			// Production battleAdapter options: NO WithMaxInventoryCapacity override
			svc := battle.NewService(
				battle.WithCharacterRepository(charRepo),
				battle.WithInventoryRepository(invRepo),
				battle.WithEquipmentRepository(equipRepo),
				battle.WithDepotRepository(depotRepo),
				battle.WithTransactionProvider(txProv),
			)

			char := corecharacter.Character{
				ID:       "char-treasure-test",
				Name:     "宝箱テスター",
				Level:    1,
				JobLevel: 0, // depot capacity 5
				Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
			}
			_ = charRepo.Update(ctx, char)

			inv, _ := coreinventory.New(char.ID)
			for _, id := range tc.heldIDs {
				inst, _ := coreitem.NewInstance(id, 1)
				_ = inv.Add(inst)
			}
			_ = invRepo.Save(ctx, inv)

			dep, _ := depot.NewDepotWithCapacity(char.ID, 0, 0, 0)
			for i := 0; i < tc.depotItems; i++ {
				dep.AppendDelivery(coreitem.Instance{ID: fmt.Sprintf("dep-%d", i), DefinitionID: "weapon-02", Quantity: 1})
			}
			_ = depotRepo.Save(ctx, dep)

			battleRes := corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeWin,
				WinnerSide:  "allies",
				RemainingHP: map[string]int{char.ID: 100},
			}

			req := battle.ApplyPostBattleRequest{
				CharacterIDs: []string{char.ID},
				BattleResult: battleRes,
				RecipientDrops: map[string][]string{
					char.ID: {tc.treasureID},
				},
			}

			resp, err := svc.ApplyPostBattleResult(ctx, req)
			if err != nil {
				t.Fatalf("ApplyPostBattleResult failed: %v", err)
			}

			if got := len(resp.InventoryDrops[char.ID]); got != tc.wantInv-len(tc.heldIDs) {
				t.Errorf("resp.InventoryDrops = %d, want %d", got, tc.wantInv-len(tc.heldIDs))
			}
			if got := len(resp.DepotDeliveries[char.ID]); got != tc.wantDepot-tc.depotItems {
				t.Errorf("resp.DepotDeliveries = %d, want %d", got, tc.wantDepot-tc.depotItems)
			}
			if got := len(resp.LostDrops[char.ID]); got != tc.wantLost {
				t.Errorf("resp.LostDrops = %d, want %d", got, tc.wantLost)
			}

			savedInv, _ := invRepo.FindByCharacterID(ctx, char.ID)
			if len(savedInv.Items) != tc.wantInv {
				t.Errorf("persisted inventory items = %d, want %d", len(savedInv.Items), tc.wantInv)
			}

			savedDepot, _ := depotRepo.FindByCharacterID(ctx, char.ID)
			if len(savedDepot.Items) != tc.wantDepot {
				t.Errorf("persisted depot items = %d, want %d", len(savedDepot.Items), tc.wantDepot)
			}
		})
	}
}

func TestApplyPostBattleResult_CombatDrop_PreservesInventoryDelivery(t *testing.T) {
	// Combat drops (DropItems / TotalReward) that are NOT treasure boxes still route to inventory
	// when inventory is below maxInventoryCapacity (legacy 1 slot), preserving non-treasure reward behavior.
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	txProv := &mockTxProvider{}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
	)

	char := corecharacter.Character{
		ID:       "char-combat-drop",
		Name:     "戦闘報酬キャラ",
		Level:    1,
		JobLevel: 0,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := coreinventory.New(char.ID)
	_ = invRepo.Save(ctx, inv)

	dep, _ := depot.NewDepotWithCapacity(char.ID, 0, 0, 0)
	_ = depotRepo.Save(ctx, dep)

	battleRes := corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		WinnerSide:  "allies",
		RemainingHP: map[string]int{char.ID: 100},
		TotalReward: corebattle.Reward{
			ItemDefinitionID: "item-combat-trophy",
			ItemQuantity:     1,
		},
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: battleRes,
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	if len(resp.InventoryDrops[char.ID]) != 1 || resp.InventoryDrops[char.ID][0].DefinitionID != "item-combat-trophy" {
		t.Errorf("expected combat drop delivered to inventory, got %+v", resp.InventoryDrops[char.ID])
	}
	if len(resp.DepotDeliveries[char.ID]) != 0 {
		t.Errorf("expected 0 depot deliveries, got %d", len(resp.DepotDeliveries[char.ID]))
	}
	if len(resp.LostDrops[char.ID]) != 0 {
		t.Errorf("expected 0 lost drops, got %d", len(resp.LostDrops[char.ID]))
	}
}
