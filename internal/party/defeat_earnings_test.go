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

func TestPartyAdventure_PerDefeatRewards(t *testing.T) {
	for _, mode := range []string{adventure.CrawlModeNormal, adventure.CrawlModeTry} {
		for _, defeat := range []bool{false, true} {
			for _, adapter := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/defeat=%v/adapter=%v", mode, defeat, adapter), func(t *testing.T) {
					calls := 0
					engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
						calls++
						r := corebattle.PartyBattleResult{
							Outcome: corebattle.OutcomeWin, RemainingHP: map[string]int{"leader": 80, "member": 0},
							TotalReward:    corebattle.Reward{Experience: 10, Currency: 20, Crystals: 1},
							DefeatEarnings: []corebattle.DefeatEarnings{{Reward: corebattle.Reward{Experience: 10, Currency: 20, Crystals: 1}, RecipientIDs: []string{"leader"}}},
						}
						if calls == 1 {
							r.BonusPercent = 10
							r.TotalReward = corebattle.Reward{Experience: 22, Currency: 33, Crystals: 2}
							r.DefeatEarnings = []corebattle.DefeatEarnings{
								{Reward: corebattle.Reward{Experience: 7, Currency: 11, Crystals: 1}, RecipientIDs: []string{"leader", "member"}},
								{Reward: corebattle.Reward{Experience: 13, Currency: 19, Crystals: 1}, RecipientIDs: []string{"leader"}},
							}
						} else {
							if len(req.Allies) != 1 || req.Allies[0].ID != "leader" {
								t.Fatal("fallen member re-entered the battle", req.Allies)
							}
							if defeat {
								r.Outcome, r.RemainingHP["leader"] = corebattle.OutcomeDefeat, 0
							}
						}
						return r, nil
					})
					history := newMemoryAdventureRepoForPartyTest()
					svc, parties, chars, invs, _ := setupPartyAdventureTestService(t, engine, WithAdventureRepository(history))
					if adapter {
						svc.battleSettler = battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs))
					}
					leader := corecharacter.Character{ID: "leader", Name: "Leader", Level: 10, Money: 100, Crystal: 5, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
					member := leader
					member.ID, member.Name = "member", "Member"
					ctx := context.Background()
					partyID := createReadyParty(ctx, t, svc, parties, chars, leader, member)
					p := parties.parties[partyID]
					p.Mode = mode
					parties.parties[partyID] = p
					r, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
					if err != nil {
						t.Fatal(err)
					}
					want := map[string]corebattle.Reward{
						"leader": {Experience: 112, Currency: 213, Crystals: 11},
						"member": {Experience: 7, Currency: 12, Crystals: 1},
					}
					if defeat {
						want["leader"] = corebattle.Reward{Experience: 32, Currency: 53, Crystals: 3}
						if mode == adventure.CrawlModeTry {
							want["leader"], want["member"] = corebattle.Reward{}, corebattle.Reward{}
						}
					}
					if len(r.Rewards) != 2 || len(history.saved) != 2 {
						t.Fatal("missing summaries or history", r.Rewards, history.saved)
					}
					for _, summary := range r.Rewards {
						e := want[summary.CharacterID]
						c, err := chars.FindByID(ctx, summary.CharacterID)
						if err != nil {
							t.Fatal(err)
						}
						if c.Experience != e.Experience || c.Money != 100+e.Currency || c.Crystal != 5+e.Crystals || summary.GainedEXP != e.Experience || summary.GainedGold != e.Currency || summary.GainedCrystals != e.Crystals {
							t.Fatalf("%s: stored=%+v summary=%+v want=%+v", c.ID, c, summary, e)
						}
					}
					for _, a := range history.saved {
						if a.ExperienceReward != want[a.CharacterID].Experience || a.BattleResult.Reward != want[a.CharacterID] {
							t.Fatal("history differs from paid reward", a)
						}
					}
				})
			}
		}
	}
}
