package battle_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/collection"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/testutil"
)

type battleMonsterDefeatAdapter struct {
	svc *collection.Service
}

func (a battleMonsterDefeatAdapter) RecordMonsterDefeat(ctx context.Context, characterID string, record battle.DefeatedMonsterRecord) error {
	if a.svc == nil {
		return nil
	}
	return a.svc.RecordMonsterDefeat(ctx, characterID, collection.DefeatedMonsterRecord{
		MonsterID:        record.MonsterID,
		MonsterName:      record.MonsterName,
		Habitat:          record.Habitat,
		Icon:             record.Icon,
		Strong:           record.Strong,
		HP:               record.HP,
		MP:               record.MP,
		Attack:           record.Attack,
		Defense:          record.Defense,
		Agility:          record.Agility,
		ExperienceReward: record.ExperienceReward,
		GoldReward:       record.GoldReward,
	})
}

type switchableLegendInductor struct {
	mu          sync.Mutex
	fail        bool
	err         error
	rankingRepo *database.RankingRepository
	calls       int
}

func (s *switchableLegendInductor) RecordLegend(ctx context.Context, category, characterID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.fail {
		if s.err != nil {
			return s.err
		}
		return errors.New("simulated lower-level legend induction failure")
	}
	if s.rankingRepo != nil {
		_, err := s.rankingRepo.RecordLegend(ctx, ranking.LegendEntry{
			Category:    ranking.LegendCategory(category),
			CharacterID: characterID,
		})
		return err
	}
	return nil
}

func setupBattleCollectionIntegration(
	t *testing.T,
	db *sql.DB,
	inductor collection.LegendInductor,
	totalMonsters int,
) (*battle.Service, *collection.Service) {
	t.Helper()

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatalf("NewCharacterRepository failed: %v", err)
	}
	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatalf("NewInventoryRepository failed: %v", err)
	}
	equipRepo, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatalf("NewEquipmentRepository failed: %v", err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatalf("NewDepotRepository failed: %v", err)
	}
	collectionRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		t.Fatalf("NewCollectionRepository failed: %v", err)
	}
	txProv := database.NewTransactionProvider(db)
	txRunner, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
	if err != nil {
		t.Fatalf("NewEconomyService failed: %v", err)
	}

	colOpts := []collection.Option{
		collection.WithCharacterRepository(charRepo),
	}
	if inductor != nil {
		colOpts = append(colOpts, collection.WithLegendInductor(inductor))
	}
	colSvc, err := collection.NewService(collectionRepo, totalMonsters, 100, colOpts...)
	if err != nil {
		t.Fatalf("NewCollectionService failed: %v", err)
	}

	battleSvc := battle.NewService(
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithTransactionRunner(txRunner),
		battle.WithMonsterDefeatRecorder(battleMonsterDefeatAdapter{svc: colSvc}),
	)

	return battleSvc, colSvc
}

