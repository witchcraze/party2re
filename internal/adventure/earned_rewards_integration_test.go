package adventure_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/testutil"
)

func earnedRewardStorage(t *testing.T) (*sql.DB, *database.CharacterRepository, *database.InventoryRepository, *database.AdventureRepository, *battle.Service) {
	t.Helper()
	db := database.OpenTestDB(t)
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	advs, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	settler := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs), battle.WithTransactionProvider(tx), battle.WithTransactionRunner(runner))
	return db, chars, invs, advs, settler
}

func TestAdventure_EarnedRewardSettlementConcurrentIntegration(t *testing.T) {
	db, chars, _, _, settler := earnedRewardStorage(t)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 2; i++ {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, "EarnRace", 100)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		ordered := ids
		if worker%2 == 1 {
			ordered = []string{ids[1], ids[0]}
		}
		_, err := settler.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{CharacterIDs: ordered, PreserveEarnedRewards: true,
			RecipientRewards: map[string]corebattle.Reward{ids[0]: {Experience: 1, Currency: 2, Crystals: 1}, ids[1]: {Experience: 2, Currency: 3, Crystals: 2}},
			BattleResult:     corebattle.PartyBattleResult{Outcome: corebattle.OutcomeDefeat, TotalReward: corebattle.Reward{Experience: 1000, Currency: 1000, Crystals: 1000}},
		})
		return err
	})
	if res.Failures != 0 || res.Successes != res.TotalOps {
		t.Fatalf("settlement failures: %+v", res)
	}
	for i, id := range ids {
		c, err := chars.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if c.Experience != int(res.Successes)*(i+1) || c.Money != 100+int(res.Successes)*(i+2) || c.Crystal != int(res.Successes)*(i+1) {
			t.Fatalf("assets not conserved: %+v, operations=%+v", c, res)
		}
	}
}
