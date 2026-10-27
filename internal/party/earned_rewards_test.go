package party

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestPartyAdventure_EarnedRewards(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		defeatFloor int
		cleared     int
		paid        bool
	}{
		{"normal first defeat", adventure.CrawlModeNormal, 1, 0, true},
		{"normal later defeat", adventure.CrawlModeNormal, 2, 1, true},
		{"normal win", adventure.CrawlModeNormal, 0, 10, true},
		{"try later defeat", adventure.CrawlModeTry, 2, 1, false},
		{"try win", adventure.CrawlModeTry, 0, 10, true},
	}
	for _, tc := range cases {
		for _, adapter := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/adapter=%v", tc.name, adapter), func(t *testing.T) {
				ctx := context.Background()
				calls := 0
				engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
					calls++
					outcome := corebattle.OutcomeWin
					if calls == tc.defeatFloor {
						outcome = corebattle.OutcomeDefeat
					}
					hps, mps := make(map[string]int), make(map[string]int)
					for _, p := range req.Allies {
						hps[p.ID], mps[p.ID] = 80, 12
						if outcome == corebattle.OutcomeDefeat {
							hps[p.ID] = 0
						}
					}
					return corebattle.PartyBattleResult{Outcome: outcome, RemainingHP: hps, RemainingMP: mps, TotalReward: corebattle.Reward{Experience: 7, Currency: 11, Crystals: 3}}, nil
				})
				svc, parties, chars, invs, _ := setupPartyAdventureTestService(t, engine)
				if adapter {
					svc.battleSettler = battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs))
				}
				leader := corecharacter.Character{ID: "earned-leader", Name: "Leader", Level: 10, JobLevel: 1, Money: 100, Crystal: 5, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, MP: 20, MaxMP: 20}}
				member := leader
				member.ID, member.Name = "earned-member", "Member"
				partyID := createReadyParty(ctx, t, svc, parties, chars, leader, member)
				p := parties.parties[partyID]
				p.Mode = tc.mode
				parties.parties[partyID] = p
				result, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
				if err != nil {
					t.Fatal(err)
				}
				outcome, hp := string(corebattle.OutcomeWin), 80
				if tc.defeatFloor != 0 {
					outcome, hp = string(corebattle.OutcomeDefeat), 1
				}
				exp, gold, crystals := tc.cleared*7, tc.cleared*11, tc.cleared*3
				if result.Outcome != outcome || result.TotalEXP != exp || result.TotalGold != gold || result.TotalCrystals != crystals {
					t.Fatalf("incorrect crawl totals: %+v", result)
				}
				if !tc.paid {
					exp, gold, crystals = 0, 0, 0
				}
				if len(result.Rewards) != 2 {
					t.Fatalf("rewards: %+v", result.Rewards)
				}
				seen := make(map[string]bool)
				for _, reward := range result.Rewards {
					seen[reward.CharacterID] = true
					c, err := chars.FindByID(ctx, reward.CharacterID)
					if err != nil {
						t.Fatal(err)
					}
					if c.Experience != exp || c.Money != 100+gold || c.Crystal != 5+crystals || c.Stats.HP != hp || c.Stats.MP != 12 || c.Tired != result.ExitFatigue {
						t.Errorf("incorrect persisted state: %+v; result: %+v", c, result)
					}
					if reward.GainedEXP != exp || reward.GainedGold != gold || reward.GainedCrystals != crystals {
						t.Errorf("incorrect summary: %+v", reward)
					}
				}
				if !seen[leader.ID] || !seen[member.ID] {
					t.Errorf("missing participant summaries: %+v", result.Rewards)
				}
				if len(parties.logs) != 1 || parties.logs[0].PartyID != partyID || parties.logs[0].Outcome != outcome || parties.logs[0].TotalEXP != tc.cleared*7 || parties.logs[0].TotalGold != tc.cleared*11 {
					t.Errorf("incorrect log: %+v", parties.logs)
				}
			})
		}
	}
}
