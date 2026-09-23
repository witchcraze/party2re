package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

func TestApplyPostBattleResult_DropBlessing_LuckyRoll(t *testing.T) {
	ctx := context.Background()
	// fixedRNG{val: 0}: 0 % 5 == 0 -> lucky roll (< 1 in 5, 20% chance)
	svc, charRepo, invRepo := setupRewardTestService("DROP", 0)

	char := corecharacter.Character{
		ID:       "char-drop-blessed",
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

	// Lucky drop roll adds +1 drop (total 2 drops)
	drops := resp.InventoryDrops[char.ID]
	if len(drops) != 2 {
		t.Errorf("expected 2 inventory drops with lucky drop blessing roll, got %d", len(drops))
	}
}

func TestApplyPostBattleResult_DropBlessing_UnluckyRoll(t *testing.T) {
	ctx := context.Background()
	// fixedRNG{val: 3}: 3 % 5 == 3 != 0 -> unlucky roll
	svc, charRepo, invRepo := setupRewardTestService("DROP", 3)

	char := corecharacter.Character{
		ID:       "char-drop-unlucky",
		Name:     "不運なドロップ信者",
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

	// Unlucky drop roll keeps 1 drop
	drops := resp.InventoryDrops[char.ID]
	if len(drops) != 1 {
		t.Errorf("expected 1 inventory drop with unlucky drop blessing roll, got %d", len(drops))
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
