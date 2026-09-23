package battle_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type mockMonsterTamer struct {
	calls   []string
	boxFull bool
	err     error
}

func (m *mockMonsterTamer) TameMonster(ctx context.Context, characterID, monsterID, customName string) error {
	m.calls = append(m.calls, characterID+":"+monsterID+":"+customName)
	if m.boxFull {
		return battle.ErrMonsterBoxFull
	}
	return m.err
}

type mockBlessingProvider struct {
	blessing string
}

func (m *mockBlessingProvider) GetActiveBlessing(ctx context.Context, characterID string) (string, error) {
	return m.blessing, nil
}

type mockMonsterDefeatRecorder struct {
	calls []string
}

func (m *mockMonsterDefeatRecorder) RecordMonsterDefeat(ctx context.Context, characterID, monsterID, monsterName, habitat string) error {
	m.calls = append(m.calls, characterID+":"+monsterID+":"+monsterName+":"+habitat)
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
	expectedMsg := "なんと スライム が起き上がりこちらを見ている。スライムはうれしそうにアレックスのモンスター預かり所に向かった"
	if res.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
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
	expectedMsg := "なんと スライム が起き上がりこちらを見ている。しかし、アレックスのモンスター預かり所はいっぱいだった。スライムは悲しそうに去っていった…"
	if res.Message != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
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
