package battle_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/monster"
)

type realDBMonsterTamerAdapter struct {
	svc *monster.Service
}

func (a realDBMonsterTamerAdapter) TameMonster(ctx context.Context, characterID, monsterID, customName string) error {
	_, err := a.svc.TameMonster(ctx, characterID, monsterID, customName)
	if errors.Is(err, monster.ErrBoxFull) {
		return battle.ErrMonsterBoxFull
	}
	return err
}

type failingSaveMonsterRepo struct {
	monster.MonsterRepository
	failOnSave int
	saveCalls  int
	err        error
}

func (r *failingSaveMonsterRepo) Save(ctx context.Context, inst monster.MonsterInstance) error {
	r.saveCalls++
	if r.failOnSave > 0 && r.saveCalls >= r.failOnSave {
		return r.err
	}
	return r.MonsterRepository.Save(ctx, inst)
}

func setupTamingIntegrationService(
	t *testing.T,
	db *sql.DB,
	monRepo monster.MonsterRepository,
	blessingProv battle.BlessingProvider,
	useRunner bool,
) *battle.Service {
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
	txProv := database.NewTransactionProvider(db)

	monSvc := monster.NewService(charRepo, monRepo, monster.WithTransactionProvider(txProv))
	opts := []battle.Option{
		battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(invRepo),
		battle.WithEquipmentRepository(equipRepo),
		battle.WithDepotRepository(depotRepo),
		battle.WithTransactionProvider(txProv),
		battle.WithRandomSource(fixedRNG{val: 0}),
		battle.WithMonsterTamer(realDBMonsterTamerAdapter{svc: monSvc}),
	}
	if blessingProv != nil {
		opts = append(opts, battle.WithBlessingProvider(blessingProv))
	}
	if useRunner {
		runner, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))
		if err != nil {
			t.Fatalf("economy.NewService failed: %v", err)
		}
		opts = append(opts, battle.WithTransactionRunner(runner))
	}
	return battle.NewService(opts...)
}

func TestPostBattleMonsterTaming_RealDB_RollbackAndRetry(t *testing.T) {
	ctx := context.Background()
	db := database.OpenTestDB(t)
	storageErr := errors.New("simulated character_monsters write failure")

	for _, route := range []struct {
		name      string
		useRunner bool
		partySize int
	}{
		{name: "single_runner", useRunner: true, partySize: 1},
		{name: "multi_provider", useRunner: false, partySize: 2},
	} {
		t.Run(route.name, func(t *testing.T) {
			baseMonRepo, err := database.NewMonsterRepository(db)
			if err != nil {
				t.Fatalf("NewMonsterRepository failed: %v", err)
			}
			// Succeed on 1st monster save inside the tx, fail on 2nd monster save.
			monRepo := &failingSaveMonsterRepo{
				MonsterRepository: baseMonRepo,
				failOnSave:        2,
				err:               storageErr,
			}
			svc := setupTamingIntegrationService(t, db, monRepo, nil, route.useRunner)

			var chars []corecharacter.Character
			var ids []string
			remHP := make(map[string]int, route.partySize)
			for i := 0; i < route.partySize; i++ {
				c, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("TameRB%s%d", route.name, i), 1000)
				if err != nil {
					t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
				}
				chars = append(chars, c)
				ids = append(ids, c.ID)
				remHP[c.ID] = 85
			}
			recipientID := ids[0]

			req := battle.ApplyPostBattleRequest{
				CharacterIDs:         ids,
				RecipientCharacterID: recipientID,
				BattleResult: corebattle.PartyBattleResult{
					Outcome:     corebattle.OutcomeWin,
					RemainingHP: remHP,
					TotalReward: corebattle.Reward{
						Currency:   300,
						Experience: 100,
					},
				},
				RecipientDrops: map[string][]string{
					recipientID: {"item-001"},
				},
				DefeatedEnemies: []corebattle.Participant{
					{ID: "monster-002-f1-1", Name: "スライムA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
					{ID: "monster-003-f1-2", Name: "ゴブリンA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
				},
			}

			_, err = svc.ApplyPostBattleResult(ctx, req)
			if !errors.Is(err, storageErr) {
				t.Fatalf("ApplyPostBattleResult error = %v, want %v", err, storageErr)
			}

			for _, orig := range chars {
				var money, exp, hp int
				if err := db.QueryRowContext(ctx, "SELECT money, experience, hp FROM characters WHERE id = ?", orig.ID).Scan(&money, &exp, &hp); err != nil {
					t.Fatalf("query character %s: %v", orig.ID, err)
				}
				if money != orig.Money || exp != orig.Experience || hp != orig.Stats.HP {
					t.Fatalf("character %s not rolled back: got money=%d exp=%d hp=%d, want money=%d exp=%d hp=%d",
						orig.ID, money, exp, hp, orig.Money, orig.Experience, orig.Stats.HP)
				}
			}

			var dropCount int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM inventory_items WHERE character_id = ? AND definition_id = 'item-001'", recipientID).Scan(&dropCount); err != nil {
				t.Fatalf("query inventory: %v", err)
			}
			if dropCount != 0 {
				t.Fatalf("expected 0 dropped items after rollback, got %d", dropCount)
			}

			var monCount int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_monsters WHERE character_id = ?", recipientID).Scan(&monCount); err != nil {
				t.Fatalf("query character_monsters: %v", err)
			}
			if monCount != 0 {
				t.Fatalf("expected 0 tamed monsters after rollback (first tame must roll back too), got %d", monCount)
			}

			// Clear fault and retry: rewards and both tames commit cleanly.
			monRepo.failOnSave = 0
			monRepo.saveCalls = 0
			resp, err := svc.ApplyPostBattleResult(ctx, req)
			if err != nil {
				t.Fatalf("retry ApplyPostBattleResult failed: %v", err)
			}
			if len(resp.MonsterTames[recipientID]) != 2 {
				t.Fatalf("expected 2 tames on retry, got %+v", resp.MonsterTames[recipientID])
			}
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_monsters WHERE character_id = ?", recipientID).Scan(&monCount); err != nil {
				t.Fatalf("query character_monsters after retry: %v", err)
			}
			if monCount != 2 {
				t.Fatalf("expected 2 tamed monsters after retry, got %d", monCount)
			}
		})
	}
}