// TestBattleCollectionSettlement_SingleCharacter_RealDB_FailureAndRecovery verifies:
//  1. When legend induction fails in a single-character battle, rewards (HP/MP/Gold/EXP/drop) and
//     collection/legend state are rolled back atomically (no partial commit).
//  2. Subsequent retry (reprocessing) successfully commits rewards, monster book, completion, and legend records exactly once.
//  3. Normal subsequent victories do not duplicate rewards, completions, or legend inductions.
func TestBattleCollectionSettlement_SingleCharacter_RealDB_FailureAndRecovery(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment failed: %v", err)
	}
	defer db.Close()

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatalf("NewRankingRepository failed: %v", err)
	}

	inductor := &switchableLegendInductor{
		fail:        true,
		rankingRepo: rankingRepo,
	}

	battleSvc, _ := setupBattleCollectionIntegration(t, db, inductor, 1)

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "SingleColChar", 1000)
	if err != nil {
		t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
	}
	initialMoney := char.Money
	initialExp := char.Experience

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{char.ID: 95},
			TotalReward: corebattle.Reward{
				Currency:   300,
				Experience: 100,
			},
		},
		RecipientDrops: map[string][]string{
			char.ID: {"item-001"},
		},
		DefeatedEnemies: []corebattle.Participant{
			{
				ID:               "mon_single_boss",
				Name:             "Single Boss",
				HP:               50,
				MaxHP:            50,
				MP:               20,
				MaxMP:            20,
				Attack:           30,
				Defense:          20,
				Agility:          15,
				ExperienceReward: 100,
				GoldReward:       300,
			},
		},
		Habitat: "平原",
	}

	// 1. Initial attempt fails due to lower-level legend induction error
	_, err = battleSvc.ApplyPostBattleResult(ctx, req)
	if err == nil {
		t.Fatal("expected ApplyPostBattleResult to fail on legend induction failure, but succeeded")
	}

	// Verify MariaDB atomic rollback:
	var currentMoney, currentExp, currentHP int
	err = db.QueryRowContext(ctx, "SELECT money, experience, hp FROM characters WHERE id = ?", char.ID).
		Scan(&currentMoney, &currentExp, &currentHP)
	if err != nil {
		t.Fatalf("failed to query character: %v", err)
	}
	if currentMoney != initialMoney {
		t.Errorf("expected money to remain %d, got %d", initialMoney, currentMoney)
	}
	if currentExp != initialExp {
		t.Errorf("expected experience to remain %d, got %d", initialExp, currentExp)
	}
	if currentHP != char.Stats.HP {
		t.Errorf("expected HP to remain %d, got %d", char.Stats.HP, currentHP)
	}

	var dropCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = 'item-001'", char.ID).
		Scan(&dropCount)
	if err != nil {
		t.Fatalf("failed to query inventory: %v", err)
	}
	if dropCount != 0 {
		t.Errorf("expected 0 dropped items in inventory, got %d", dropCount)
	}

	var bookRows, defeatedCountSum int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(defeated_count), 0) FROM character_monster_book WHERE character_id = ?", char.ID).
		Scan(&bookRows, &defeatedCountSum)
	if err != nil {
		t.Fatalf("failed to query monster book: %v", err)
	}
	if bookRows != 0 || defeatedCountSum != 0 {
		t.Errorf("expected 0 monster book rows/counts, got rows=%d sum=%d", bookRows, defeatedCountSum)
	}

	var compCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ? AND kind = 'monster_book'", char.ID).
		Scan(&compCount)
	if err != nil {
		t.Fatalf("failed to query completions: %v", err)
	}
	if compCount != 0 {
		t.Errorf("expected 0 completions, got %d", compCount)
	}

	var legendCount int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_mon'", char.ID).
		Scan(&legendCount)
	if err != nil {
		t.Fatalf("failed to query legends: %v", err)
	}
	if legendCount != 0 {
		t.Errorf("expected 0 legends, got %d", legendCount)
	}

	// 2. Retry / Reprocessing after transient legend failure is resolved
	inductor.mu.Lock()
	inductor.fail = false
	inductor.mu.Unlock()

	resp, err := battleSvc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("reprocessing ApplyPostBattleResult failed: %v", err)
	}
	if len(resp.InventoryDrops[char.ID]) != 1 {
		t.Errorf("expected 1 inventory drop in response, got %d", len(resp.InventoryDrops[char.ID]))
	}

	// Verify MariaDB state committed exactly once:
	err = db.QueryRowContext(ctx, "SELECT money, experience, hp FROM characters WHERE id = ?", char.ID).
		Scan(&currentMoney, &currentExp, &currentHP)
	if err != nil {
		t.Fatalf("failed to query character: %v", err)
	}
	if currentMoney != initialMoney+300 {
		t.Errorf("expected money to be %d, got %d", initialMoney+300, currentMoney)
	}
	if currentExp != initialExp+100 {
		t.Errorf("expected experience to be %d, got %d", initialExp+100, currentExp)
	}
	if currentHP != resp.UpdatedCharacters[char.ID].Stats.HP {
		t.Errorf("expected HP to be %d, got %d", resp.UpdatedCharacters[char.ID].Stats.HP, currentHP)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = 'item-001'", char.ID).
		Scan(&dropCount)
	if err != nil {
		t.Fatalf("failed to query inventory: %v", err)
	}
	if dropCount != 1 {
		t.Errorf("expected 1 dropped item in inventory, got %d", dropCount)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(defeated_count), 0) FROM character_monster_book WHERE character_id = ?", char.ID).
		Scan(&bookRows, &defeatedCountSum)
	if err != nil {
		t.Fatalf("failed to query monster book: %v", err)
	}
	if bookRows != 1 || defeatedCountSum != 1 {
		t.Errorf("expected 1 monster book row with defeated_count=1, got rows=%d sum=%d", bookRows, defeatedCountSum)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ? AND kind = 'monster_book'", char.ID).
		Scan(&compCount)
	if err != nil {
		t.Fatalf("failed to query completions: %v", err)
	}
	if compCount != 1 {
		t.Errorf("expected 1 completion, got %d", compCount)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_mon'", char.ID).
		Scan(&legendCount)
	if err != nil {
		t.Fatalf("failed to query legends: %v", err)
	}
	if legendCount != 1 {
		t.Errorf("expected 1 legend, got %d", legendCount)
	}

	// 3. Normal next battle: defeats the same monster again
	req2 := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{char.ID: 90},
			TotalReward: corebattle.Reward{
				Currency:   50,
				Experience: 20,
			},
		},
		DefeatedEnemies: req.DefeatedEnemies,
		Habitat:         "平原",
	}

	_, err = battleSvc.ApplyPostBattleResult(ctx, req2)
	if err != nil {
		t.Fatalf("second battle ApplyPostBattleResult failed: %v", err)
	}

	// Verify that monster book count incremented to 2, but completions and legend records remain exactly 1
	err = db.QueryRowContext(ctx, "SELECT money, experience FROM characters WHERE id = ?", char.ID).
		Scan(&currentMoney, &currentExp)
	if err != nil {
		t.Fatalf("failed to query character: %v", err)
	}
	if currentMoney != initialMoney+300+50 {
		t.Errorf("expected money to be %d, got %d", initialMoney+350, currentMoney)
	}
	if currentExp != initialExp+100+20 {
		t.Errorf("expected experience to be %d, got %d", initialExp+120, currentExp)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(defeated_count), 0) FROM character_monster_book WHERE character_id = ?", char.ID).
		Scan(&bookRows, &defeatedCountSum)
	if err != nil {
		t.Fatalf("failed to query monster book: %v", err)
	}
	if bookRows != 1 || defeatedCountSum != 2 {
		t.Errorf("expected 1 monster book row with defeated_count=2, got rows=%d sum=%d", bookRows, defeatedCountSum)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ? AND kind = 'monster_book'", char.ID).
		Scan(&compCount)
	if err != nil {
		t.Fatalf("failed to query completions: %v", err)
	}
	if compCount != 1 {
		t.Errorf("expected completion count to remain 1, got %d", compCount)
	}

	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_mon'", char.ID).
		Scan(&legendCount)
	if err != nil {
		t.Fatalf("failed to query legends: %v", err)
	}
	if legendCount != 1 {
		t.Errorf("expected legend count to remain 1, got %d", legendCount)
	}
}

// TestBattleCollectionSettlement_MultiCharacter_RealDB_FailureAndRecovery verifies:
//  1. In multi-character party battles with TransactionProvider, lower-level legend induction failure
//     rolls back all party members' rewards (Gold, EXP, HP, drops) and monster collection state.
//  2. Reprocessing retry commits rewards and milestone records for all members exactly once.
func TestBattleCollectionSettlement_MultiCharacter_RealDB_FailureAndRecovery(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment failed: %v", err)
	}
	defer db.Close()

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatalf("NewRankingRepository failed: %v", err)
	}

	inductor := &switchableLegendInductor{
		fail:        true,
		rankingRepo: rankingRepo,
	}

	battleSvc, _ := setupBattleCollectionIntegration(t, db, inductor, 1)

	char1, err := database.CreateTestCharacterWithFunds(ctx, db, "PartyChar1", 500)
	if err != nil {
		t.Fatalf("CreateTestCharacter 1 failed: %v", err)
	}
	char2, err := database.CreateTestCharacterWithFunds(ctx, db, "PartyChar2", 600)
	if err != nil {
		t.Fatalf("CreateTestCharacter 2 failed: %v", err)
	}

	req := battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char1.ID, char2.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeWin,
			RemainingHP: map[string]int{
				char1.ID: 80,
				char2.ID: 75,
			},
			TotalReward: corebattle.Reward{
				Currency:   200,
				Experience: 80,
			},
		},
		DefeatedEnemies: []corebattle.Participant{
			{
				ID:               "mon_party_boss",
				Name:             "Party Boss",
				HP:               100,
				MaxHP:            100,
				MP:               30,
				MaxMP:            30,
				Attack:           40,
				Defense:          30,
				Agility:          20,
				ExperienceReward: 80,
				GoldReward:       200,
			},
		},
		Habitat: "洞窟",
	}

	// 1. Initial attempt fails due to legend failure
	_, err = battleSvc.ApplyPostBattleResult(ctx, req)
	if err == nil {
		t.Fatal("expected multi-character ApplyPostBattleResult to fail, but succeeded")
	}

	// Verify both characters have zero changes in MariaDB
	for _, cID := range []string{char1.ID, char2.ID} {
		var money, exp int
		err = db.QueryRowContext(ctx, "SELECT money, experience FROM characters WHERE id = ?", cID).
			Scan(&money, &exp)
		if err != nil {
			t.Fatalf("failed to query character %s: %v", cID, err)
		}
		expectedMoney := 500
		if cID == char2.ID {
			expectedMoney = 600
		}
		if money != expectedMoney || exp != 0 {
			t.Errorf("character %s expected money=%d exp=0, got money=%d exp=%d", cID, expectedMoney, money, exp)
		}

		var bookRows int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_monster_book WHERE character_id = ?", cID).
			Scan(&bookRows)
		if err != nil {
			t.Fatalf("failed to query monster book: %v", err)
		}
		if bookRows != 0 {
			t.Errorf("character %s expected 0 monster book rows, got %d", cID, bookRows)
		}

		var compRows int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ? AND kind = 'monster_book'", cID).
			Scan(&compRows)
		if err != nil {
			t.Fatalf("failed to query completions: %v", err)
		}
		if compRows != 0 {
			t.Errorf("character %s expected 0 completions, got %d", cID, compRows)
		}

		var legendRows int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_mon'", cID).
			Scan(&legendRows)
		if err != nil {
			t.Fatalf("failed to query legends: %v", err)
		}
		if legendRows != 0 {
			t.Errorf("character %s expected 0 legend records, got %d", cID, legendRows)
		}
	}

	// 2. Retry / Reprocessing succeeds
	inductor.mu.Lock()
	inductor.fail = false
	inductor.mu.Unlock()

	_, err = battleSvc.ApplyPostBattleResult(ctx, req)
	if err != nil {
		t.Fatalf("multi-character retry failed: %v", err)
	}

	// Verify both characters have rewards and completion records committed
	for _, cID := range []string{char1.ID, char2.ID} {
		var money, exp int
		err = db.QueryRowContext(ctx, "SELECT money, experience FROM characters WHERE id = ?", cID).
			Scan(&money, &exp)
		if err != nil {
			t.Fatalf("failed to query character %s: %v", cID, err)
		}
		expectedMoney := 700
		if cID == char2.ID {
			expectedMoney = 800
		}
		if money != expectedMoney || exp != 80 {
			t.Errorf("character %s expected money=%d exp=80, got money=%d exp=%d", cID, expectedMoney, money, exp)
		}

		var bookRows, defSum int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(defeated_count), 0) FROM character_monster_book WHERE character_id = ?", cID).
			Scan(&bookRows, &defSum)
		if err != nil {
			t.Fatalf("failed to query monster book: %v", err)
		}
		if bookRows != 1 || defSum != 1 {
			t.Errorf("character %s expected 1 monster book row with count=1, got rows=%d count=%d", cID, bookRows, defSum)
		}

		var compRows int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ? AND kind = 'monster_book'", cID).
			Scan(&compRows)
		if err != nil {
			t.Fatalf("failed to query completions: %v", err)
		}
		if compRows != 1 {
			t.Errorf("character %s expected 1 completion, got %d", cID, compRows)
		}

		var legendRows int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_mon'", cID).
			Scan(&legendRows)
		if err != nil {
			t.Fatalf("failed to query legends: %v", err)
		}
		if legendRows != 1 {
			t.Errorf("character %s expected 1 legend record, got %d", cID, legendRows)
		}
	}
}

