package party

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestPartyAdventure_PreserveEarnedRewardsAfterDefeat(t *testing.T) {
	for _, adapter := range []bool{false, true} {
		t.Run(fmt.Sprintf("adapter=%v", adapter), func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
				calls++
				outcome := corebattle.OutcomeWin
				if calls == 2 {
					outcome = corebattle.OutcomeDefeat
				}
				hps := make(map[string]int)
				for _, p := range req.Allies {
					hps[p.ID] = p.HP
					if calls == 2 {
						hps[p.ID] = 0
					}
				}
				return corebattle.PartyBattleResult{Outcome: outcome, RemainingHP: hps, TotalReward: corebattle.Reward{Experience: 7, Currency: 11, Crystals: 3}}, nil
			})
			svc, parties, chars, invs, _ := setupPartyAdventureTestService(t, engine)
			if adapter {
				svc.battleSettler = battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs))
			}
			leader := corecharacter.Character{ID: "earned-leader", Name: "Leader", Level: 10, JobLevel: 1, Money: 100, Crystal: 5, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
			member := leader
			member.ID, member.Name = "earned-member", "Member"
			partyID := createReadyParty(ctx, t, svc, parties, chars, leader, member)
			result, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != string(corebattle.OutcomeDefeat) || result.TotalEXP != 7 || result.TotalGold != 11 || result.TotalCrystals != 3 {
				t.Fatalf("lost earned rewards: %+v", result)
			}
			for _, id := range []string{leader.ID, member.ID} {
				c, err := chars.FindByID(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if c.Experience != 7 || c.Money != 111 || c.Crystal != 8 || c.Stats.HP != 1 {
					t.Fatalf("incorrect persisted earnings: %+v", c)
				}
			}
		})
	}
}
