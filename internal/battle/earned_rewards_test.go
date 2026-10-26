package battle_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
)

func TestApplyPostBattleResult_PreserveEarnedRewards(t *testing.T) {
	for _, count := range []int{1, 2} {
		for _, preserve := range []bool{false, true} {
			t.Run(fmt.Sprintf("members=%d/preserve=%v", count, preserve), func(t *testing.T) {
				ctx := context.Background()
				svc, chars, invs := setupRewardTestService("", 1)
				runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(&mockTxProvider{}))
				if err != nil {
					t.Fatal(err)
				}
				battle.WithTransactionRunner(runner)(svc)
				req := battle.ApplyPostBattleRequest{
					PreserveEarnedRewards: preserve,
					BattleResult: corebattle.PartyBattleResult{
						Outcome:     corebattle.OutcomeDefeat,
						TotalReward: corebattle.Reward{Experience: 7, Currency: 11, Crystals: 3, ItemDefinitionID: "unearned-drop", ItemQuantity: 1},
					},
					DropItems: []string{"unearned-extra"},
				}
				for i := 0; i < count; i++ {
					c := corecharacter.Character{ID: fmt.Sprintf("earned-%d", i), Name: "Hero", Level: 10, JobLevel: 1, Money: 100, Crystal: 5, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
					if err := chars.Update(ctx, c); err != nil {
						t.Fatal(err)
					}
					inv, err := coreinventory.New(c.ID)
					if err != nil {
						t.Fatal(err)
					}
					if err := invs.Save(ctx, inv); err != nil {
						t.Fatal(err)
					}
					req.CharacterIDs = append(req.CharacterIDs, c.ID)
				}
				resp, err := svc.ApplyPostBattleResult(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range req.CharacterIDs {
					wantEXP, wantGold, wantCrystals := 0, 0, 0
					if preserve {
						wantEXP, wantGold, wantCrystals = 7, 11, 3
					}
					c := resp.UpdatedCharacters[id]
					if c.Experience != wantEXP || c.Money != 100+wantGold || c.Crystal != 5+wantCrystals || resp.GainedExperience[id] != wantEXP || resp.GainedGold[id] != wantGold || resp.GainedCrystals[id] != wantCrystals {
						t.Fatalf("incorrect earned assets for %s: character=%+v response=%+v", id, c, resp)
					}
					if len(resp.InventoryDrops[id])+len(resp.DepotDeliveries[id]) != 0 {
						t.Fatal("defeat granted victory drops")
					}
				}
			})
		}
	}
}
