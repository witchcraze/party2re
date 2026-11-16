package battle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

type mockMonsterTamer struct {
	calls   []string
	inTx    []bool
	boxFull bool
	err     error
	fn      func(ctx context.Context, characterID, monsterID, customName string, callIndex int) error
}

type tameTxMarkerKey struct{}

type markingTxProvider struct {
	committed  int
	rolledBack int
}

func (m *markingTxProvider) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	txCtx := context.WithValue(ctx, tameTxMarkerKey{}, true)
	if err := fn(txCtx); err != nil {
		m.rolledBack++
		return err
	}
	m.committed++
	return nil
}

func (m *mockMonsterTamer) TameMonster(ctx context.Context, characterID, monsterID, customName string) error {
	idx := len(m.calls)
	m.calls = append(m.calls, characterID+":"+monsterID+":"+customName)
	m.inTx = append(m.inTx, ctx.Value(tameTxMarkerKey{}) == true)
	if m.fn != nil {
		return m.fn(ctx, characterID, monsterID, customName, idx)
	}
	if m.boxFull {
		return battle.ErrMonsterBoxFull
	}
	return m.err
}

type mockBlessingProvider struct {
	blessing string
	err      error
	calls    int
}

func (m *mockBlessingProvider) GetActiveBlessing(ctx context.Context, characterID string) (string, error) {
	m.calls++
	if m.err != nil {
		return "", m.err
	}
	return m.blessing, nil
}

type mockMonsterDefeatRecorder struct {
	calls   []string
	records []battle.DefeatedMonsterRecord
	err     error
}

func (m *mockMonsterDefeatRecorder) RecordMonsterDefeat(ctx context.Context, characterID string, record battle.DefeatedMonsterRecord) error {
	if m.err != nil {
		return m.err
	}
	m.calls = append(m.calls, characterID+":"+record.MonsterID+":"+record.MonsterName+":"+record.Habitat)
	m.records = append(m.records, record)
	return nil
}

func TestStrongAndIsStrong(t *testing.T) {
	// strong(p) = int(mhp + mmp + at + df * 0.5 + ag)
	// Example: mhp=100, mmp=50, at=30, df=20, ag=10 -> 100 + 50 + 30 + 10 + 10 = 200
	s := battle.Strong(100, 50, 30, 20, 10)
	if s != 200 {
		t.Fatalf("expected 200, got %d", s)
	}

	// is_strong: strong(enemy) > strong(player) * 0.5
	// If player strong = 200, half = 100.
	// Enemy strong = 101 -> is_strong true.
	if !battle.IsStrongEnemy(101, 200) {
		t.Errorf("expected true when enemyStrong=101 > 100")
	}
	// Enemy strong = 100 -> is_strong false.
	if battle.IsStrongEnemy(100, 200) {
		t.Errorf("expected false when enemyStrong=100 <= 100")
	}
	// Enemy strong = 50 -> is_strong false.
	if battle.IsStrongEnemy(50, 200) {
		t.Errorf("expected false when enemyStrong=50 <= 100")
	}
}

