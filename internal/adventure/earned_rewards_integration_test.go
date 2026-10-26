package adventure_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/testutil"
)

type earnedRewardEngine struct {
	corebattle.Engine
	wins, calls int
}

func (e *earnedRewardEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls++
	if e.calls > e.wins {
		return defeatBattleResolver{}.ResolvePartyBattle(req)
	}
	return crystalRewardBattleResolver{crystalsPerFloor: 3}.ResolvePartyBattle(req)
}

type failingEarnedSettler struct {
	adventure.PostBattleSettler
	err error
}

func (s failingEarnedSettler) ApplyPostBattleResult(ctx context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	resp, err := s.PostBattleSettler.ApplyPostBattleResult(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, s.err
}

func earnedRewardStorage(t *testing.T) (*sql.DB, *database.CharacterRepository, *database.InventoryRepository, *database.AdventureRepository, *battle.Service) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
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

func TestAdventure_EarnedRewardsAfterDefeatIntegration(t *testing.T) {
	db, chars, invs, advs, settler := earnedRewardStorage(t)
	ctx := context.Background()
	for _, count := range []int{1, 2} {
		for _, fallback := range []bool{false, true} {
			for _, tc := range []struct {
				mode string
				wins int
			}{
				{adventure.CrawlModeNormal, 0},
				{adventure.CrawlModeNormal, 1},
				{adventure.CrawlModeTry, 1},
			} {
				wins := tc.wins
				t.Run(fmt.Sprintf("members=%d/fallback=%v/mode=%s/wins=%d", count, fallback, tc.mode, wins), func(t *testing.T) {
					var originals []corecharacter.Character
					var ids []string
					for i := 0; i < count; i++ {
						c, err := database.CreateTestCharacterWithFunds(ctx, db, "Earned", 100)
						if err != nil {
							t.Fatal(err)
						}
						originals = append(originals, c)
						ids = append(ids, c.ID)
					}
					engine := &earnedRewardEngine{wins: wins}
					injected := errors.New("reward settlement failed after writes")
					opts := []adventure.Option{adventure.WithTransactionProvider(database.NewTransactionProvider(db))}
					if !fallback {
						opts = append(opts, adventure.WithPostBattleSettler(failingEarnedSettler{PostBattleSettler: settler, err: injected}))
					}
					svc, err := adventure.NewService(advs, chars, engine, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					for _, opt := range opts {
						opt(svc)
					}
					request := adventure.DungeonCrawlRequest{CharacterIDs: ids, StageID: adventure.StarterAdventure, Mode: tc.mode, Rng: func(int) int { return 0 }}
					if !fallback {
						if _, err := svc.ExecuteCrawl(ctx, request); !errors.Is(err, injected) {
							t.Fatalf("expected reward error, got %v", err)
						}
						for _, before := range originals {
							after, err := chars.FindByID(ctx, before.ID)
							if err != nil {
								t.Fatal(err)
							}
							if after.Money != before.Money || after.Experience != before.Experience || after.Crystal != before.Crystal || after.Stats.HP != before.Stats.HP || after.Tired != before.Tired {
								t.Fatalf("failed reward escaped rollback: before=%+v after=%+v", before, after)
							}
							history, total, err := advs.ListByCharacterID(ctx, before.ID, 10, 0)
							if err != nil {
								t.Fatal(err)
							}
							if total != 0 || len(history) != 0 {
								t.Fatal("failed settlement wrote history")
							}
						}
						adventure.WithPostBattleSettler(settler)(svc)
						engine.calls = 0
					}
					result, err := svc.ExecuteCrawl(ctx, request)
					if err != nil {
						t.Fatal(err)
					}
					if result.Outcome != corebattle.OutcomeDefeat || result.FloorsCleared != wins || result.TotalEXP != wins*10 || result.TotalGold != wins*20 || result.TotalCrystals != wins*3 {
						t.Fatalf("lost earned rewards: %+v", result)
					}
					creditedWins := wins
					if tc.mode == adventure.CrawlModeTry {
						creditedWins = 0
					}
					for _, before := range originals {
						after, err := chars.FindByID(ctx, before.ID)
						if err != nil {
							t.Fatal(err)
						}
						if after.Money != before.Money+creditedWins*20 || after.Experience != before.Experience+creditedWins*10 || after.Crystal != before.Crystal+creditedWins*3 || after.Stats.HP != 1 {
							t.Fatalf("incorrect persisted earnings: before=%+v after=%+v", before, after)
						}
						history, err := advs.FindByID(ctx, result.AdventureIDs[before.ID])
						if err != nil {
							t.Fatal(err)
						}
						if history.BattleResult.Outcome != corebattle.OutcomeDefeat || history.ExperienceReward != wins*10 {
							t.Fatalf("incorrect defeat history: %+v", history)
						}
						inv, err := invs.FindByCharacterID(ctx, before.ID)
						if err != nil {
							t.Fatal(err)
						}
						if len(inv.Items) != 0 {
							t.Fatal("defeat granted treasure")
						}
					}
				})
			}
		}
	}
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
		_, err := settler.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{CharacterIDs: ordered, PreserveEarnedRewards: true, BattleResult: corebattle.PartyBattleResult{Outcome: corebattle.OutcomeDefeat, TotalReward: corebattle.Reward{Experience: 1, Currency: 2, Crystals: 1}}})
		return err
	})
	if res.Failures != 0 || res.Successes != res.TotalOps {
		t.Fatalf("settlement failures: %+v", res)
	}
	for _, id := range ids {
		c, err := chars.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if c.Experience != int(res.Successes) || c.Money != 100+int(res.Successes)*2 || c.Crystal != int(res.Successes) {
			t.Fatalf("assets not conserved: %+v, operations=%+v", c, res)
		}
	}
}