// TestBattleCollectionSettlement_Concurrency_NoDeadlock verifies that concurrent battles
// completing monster collection records do not encounter deadlocks and conserve assets.
func TestBattleCollectionSettlement_Concurrency_NoDeadlock(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatalf("OpenFromEnvironment failed: %v", err)
	}
	defer db.Close()

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatalf("NewRankingRepository failed: %v", err)
	}

	inductor := &switchableLegendInductor{
		fail:        false,
		rankingRepo: rankingRepo,
	}

	battleSvc, _ := setupBattleCollectionIntegration(t, db, inductor, 2)

	// Create 4 test characters
	chars := make([]corecharacter.Character, 4)
	for i := 0; i < 4; i++ {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("ConcChar%d", i), 1000)
		if err != nil {
			t.Fatalf("CreateTestCharacter %d failed: %v", i, err)
		}
		chars[i] = c
	}

	// Run concurrent battles: 2 characters each in separate parties
	res := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{
		Workers:      4,
		OpsPerWorker: 2,
	}, func(workerID int, op int) error {
		pIdx := workerID % 2
		pChars := []string{chars[pIdx*2].ID, chars[pIdx*2+1].ID}

		monID := fmt.Sprintf("conc_mon_%d", op)
		req := battle.ApplyPostBattleRequest{
			CharacterIDs: pChars,
			BattleResult: corebattle.PartyBattleResult{
				Outcome: corebattle.OutcomeWin,
				RemainingHP: map[string]int{
					pChars[0]: 90,
					pChars[1]: 85,
				},
				TotalReward: corebattle.Reward{
					Currency:   10,
					Experience: 5,
				},
			},
			DefeatedEnemies: []corebattle.Participant{
				{
					ID:               monID,
					Name:             "Conc Monster " + monID,
					HP:               20,
					MaxHP:            20,
					Attack:           10,
					Defense:          10,
					Agility:          10,
					ExperienceReward: 5,
					GoldReward:       10,
				},
			},
			Habitat: "山脈",
		}

		_, err := battleSvc.ApplyPostBattleResult(ctx, req)
		return err
	})

	if res.Deadlocks > 0 {
		t.Fatalf("encountered %d deadlocks during concurrent battle settlement", res.Deadlocks)
	}

	// Verify all characters completed monster book exactly once
	for _, c := range chars {
		var compCount int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_collection_completions WHERE character_id = ? AND kind = 'monster_book'", c.ID).
			Scan(&compCount)
		if err != nil {
			t.Fatalf("failed to query completions for %s: %v", c.ID, err)
		}
		if compCount != 1 {
			t.Errorf("character %s expected exactly 1 completion record, got %d", c.ID, compCount)
		}

		var legendCount int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM legend_records WHERE character_id = ? AND category = 'comp_mon'", c.ID).
			Scan(&legendCount)
		if err != nil {
			t.Fatalf("failed to query legend for %s: %v", c.ID, err)
		}
		if legendCount != 1 {
			t.Errorf("character %s expected exactly 1 legend record, got %d", c.ID, legendCount)
		}
	}
}