func TestPostBattleMonsterTaming_RealDB_BoxFullAndBlessingError(t *testing.T) {
	ctx := context.Background()
	db := database.OpenTestDB(t)

	monRepo, err := database.NewMonsterRepository(db)
	if err != nil {
		t.Fatalf("NewMonsterRepository failed: %v", err)
	}
	blessingProv := &mockBlessingProvider{}
	svc := setupTamingIntegrationService(t, db, monRepo, blessingProv, true)

	char, err := database.CreateTestCharacterWithFunds(ctx, db, "BoxFullChar", 500)
	if err != nil {
		t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
	}

	for i := 0; i < monster.BaseBoxCapacity; i++ {
		if err := monRepo.Save(ctx, monster.MonsterInstance{
			CharacterID: char.ID,
			MonsterID:   "monster-001",
			CustomName:  fmt.Sprintf("Pet%d", i),
			Location:    monster.LocationBox,
		}); err != nil {
			t.Fatalf("seed monster %d: %v", i, err)
		}
	}

	// 1. Blessing read failure rolls back settlement without committing rewards.
	targetHP := char.Stats.HP - 5
	if targetHP < 1 {
		targetHP = 1
	}
	blessingErr := errors.New("chapel DB unavailable")
	blessingProv.err = blessingErr
	_, err = svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{char.ID: targetHP},
			TotalReward: corebattle.Reward{Currency: 200},
		},
		DefeatedEnemies: []corebattle.Participant{
			{ID: "monster-002-f1-1", Name: "スライムA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
		},
	})
	if !errors.Is(err, blessingErr) {
		t.Fatalf("expected blessing error, got %v", err)
	}
	var money, hp int
	if err := db.QueryRowContext(ctx, "SELECT money, hp FROM characters WHERE id = ?", char.ID).Scan(&money, &hp); err != nil {
		t.Fatalf("query character: %v", err)
	}
	if money != char.Money || hp != char.Stats.HP {
		t.Fatalf("character mutated on blessing error: money=%d hp=%d", money, hp)
	}

	// 2. Clear blessing failure; full monster box returns BoxFull outcome while committing rewards.
	blessingProv.err = nil
	resp, err := svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{char.ID: targetHP},
			TotalReward: corebattle.Reward{Currency: 200},
		},
		DefeatedEnemies: []corebattle.Participant{
			{ID: "monster-002-f1-1", Name: "スライムA", MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error on box full, got %v", err)
	}
	tames := resp.MonsterTames[char.ID]
	if len(tames) != 1 || tames[0].Success || !tames[0].BoxFull {
		t.Fatalf("expected single BoxFull result, got %+v", tames)
	}
	if err := db.QueryRowContext(ctx, "SELECT money, hp FROM characters WHERE id = ?", char.ID).Scan(&money, &hp); err != nil {
		t.Fatalf("query character after box full: %v", err)
	}
	if money != char.Money+200 || hp != targetHP {
		t.Fatalf("expected committed rewards after BoxFull: money=%d hp=%d (want money=%d hp=%d)", money, hp, char.Money+200, targetHP)
	}
}