func TestCalculateTamePar(t *testing.T) {
	tests := []struct {
		name               string
		isStrong           bool
		jobID              string
		hasMonsterFood     bool
		hasMonsterBlessing bool
		hasDragonRulerSet  bool
		isCaptured         bool
		expectedPar        float64
	}{
		{
			name:        "base weak enemy, standard job",
			isStrong:    false,
			jobID:       "job-01",
			expectedPar: 2.0,
		},
		{
			name:        "strong enemy, non-tamer drops to 1.0",
			isStrong:    true,
			jobID:       "job-01",
			expectedPar: 1.0,
		},
		{
			name:        "strong enemy, monster tamer (job-12) stays 2.0",
			isStrong:    true,
			jobID:       "job-12",
			expectedPar: 2.0,
		},
		{
			name:           "weak enemy + monster food (item-077)",
			isStrong:       false,
			jobID:          "job-01",
			hasMonsterFood: true,
			expectedPar:    4.0, // 2 + 2
		},
		{
			name:               "weak enemy + chapel blessing 3",
			isStrong:           false,
			jobID:              "job-01",
			hasMonsterBlessing: true,
			expectedPar:        2.5, // 2 + 0.5
		},
		{
			name:              "weak enemy + dragon ruler set (weapon-36 + armor-39)",
			isStrong:          false,
			jobID:             "job-01",
			hasDragonRulerSet: true,
			expectedPar:       2.5, // 2 + 0.5
		},
		{
			name:        "weak enemy + captured state ('捕縛')",
			isStrong:    false,
			jobID:       "job-01",
			isCaptured:  true,
			expectedPar: 6.0, // 2 + 4
		},
		{
			name:               "strong enemy + tamer + food + blessing + gear + captured",
			isStrong:           true,
			jobID:              "job-12",
			hasMonsterFood:     true,
			hasMonsterBlessing: true,
			hasDragonRulerSet:  true,
			isCaptured:         true,
			expectedPar:        9.0, // 2 + 2 + 0.5 + 0.5 + 4
		},
		{
			name:               "strong enemy + non-tamer + food + blessing + gear + captured",
			isStrong:           true,
			jobID:              "job-01",
			hasMonsterFood:     true,
			hasMonsterBlessing: true,
			hasDragonRulerSet:  true,
			isCaptured:         true,
			expectedPar:        8.0, // 1 + 2 + 0.5 + 0.5 + 4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := battle.CalculateTamePar(
				tt.isStrong,
				tt.jobID,
				tt.hasMonsterFood,
				tt.hasMonsterBlessing,
				tt.hasDragonRulerSet,
				tt.isCaptured,
			)
			if got != tt.expectedPar {
				t.Errorf("got par = %v, expected %v", got, tt.expectedPar)
			}
		})
	}
}

func TestCleanMonsterName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"@スライムA", "スライム"},
		{"スライムB", "スライム"},
		{"スライム", "スライム"},
		{"@ドラゴン", "ドラゴン"},
		{"ドットスライムC", "ドットスライム"},
		{"ハイヒーラースライム", "ハイヒーラースライム"},
	}

	for _, tt := range tests {
		got := battle.CleanMonsterName(tt.input)
		if got != tt.expected {
			t.Errorf("CleanMonsterName(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestPostBattleMonsterTamingSuccess(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	txProv := &mockTxProvider{}
	tamer := &mockMonsterTamer{}
	recorder := &mockMonsterDefeatRecorder{}

	char := corecharacter.Character{
		ID:    "c1",
		Name:  "アレックス",
		JobID: "job-01",
		Stats: corecharacter.Stats{
			MaxHP:   100,
			MaxMP:   50,
			Attack:  30,
			Defense: 20,
			Agility: 10,
		},
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := coreinventory.New("c1")
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New("c1")
	_ = equipRepo.Save(ctx, equip)

	// fixedRNG{val: 0} -> Intn(2000) = 0 -> roll succeeds (0 < int(par * 10))
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(fixedRNG{val: 0}),
		battle.WithMonsterTamer(tamer),
		battle.WithMonsterDefeatRecorder(recorder),
	)

	enemy := corebattle.Participant{
		ID:      "monster-002-f1-1",
		Name:    "スライムA",
		MaxHP:   10,
		MaxMP:   0,
		Attack:  5,
		Defense: 5,
		Agility: 5,
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"c1"},
		BattleResult: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			RemainingHP: map[string]int{
				"c1": 100,
			},
		},
		DefeatedEnemies: []corebattle.Participant{enemy},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	tames, ok := resp.MonsterTames["c1"]
	if !ok || len(tames) != 1 {
		t.Fatalf("expected 1 tame result for c1, got %v", tames)
	}

	res := tames[0]
	if !res.Success || res.BoxFull {
		t.Errorf("expected success=true, boxFull=false, got %+v", res)
	}
	if res.MonsterID != "monster-002" {
		t.Errorf("expected MonsterID monster-002, got %q", res.MonsterID)
	}
	if res.MonsterName != "スライム" {
		t.Errorf("expected MonsterName スライム, got %q", res.MonsterName)
	}
	if res.TamerName != "アレックス" {
		t.Errorf("unexpected tamer name: %q", res.TamerName)
	}

	if len(tamer.calls) != 1 || tamer.calls[0] != "c1:monster-002:スライム" {
		t.Errorf("unexpected tamer calls: %v", tamer.calls)
	}
	if len(recorder.calls) != 1 {
		t.Errorf("unexpected recorder calls: %v", recorder.calls)
	}
}

func TestPostBattleMonsterTamingBoxFull(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	txProv := &mockTxProvider{}
	tamer := &mockMonsterTamer{boxFull: true}

	char := corecharacter.Character{
		ID:    "c1",
		Name:  "アレックス",
		JobID: "job-01",
		Stats: corecharacter.Stats{
			MaxHP:   100,
			MaxMP:   50,
			Attack:  30,
			Defense: 20,
			Agility: 10,
		},
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := coreinventory.New("c1")
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New("c1")
	_ = equipRepo.Save(ctx, equip)

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(fixedRNG{val: 0}),
		battle.WithMonsterTamer(tamer),
	)

	enemy := corebattle.Participant{
		ID:      "monster-002-f1-1",
		Name:    "スライムA",
		MaxHP:   10,
		MaxMP:   0,
		Attack:  5,
		Defense: 5,
		Agility: 5,
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"c1"},
		BattleResult: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			RemainingHP: map[string]int{
				"c1": 100,
			},
		},
		DefeatedEnemies: []corebattle.Participant{enemy},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	tames, ok := resp.MonsterTames["c1"]
	if !ok || len(tames) != 1 {
		t.Fatalf("expected 1 tame result for c1, got %v", tames)
	}

	res := tames[0]
	if res.Success || !res.BoxFull {
		t.Errorf("expected success=false, boxFull=true, got %+v", res)
	}
	if res.TamerName != "アレックス" {
		t.Errorf("unexpected tamer name: %q", res.TamerName)
	}
}

