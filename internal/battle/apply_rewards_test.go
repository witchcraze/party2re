package battle_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
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
		{
			// A legacy empty pool entry opens as an empty chest (_npc_action.cgi#shiraberu; #2118).
			name:       "empty chest -> nothing delivered",
			treasureID: "",
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

func TestApplyPostBattleResult_BlessingReadError(t *testing.T) {
	storageErr := errors.New("chapel storage unavailable")

	for _, route := range []string{"single runner", "multi provider"} {
		for _, scenario := range []string{"error", "canceled", "absent", "finalized"} {
			t.Run(route+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				charRepo := newMockCharRepo()
				invRepo := newMockInvRepo()
				equipRepo := newMockEquipRepo()
				txProv := &markingTxProvider{}
				blessingProv := &mockBlessingProvider{}

				var wantErr error
				switch scenario {
				case "error":
					wantErr = storageErr
					blessingProv.err = wantErr
				case "canceled":
					wantErr = context.Canceled
					blessingProv.err = wantErr
				}

				ids := []string{"c1"}
				if route == "multi provider" {
					ids = append(ids, "c2")
				}
				for _, id := range ids {
					_ = charRepo.Update(ctx, corecharacter.Character{
						ID:       id,
						Name:     "キャラ-" + id,
						Level:    1,
						JobLevel: 1,
						Money:    100,
						Stats:    corecharacter.Stats{HP: 100, MaxHP: 100},
					})
					inv, _ := coreinventory.New(id)
					_ = invRepo.Save(ctx, inv)
				}

				opts := []battle.Option{
					battle.WithCharacterRepository(charRepo),
					battle.WithInventoryRepository(invRepo),
					battle.WithEquipmentRepository(equipRepo),
					battle.WithBlessingProvider(blessingProv),
					battle.WithRandomSource(fixedRNG{val: 0}),
				}
				if route == "single runner" {
					runner, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
					if err != nil {
						t.Fatal(err)
					}
					opts = append(opts, battle.WithTransactionRunner(runner))
				} else {
					opts = append(opts, battle.WithTransactionProvider(txProv))
				}

				svc := battle.NewService(opts...)
				req := battle.ApplyPostBattleRequest{
					CharacterIDs: ids,
					BattleResult: corebattle.PartyBattleResult{
						Outcome:     corebattle.OutcomeWin,
						RemainingHP: map[string]int{"c1": 90, "c2": 90},
						TotalReward: corebattle.Reward{Currency: 100, Experience: 50},
					},
				}
				if scenario == "finalized" {
					blessingProv.err = storageErr
					req.RecipientRewards = map[string]corebattle.Reward{
						"c1": {Currency: 100, Experience: 50},
						"c2": {Currency: 100, Experience: 50},
					}
				}

				resp, err := svc.ApplyPostBattleResult(ctx, req)
				if wantErr != nil {
					if !errors.Is(err, wantErr) {
						t.Fatalf("error = %v, want %v", err, wantErr)
					}
					if txProv.committed != 0 || txProv.rolledBack != 1 {
						t.Fatalf("expected rollback, got committed=%d rolledBack=%d", txProv.committed, txProv.rolledBack)
					}
					if route == "single runner" {
						saved, _ := charRepo.FindByID(ctx, "c1")
						if saved.Money != 100 || saved.Experience != 0 {
							t.Fatalf("character mutated despite error: %+v", saved)
						}
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if scenario == "finalized" && blessingProv.calls != 0 {
					t.Fatalf("expected 0 blessing calls when rewards are finalized, got %d", blessingProv.calls)
				}
				if resp.GainedGold["c1"] != 100 {
					t.Fatalf("gained gold = %d, want 100", resp.GainedGold["c1"])
				}
			})
		}
	}
}

type scriptedFloatRNG struct {
	values []float64
	index  int
}

func (r *scriptedFloatRNG) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	return int(r.Float64() * float64(n)), nil
}

func (r *scriptedFloatRNG) Float64() float64 {
	v := r.values[r.index]
	if r.index < len(r.values)-1 {
		r.index++
	}
	return v
}

func setupOrbSettlementService(
	t *testing.T,
	useRunner bool,
	rng corecharacter.RandomSource,
	jobs mockJobProvider,
) (*battle.Service, *mockCharRepo, *mockInvRepo, *mockEquipRepo) {
	t.Helper()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	depotRepo := newMockDepotRepo()
	txProv := &mockTxProvider{}

	opts := []battle.Option{
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithJobDefinitionProvider(jobs),
		battle.WithRandomSource(rng),
	}
	if useRunner {
		runner, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
		if err != nil {
			t.Fatal(err)
		}
		opts = append(opts, battle.WithTransactionRunner(runner))
	} else {
		opts = append(opts, battle.WithTransactionProvider(txProv))
	}
	return battle.NewService(opts...), charRepo, invRepo, equipRepo
}

func TestApplyPostBattleResult_ActiveItemOrbModifiers(t *testing.T) {
	standardJob, err := job.NewDefinition("job-01", "見習い", 2, 2, 2, 2, 2, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	highHPJob, err := job.NewDefinition("job-high", "重戦士", 9, 2, 2, 2, 2, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	jobs := mockJobProvider{
		"job-01":   standardJob,
		"job-high": highHPJob,
	}

	t.Run("equipped stat orb applies with cap re-roll while bag orbs do not", func(t *testing.T) {
		for _, useRunner := range []bool{true, false} {
			ctx := context.Background()
			// HP (job-high, HPGrowth=9 + item-152 => bound 11): 0.95 -> 10 (>9) -> re-roll 0.4 -> int(0.4*9)+1 = 4 (+1 HP min = 5).
			// Other stats (growth=2): int(0.99 * 3) = 2 (no bonus from bag item-154).
			rng := &scriptedFloatRNG{values: []float64{0.95, 0.4, 0.99, 0.99, 0.99, 0.99}}
			svc, charRepo, invRepo, equipRepo := setupOrbSettlementService(t, useRunner, rng, jobs)

			char := corecharacter.Character{
				ID:    "char-active-stat",
				Name:  "宝珠使い",
				JobID: "job-high",
				Level: 1,
				Stats: corecharacter.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6},
			}
			_ = charRepo.Update(ctx, char)

			inv, _ := coreinventory.New(char.ID)
			lifeOrb, _ := coreitem.NewInstance(progression.ItemLifeStatOrb, 1)
			powerOrb, _ := coreitem.NewInstance(progression.ItemPowerStatOrb, 1)
			skillOrb, _ := coreitem.NewInstance(battle.ItemSkillOrb, 1)
			_ = inv.Add(lifeOrb)
			_ = inv.Add(powerOrb)
			_ = inv.Add(skillOrb)
			_ = invRepo.Save(ctx, inv)

			equip, _ := coreequipment.New(char.ID)
			equip.Slots[coreitem.SlotAccessory] = lifeOrb.ID
			_ = equipRepo.Save(ctx, equip)

			resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
				CharacterIDs: []string{char.ID},
				BattleResult: corebattle.PartyBattleResult{
					Outcome:     corebattle.OutcomeWin,
					RemainingHP: map[string]int{char.ID: 30},
					TotalReward: corebattle.Reward{Experience: 10},
				},
			})
			if err != nil {
				t.Fatalf("ApplyPostBattleResult failed: %v", err)
			}
			if resp.LevelUpResults[char.ID].LevelsGained != 1 {
				t.Fatalf("LevelsGained = %d, want 1", resp.LevelUpResults[char.ID].LevelsGained)
			}
			updated, _ := charRepo.FindByID(ctx, char.ID)
			if updated.SP != 1 {
				t.Fatalf("SP = %d, want 1 (inactive bag skill orb must not grant bonus SP)", updated.SP)
			}
			// MaxHP: 30 + (4 + 1) = 35; Attack: 6 + 2 = 8 (no bonus from bag power orb).
			want := corecharacter.Stats{MaxHP: 35, HP: 30, MaxMP: 8, MP: 6, Attack: 8, Defense: 8, Agility: 8}
			if updated.Stats != want {
				t.Fatalf("stats (useRunner=%v) = %+v, want %+v", useRunner, updated.Stats, want)
			}
		}
	})

	t.Run("single held tool stat orb applies without SlotAccessory", func(t *testing.T) {
		ctx := context.Background()
		rng := &scriptedFloatRNG{values: []float64{0.99}}
		svc, charRepo, invRepo, _ := setupOrbSettlementService(t, true, rng, jobs)

		char := corecharacter.Character{
			ID:    "char-held-stat",
			Name:  "単体宝珠",
			JobID: "job-01",
			Level: 1,
			Stats: corecharacter.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6},
		}
		_ = charRepo.Update(ctx, char)

		inv, _ := coreinventory.New(char.ID)
		lifeOrb, _ := coreitem.NewInstance(progression.ItemLifeStatOrb, 1)
		_ = inv.Add(lifeOrb)
		_ = invRepo.Save(ctx, inv)

		_, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
			CharacterIDs: []string{char.ID},
			BattleResult: corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{char.ID: 30},
				TotalReward: corebattle.Reward{Experience: 10},
			},
		})
		if err != nil {
			t.Fatalf("ApplyPostBattleResult failed: %v", err)
		}
		updated, _ := charRepo.FindByID(ctx, char.ID)
		// MaxHP: bound 4 -> int(0.99*4)=3 + 1 = +4 (34). Other stats: bound 3 -> +2.
		want := corecharacter.Stats{MaxHP: 34, HP: 30, MaxMP: 8, MP: 6, Attack: 8, Defense: 8, Agility: 8}
		if updated.Stats != want {
			t.Fatalf("stats = %+v, want %+v", updated.Stats, want)
		}
	})

	t.Run("active skill orb grants bonus SP without applying bag stat orbs", func(t *testing.T) {
		for _, equipped := range []bool{true, false} {
			ctx := context.Background()
			// Draw 0.0 for skill orb (triggers +1 SP), then 0.99 for all 5 stat rolls.
			rng := &scriptedFloatRNG{values: []float64{0.0, 0.99}}
			svc, charRepo, invRepo, equipRepo := setupOrbSettlementService(t, true, rng, jobs)

			char := corecharacter.Character{
				ID:    "char-skill-orb",
				Name:  "スキル宝珠",
				JobID: "job-01",
				Level: 1,
				Stats: corecharacter.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6},
			}
			_ = charRepo.Update(ctx, char)

			inv, _ := coreinventory.New(char.ID)
			skillOrb, _ := coreitem.NewInstance(battle.ItemSkillOrb, 1)
			_ = inv.Add(skillOrb)
			if equipped {
				lifeOrb, _ := coreitem.NewInstance(progression.ItemLifeStatOrb, 1)
				_ = inv.Add(lifeOrb)
				equip, _ := coreequipment.New(char.ID)
				equip.Slots[coreitem.SlotAccessory] = skillOrb.ID
				_ = equipRepo.Save(ctx, equip)
			}
			_ = invRepo.Save(ctx, inv)

			_, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
				CharacterIDs: []string{char.ID},
				BattleResult: corebattle.PartyBattleResult{
					Outcome:     corebattle.OutcomeWin,
					RemainingHP: map[string]int{char.ID: 30},
					TotalReward: corebattle.Reward{Experience: 10},
				},
			})
			if err != nil {
				t.Fatalf("ApplyPostBattleResult failed: %v", err)
			}
			updated, _ := charRepo.FindByID(ctx, char.ID)
			if updated.SP != 2 {
				t.Fatalf("SP (equipped=%v) = %d, want 2", equipped, updated.SP)
			}
			// Even with life orb in bag (when equipped=true), MaxHP gets no stat orb bonus: 30 + (2+1) = 33.
			want := corecharacter.Stats{MaxHP: 33, HP: 30, MaxMP: 8, MP: 6, Attack: 8, Defense: 8, Agility: 8}
			if updated.Stats != want {
				t.Fatalf("stats (equipped=%v) = %+v, want %+v", equipped, updated.Stats, want)
			}
		}
	})

	t.Run("inactive or ambiguous bag orbs grant no stat or SP bonus", func(t *testing.T) {
		for _, tc := range []struct {
			name         string
			equippedDef  string
			consumeEquip bool
		}{
			{name: "other accessory active while orbs in bag", equippedDef: battle.ItemPrayerRing},
			{name: "ambiguous multi-item bag with no accessory slot", equippedDef: ""},
			{name: "active non-orb consumed leaving bag orb as sole item", equippedDef: battle.ItemPrayerRing, consumeEquip: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx := context.Background()
				// 0.0 would trigger skill orb if active; 0.99 for stats would hit +1 orb bound if active.
				rng := &scriptedFloatRNG{values: []float64{0.99}}
				svc, charRepo, invRepo, equipRepo := setupOrbSettlementService(t, true, rng, jobs)

				char := corecharacter.Character{
					ID:    "char-bag-only",
					Name:  "鞄の宝珠",
					JobID: "job-01",
					Level: 1,
					Stats: corecharacter.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6},
				}
				_ = charRepo.Update(ctx, char)

				inv, _ := coreinventory.New(char.ID)
				lifeOrb, _ := coreitem.NewInstance(progression.ItemLifeStatOrb, 1)
				powerOrb, _ := coreitem.NewInstance(progression.ItemPowerStatOrb, 1)
				_ = inv.Add(lifeOrb)

				var consumed []corebattle.ConsumedItem
				if tc.equippedDef != "" {
					ring, _ := coreitem.NewInstance(tc.equippedDef, 1)
					_ = inv.Add(ring)
					equip, _ := coreequipment.New(char.ID)
					equip.Slots[coreitem.SlotAccessory] = ring.ID
					_ = equipRepo.Save(ctx, equip)
					if tc.consumeEquip {
						consumed = append(consumed, corebattle.ConsumedItem{ID: ring.DefinitionID, InstanceID: ring.ID, Quantity: 1})
					} else {
						_ = inv.Add(powerOrb)
					}
				} else {
					skillOrb, _ := coreitem.NewInstance(battle.ItemSkillOrb, 1)
					_ = inv.Add(skillOrb)
					_ = inv.Add(powerOrb)
				}
				_ = invRepo.Save(ctx, inv)

				// Also verify skill orb does not trigger with draw 0.0 when inactive (consumed by HP instead).
				wantMaxHP := 33
				if !tc.consumeEquip {
					rng.values = []float64{0.0, 0.99}
					wantMaxHP = 31
				}

				_, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
					CharacterIDs: []string{char.ID},
					BattleResult: corebattle.PartyBattleResult{
						Outcome:       corebattle.OutcomeWin,
						RemainingHP:   map[string]int{char.ID: 30},
						TotalReward:   corebattle.Reward{Experience: 10},
						ConsumedItems: map[string][]corebattle.ConsumedItem{char.ID: consumed},
					},
				})
				if err != nil {
					t.Fatalf("ApplyPostBattleResult failed: %v", err)
				}
				updated, _ := charRepo.FindByID(ctx, char.ID)
				if updated.SP != 1 {
					t.Fatalf("SP = %d, want 1", updated.SP)
				}
				want := corecharacter.Stats{MaxHP: wantMaxHP, HP: 30, MaxMP: 8, MP: 6, Attack: 8, Defense: 8, Agility: 8}
				if updated.Stats != want {
					t.Fatalf("stats = %+v, want %+v", updated.Stats, want)
				}
			})
		}
	})
}