func TestPostBattleMonsterTamingRollFailed(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	txProv := &mockTxProvider{}
	tamer := &mockMonsterTamer{}

	char := corecharacter.Character{
		ID:    "c1",
		Name:  "アレックス",
		JobID: "job-01",
		Stats: corecharacter.Stats{
			MaxHP:   100,
			MaxMP:   50,
			Attack:  30,
			Defense: 20,
			Agility: 10,
		},
	}
	_ = charRepo.Update(ctx, char)

	inv, _ := coreinventory.New("c1")
	_ = invRepo.Save(ctx, inv)

	equip, _ := coreequipment.New("c1")
	_ = equipRepo.Save(ctx, equip)

	// fixedRNG{val: 1999} -> Intn(2000) = 1999 -> roll fails (1999 >= int(par * 10))
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(fixedRNG{val: 1999}),
		battle.WithMonsterTamer(tamer),
	)

	enemy := corebattle.Participant{
		ID:      "monster-002-f1-1",
		Name:    "スライムA",
		MaxHP:   10,
		MaxMP:   0,
		Attack:  5,
		Defense: 5,
		Agility: 5,
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"c1"},
		BattleResult: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			RemainingHP: map[string]int{
				"c1": 100,
			},
		},
		DefeatedEnemies: []corebattle.Participant{enemy},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	tames := resp.MonsterTames["c1"]
	if len(tames) != 0 {
		t.Errorf("expected 0 tame results on roll failure, got %v", tames)
	}
	if len(tamer.calls) != 0 {
		t.Errorf("expected 0 tamer calls, got %v", tamer.calls)
	}
}

func TestPostBattleMonsterTaming_WithModifiers_MultiCharacterParty(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	txProv := &mockTxProvider{}
	tamer := &mockMonsterTamer{}
	recorder := &mockMonsterDefeatRecorder{}
	blessingProv := &mockBlessingProvider{blessing: "MONSTER"}

	// c1: leader
	char1 := corecharacter.Character{
		ID:    "c1",
		Name:  "リーダー",
		JobID: "job-01",
		Stats: corecharacter.Stats{MaxHP: 100, MaxMP: 50, Attack: 30, Defense: 20, Agility: 10},
	}
	_ = charRepo.Update(ctx, char1)
	inv1, _ := coreinventory.New("c1")
	_ = invRepo.Save(ctx, inv1)
	equip1, _ := coreequipment.New("c1")
	_ = equipRepo.Save(ctx, equip1)

	// c2: recipient with food item-077, dragon ruler set (weapon-36, armor-39)
	char2 := corecharacter.Character{
		ID:    "c2",
		Name:  "魔物ハンター",
		JobID: "job-01",
		Stats: corecharacter.Stats{MaxHP: 100, MaxMP: 50, Attack: 30, Defense: 20, Agility: 10},
	}
	_ = charRepo.Update(ctx, char2)
	inv2, _ := coreinventory.New("c2")
	foodInst, _ := coreitem.NewInstance("item-077", 1)
	_ = inv2.Add(foodInst)
	weaInst, _ := coreitem.NewInstance("weapon-36", 1)
	_ = inv2.Add(weaInst)
	armInst, _ := coreitem.NewInstance("armor-39", 1)
	_ = inv2.Add(armInst)
	_ = invRepo.Save(ctx, inv2)

	equip2, _ := coreequipment.New("c2")
	equip2.Slots[coreitem.SlotMainHand] = weaInst.ID
	equip2.Slots[coreitem.SlotBody] = armInst.ID
	_ = equipRepo.Save(ctx, equip2)

	// Enemy is strong: strong(enemy) > strong(c2)*0.5 = 200 * 0.5 = 100
	// Enemy strong = 200 > 100. Enemy has Status: "captured"
	// Total par for c2:
	// strong base: 1.0
	// item-077 food: +2.0
	// blessing: +0.5
	// dragon ruler set: +0.5
	// captured: +4.0
	// total par = 8.0 -> threshold = int(8.0 * 10) = 80 out of 2000.
	// With val: 79, 79 < 80 -> succeeds.
	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(fixedRNG{val: 79}),
		battle.WithMonsterTamer(tamer),
		battle.WithMonsterDefeatRecorder(recorder),
		battle.WithBlessingProvider(blessingProv),
	)

	enemy := corebattle.Participant{
		ID:      "monster-010-boss",
		Name:    "ドラゴン",
		MaxHP:   100,
		MaxMP:   50,
		Attack:  30,
		Defense: 20,
		Agility: 10,
		Status:  "captured",
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs:         []string{"c1", "c2"},
		RecipientCharacterID: "c2",
		BattleResult: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			RemainingHP: map[string]int{
				"c1": 100,
				"c2": 100,
			},
		},
		DefeatedEnemies: []corebattle.Participant{enemy},
	}

	resp, err := svc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("ApplyPostBattleResult failed: %v", err)
	}

	// c1 should have 0 tames
	if len(resp.MonsterTames["c1"]) != 0 {
		t.Errorf("expected 0 tames for c1, got %v", resp.MonsterTames["c1"])
	}

	// c2 should have 1 tame
	tames2, ok := resp.MonsterTames["c2"]
	if !ok || len(tames2) != 1 {
		t.Fatalf("expected 1 tame for c2, got %v", resp.MonsterTames["c2"])
	}
	if !tames2[0].Success {
		t.Errorf("expected success for c2 tame")
	}
	if tames2[0].MonsterID != "monster-010" {
		t.Errorf("expected monster-010, got %s", tames2[0].MonsterID)
	}
	if len(tamer.calls) != 1 || tamer.calls[0] != "c2:monster-010:ドラゴン" {
		t.Errorf("unexpected tamer calls: %v", tamer.calls)
	}
}