func TestApplyPostBattleResult_Job49PreservesRequestStartSP(t *testing.T) {
	onionDef := job.Definition{ID: "job-49", Name: "たまねぎ剣士"}
	jobs := mockJobProvider{"job-49": onionDef}

	t.Run("SP 25 uses fractional bound 1.5", func(t *testing.T) {
		ctx := context.Background()
		rng := &scriptedFloatRNG{values: []float64{0.8}}
		svc, charRepo, invRepo, _ := setupOrbSettlementService(t, true, rng, jobs)

		char := corecharacter.Character{
			ID:    "char-onion-25",
			Name:  "若芽25",
			JobID: "job-49",
			Level: 1,
			SP:    25,
			Stats: corecharacter.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6},
		}
		_ = charRepo.Update(ctx, char)
		inv, _ := coreinventory.New(char.ID)
		_ = invRepo.Save(ctx, inv)

		_, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
			CharacterIDs: []string{char.ID},
			BattleResult: corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{char.ID: 30},
				TotalReward: corebattle.Reward{Experience: 10},
			},
		})
		if err != nil {
			t.Fatalf("ApplyPostBattleResult failed: %v", err)
		}
		updated, _ := charRepo.FindByID(ctx, char.ID)
		want := corecharacter.Stats{MaxHP: 32, HP: 30, MaxMP: 7, MP: 6, Attack: 7, Defense: 7, Agility: 7}
		if updated.Stats != want || updated.SP != 26 || updated.Level != 2 {
			t.Fatalf("updated = %+v, want stats %+v SP 26 Level 2", updated, want)
		}
	})

	t.Run("SP 49 across two levels with active skill orb preserves bound 1.98", func(t *testing.T) {
		ctx := context.Background()
		rng := &scriptedFloatRNG{values: []float64{
			0.0, 0.502, 0.502, 0.502, 0.502, 0.502,
			0.0, 0.502, 0.502, 0.502, 0.502, 0.502,
		}}
		svc, charRepo, invRepo, equipRepo := setupOrbSettlementService(t, false, rng, jobs)

		char := corecharacter.Character{
			ID:    "char-onion-49",
			Name:  "若芽49",
			JobID: "job-49",
			Level: 1,
			SP:    49,
			Stats: corecharacter.Stats{MaxHP: 30, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6},
		}
		_ = charRepo.Update(ctx, char)
		inv, _ := coreinventory.New(char.ID)
		skillOrb, _ := coreitem.NewInstance(battle.ItemSkillOrb, 1)
		_ = inv.Add(skillOrb)
		_ = invRepo.Save(ctx, inv)
		equip, _ := coreequipment.New(char.ID)
		equip.Slots[coreitem.SlotAccessory] = skillOrb.ID
		_ = equipRepo.Save(ctx, equip)

		_, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
			CharacterIDs: []string{char.ID},
			BattleResult: corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{char.ID: 30},
				TotalReward: corebattle.Reward{Experience: 40},
			},
		})
		if err != nil {
			t.Fatalf("ApplyPostBattleResult failed: %v", err)
		}
		updated, _ := charRepo.FindByID(ctx, char.ID)
		want := corecharacter.Stats{MaxHP: 32, HP: 30, MaxMP: 6, MP: 6, Attack: 6, Defense: 6, Agility: 6}
		if updated.Stats != want || updated.SP != 53 || updated.Level != 3 {
			t.Fatalf("updated = %+v, want stats %+v SP 53 Level 3", updated, want)
		}
	})
}