func TestPostBattleMonsterTamingStorageError(t *testing.T) {
	storageErr := errors.New("monster storage unavailable")

	for _, route := range []string{"single runner", "multi provider"} {
		for _, scenario := range []string{"first fails", "second fails after success", "second fails after box full", "canceled"} {
			t.Run(route+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				charRepo := newMockCharRepo()
				invRepo := newMockInvRepo()
				equipRepo := newMockEquipRepo()
				txProv := &markingTxProvider{}

				ids := []string{"c1"}
				if route == "multi provider" {
					ids = append(ids, "c2")
				}
				for _, id := range ids {
					_ = charRepo.Update(ctx, corecharacter.Character{
						ID:    id,
						Name:  "テイマー-" + id,
						JobID: "job-01",
						Money: 100,
						Stats: corecharacter.Stats{
							HP:      100,
							MaxHP:   100,
							MaxMP:   50,
							Attack:  30,
							Defense: 20,
							Agility: 10,
						},
					})
					inv, _ := coreinventory.New(id)
					_ = invRepo.Save(ctx, inv)
					equip, _ := coreequipment.New(id)
					_ = equipRepo.Save(ctx, equip)
				}

				wantErr := storageErr
				if scenario == "canceled" {
					wantErr = context.Canceled
				}

				tamer := &mockMonsterTamer{
					fn: func(_ context.Context, _, _, _ string, idx int) error {
						switch scenario {
						case "first fails", "canceled":
							return wantErr
						case "second fails after success":
							if idx == 0 {
								return nil
							}
							return wantErr
						case "second fails after box full":
							if idx == 0 {
								return battle.ErrMonsterBoxFull
							}
							return wantErr
						default:
							return nil
						}
					},
				}

				opts := []battle.Option{
					battle.WithCharacterRepository(charRepo),
					battle.WithInventoryRepository(invRepo),
					battle.WithEquipmentRepository(equipRepo),
					battle.WithRandomSource(fixedRNG{val: 0}),
					battle.WithMonsterTamer(tamer),
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
				enemies := []corebattle.Participant{
					{ID: "monster-002-f1-1", Name: "スライムA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
				}
				if scenario == "second fails after success" || scenario == "second fails after box full" {
					enemies = append(enemies, corebattle.Participant{
						ID: "monster-003-f1-2", Name: "ゴブリンA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5,
					})
				}

				resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
					CharacterIDs: ids,
					BattleResult: corebattle.PartyBattleResult{
						Outcome:     corebattle.OutcomeWin,
						RemainingHP: map[string]int{"c1": 50, "c2": 60},
						TotalReward: corebattle.Reward{Currency: 250},
					},
					DefeatedEnemies: enemies,
				})
				if !errors.Is(err, wantErr) {
					t.Fatalf("error = %v, want %v", err, wantErr)
				}
				if len(resp.MonsterTames) != 0 || len(resp.UpdatedCharacters) != 0 {
					t.Fatalf("expected empty response on failure, got %+v", resp)
				}
				if txProv.committed != 0 || txProv.rolledBack != 1 {
					t.Fatalf("expected committed=0 rolledBack=1, got committed=%d rolledBack=%d", txProv.committed, txProv.rolledBack)
				}
				for i, inTx := range tamer.inTx {
					if !inTx {
						t.Fatalf("tamer call %d executed outside transaction boundary", i)
					}
				}
				if route == "single runner" {
					saved, _ := charRepo.FindByID(ctx, "c1")
					if saved.Money != 100 || saved.Stats.HP != 100 {
						t.Fatalf("single runner character mutated despite rollback: %+v", saved)
					}
				}
			})
		}
	}
}

func TestPostBattleMonsterTamingBoxFullFollowedBySuccess(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	equipRepo := newMockEquipRepo()
	txProv := &markingTxProvider{}
	runner, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
	if err != nil {
		t.Fatal(err)
	}

	_ = charRepo.Update(ctx, corecharacter.Character{
		ID:    "c1",
		Name:  "アレックス",
		JobID: "job-01",
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MaxMP: 50, Attack: 30, Defense: 20, Agility: 10},
	})
	inv, _ := coreinventory.New("c1")
	_ = invRepo.Save(ctx, inv)
	equip, _ := coreequipment.New("c1")
	_ = equipRepo.Save(ctx, equip)

	tamer := &mockMonsterTamer{
		fn: func(_ context.Context, _, _, _ string, idx int) error {
			if idx == 0 {
				return battle.ErrMonsterBoxFull
			}
			return nil
		},
	}

	svc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithTransactionRunner(runner),
		battle.WithRandomSource(fixedRNG{val: 0}),
		battle.WithMonsterTamer(tamer),
	)

	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{"c1"},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{"c1": 90},
		},
		DefeatedEnemies: []corebattle.Participant{
			{ID: "monster-002-f1-1", Name: "スライムA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
			{ID: "monster-003-f1-2", Name: "ゴブリンA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error on BoxFull + Success, got %v", err)
	}
	if txProv.committed != 1 || txProv.rolledBack != 0 {
		t.Fatalf("expected committed=1 rolledBack=0, got committed=%d rolledBack=%d", txProv.committed, txProv.rolledBack)
	}
	tames := resp.MonsterTames["c1"]
	if len(tames) != 2 {
		t.Fatalf("expected 2 tame results, got %+v", tames)
	}
	if !tames[0].BoxFull || tames[0].Success {
		t.Errorf("expected first result BoxFull=true Success=false, got %+v", tames[0])
	}
	if !tames[1].Success || tames[1].BoxFull {
		t.Errorf("expected second result Success=true BoxFull=false, got %+v", tames[1])
	}
}

func TestPostBattleMonsterTamingBlessingReadError(t *testing.T) {
	blessingErr := errors.New("blessing store unavailable")
	for _, scenario := range []string{"error", "absent"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			charRepo := newMockCharRepo()
			invRepo := newMockInvRepo()
			equipRepo := newMockEquipRepo()
			txProv := &markingTxProvider{}
			tamer := &mockMonsterTamer{}
			blessingProv := &mockBlessingProvider{}
			if scenario == "error" {
				blessingProv.err = blessingErr
			}

			_ = charRepo.Update(ctx, corecharacter.Character{
				ID:    "c1",
				Name:  "アレックス",
				JobID: "job-01",
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MaxMP: 50, Attack: 30, Defense: 20, Agility: 10},
			})
			inv, _ := coreinventory.New("c1")
			_ = invRepo.Save(ctx, inv)
			equip, _ := coreequipment.New("c1")
			_ = equipRepo.Save(ctx, equip)

			runner, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
			if err != nil {
				t.Fatal(err)
			}
			svc := battle.NewService(
				battle.WithCharacterRepository(charRepo),
				battle.WithInventoryRepository(invRepo),
				battle.WithEquipmentRepository(equipRepo),
				battle.WithTransactionRunner(runner),
				battle.WithRandomSource(fixedRNG{val: 0}),
				battle.WithMonsterTamer(tamer),
				battle.WithBlessingProvider(blessingProv),
			)

			resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
				CharacterIDs:          []string{"c1"},
				RecipientRewards:      map[string]corebattle.Reward{"c1": {}},
				BattleResult:          corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, RemainingHP: map[string]int{"c1": 80}},
				DefeatedEnemies:       []corebattle.Participant{{ID: "monster-002-f1-1", Name: "スライムA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5}},
				PreserveEarnedRewards: true,
			})
			if scenario == "error" {
				if !errors.Is(err, blessingErr) {
					t.Fatalf("error = %v, want %v", err, blessingErr)
				}
				if len(tamer.calls) != 0 {
					t.Fatalf("expected 0 tamer calls on blessing error, got %v", tamer.calls)
				}
				if txProv.committed != 0 || txProv.rolledBack != 1 {
					t.Fatalf("expected rollback, got committed=%d rolledBack=%d", txProv.committed, txProv.rolledBack)
				}
				return
			}
			if err != nil {
				t.Fatalf("absent blessing failed: %v", err)
			}
			if len(resp.MonsterTames["c1"]) != 1 || !resp.MonsterTames["c1"][0].Success {
				t.Fatalf("expected taming success with absent blessing, got %+v", resp.MonsterTames)
			}
		})
	}
}
