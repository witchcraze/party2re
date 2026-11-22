package party

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
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

func TestPartyAdventure_GoldAndExpBlessings(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		blessing string
		rngVal   int
		wantEXP  int
		wantGold int
	}{
		{name: "none", blessing: "", rngVal: 0, wantEXP: 1100, wantGold: 550},
		{name: "gold lucky", blessing: "GOLD", rngVal: 0, wantEXP: 1100, wantGold: 820},
		{name: "exp lucky", blessing: "EXP", rngVal: 0, wantEXP: 1650, wantGold: 550},
		{name: "gold unlucky", blessing: "GOLD", rngVal: 1, wantEXP: 1100, wantGold: 550},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blessingProv := mockPartyBlessingProvider{
				blessings: map[string]string{
					"leader": tc.blessing,
					"member": tc.blessing,
				},
			}
			svc, parties, chars, invs, depotRepo := setupPartyAdventureTestService(
				t,
				fixedBattleEngine{outcome: corebattle.OutcomeWin},
				WithBlessingProvider(blessingProv),
				WithRNG(func(n int) int {
					if n <= 0 {
						return 0
					}
					return tc.rngVal % n
				}),
			)
			svc.battleSettler = battle.NewService(
				battle.WithCharacterRepository(chars),
				battle.WithInventoryRepository(invs),
				battle.WithDepotRepository(depotRepo),
				battle.WithBlessingProvider(blessingProv),
				battle.WithRandomSource(fixedPartyBattleRand{val: 0}),
			)

			leader := corecharacter.Character{ID: "leader", Name: "Leader", Level: 10, Money: 100, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
			member := leader
			member.ID, member.Name = "member", "Member"
			partyID := createReadyParty(ctx, t, svc, parties, chars, leader, member)

			res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatalf("StartPartyAdventure failed: %v", err)
			}
			for _, summary := range res.Rewards {
				c, err := chars.FindByID(ctx, summary.CharacterID)
				if err != nil {
					t.Fatal(err)
				}
				if summary.GainedEXP != tc.wantEXP || summary.GainedGold != tc.wantGold || c.Experience != tc.wantEXP || c.Money != 100+tc.wantGold {
					t.Fatalf("%s: summary=%+v stored=%+v, want EXP=%d Gold=%d", summary.CharacterID, summary, c, tc.wantEXP, tc.wantGold)
				}
			}
		})
	}
}

type perCharacterBlessingErrorProvider struct {
	blessings map[string]string
	errFor    map[string]error
}

func (p perCharacterBlessingErrorProvider) GetActiveBlessing(_ context.Context, characterID string) (string, error) {
	if err := p.errFor[characterID]; err != nil {
		return "", err
	}
	return p.blessings[characterID], nil
}

type partyCrawlAdvRepo struct {
	*memoryAdventureRepoForPartyTest
}

func (r partyCrawlAdvRepo) ListByCharacterID(context.Context, string, int, int) ([]adventure.Adventure, int, error) {
	return nil, 0, nil
}
func (r partyCrawlAdvRepo) ListByCharacterIDByCursor(context.Context, string, int, time.Time, string) ([]adventure.Adventure, error) {
	return nil, nil
}
func (r partyCrawlAdvRepo) GetAggregatedStats(context.Context, string) (adventure.AggregatedStats, error) {
	return adventure.AggregatedStats{}, nil
}

type partyCrawlResolver struct{ battleEngineFunc }

func (r partyCrawlResolver) Resolve(corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{}, nil
}

func TestPartyAdventure_NonLeaderBlessingReadFailureAndExecuteCrawlParity(t *testing.T) {
	ctx := context.Background()

	// 1. Non-leader blessing read failure aborts without mutating party or characters
	leader := corecharacter.Character{ID: "p-lead", Name: "Leader", Level: 10, Money: 100, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
	member := leader
	member.ID, member.Name = "p-mem", "Member"
	readErr := errors.New("member blessing read failed")
	failSvc, failParties, failChars, _, _ := setupPartyAdventureTestService(
		t,
		fixedBattleEngine{outcome: corebattle.OutcomeWin},
		WithBlessingProvider(perCharacterBlessingErrorProvider{errFor: map[string]error{member.ID: readErr}}),
	)
	partyID := createReadyParty(ctx, t, failSvc, failParties, failChars, leader, member)
	if _, err := failSvc.StartPartyAdventure(ctx, partyID, leader.ID); !errors.Is(err, readErr) {
		t.Fatalf("expected %v, got %v", readErr, err)
	}
	if p, _ := failParties.GetParty(ctx, partyID); p.Status != StatusRecruiting {
		t.Fatalf("party remained %s after blessing error", p.Status)
	}
	if len(failParties.logs) != 0 {
		t.Fatalf("unexpected adventure logs: %+v", failParties.logs)
	}

	// 2. Party start (adapter & fallback) matches multi-character ExecuteCrawl (adapter & fallback)
	blessings := mockPartyBlessingProvider{blessings: map[string]string{leader.ID: "GOLD", member.ID: "EXP"}}
	rngFn := func(n int) int { return 0 }
	rngFloatFn := func() float64 { return 0 }
	engine := battleEngineFunc(func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		return corebattle.PartyBattleResult{
			Outcome:      corebattle.OutcomeWin,
			RemainingHP:  map[string]int{leader.ID: 80, member.ID: 80},
			BonusPercent: 10,
			TotalReward:  corebattle.Reward{Experience: 110, Currency: 55},
			DefeatEarnings: []corebattle.DefeatEarnings{
				{ActorID: leader.ID, Reward: corebattle.Reward{Experience: 50, Currency: 25}, RecipientIDs: []string{leader.ID, member.ID}},
				{ActorID: member.ID, Reward: corebattle.Reward{Experience: 50, Currency: 25}, RecipientIDs: []string{leader.ID, member.ID}},
			},
		}, nil
	})

	var results [4]map[string]corebattle.Reward
	for idx, isParty := range []bool{true, false} {
		for j, useAdapter := range []bool{true, false} {
			slot := idx*2 + j
			results[slot] = make(map[string]corebattle.Reward)
			if isParty {
				svc, parties, chars, invs, depotRepo := setupPartyAdventureTestService(t, engine, WithBlessingProvider(blessings), WithRNG(rngFn))
				if useAdapter {
					svc.battleSettler = battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs), battle.WithDepotRepository(depotRepo), battle.WithBlessingProvider(blessings), battle.WithRandomSource(fixedPartyBattleRand{val: 0}))
				}
				pID := createReadyParty(ctx, t, svc, parties, chars, leader, member)
				res, err := svc.StartPartyAdventure(ctx, pID, leader.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range res.Rewards {
					results[slot][r.CharacterID] = corebattle.Reward{Experience: r.GainedEXP, Currency: r.GainedGold, Crystals: r.GainedCrystals}
				}
			} else {
				chars := newMockCharacterRepository()
				chars.chars[leader.ID], chars.chars[member.ID] = leader, member
				invs := &mockInventoryRepository{inventories: map[string]coreinventory.Inventory{}}
				advs := partyCrawlAdvRepo{newMemoryAdventureRepoForPartyTest()}
				opts := []adventure.Option{adventure.WithBlessingProvider(blessings)}
				if useAdapter {
					opts = append(opts, adventure.WithPostBattleSettler(battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs), battle.WithBlessingProvider(blessings), battle.WithRandomSource(fixedPartyBattleRand{val: 0}))))
				}
				svc, err := adventure.NewService(advs, chars, partyCrawlResolver{engine}, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, opt := range opts {
					opt(svc)
				}
				res, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{CharacterIDs: []string{leader.ID, member.ID}, StageID: "stage-01", Rng: rngFn, RngFloat: rngFloatFn})
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{leader.ID, member.ID} {
					results[slot][id] = res.RewardForCharacter(id)
				}
			}
		}
	}
	for i := 1; i < len(results); i++ {
		for _, id := range []string{leader.ID, member.ID} {
			if results[i][id] != results[0][id] {
				t.Fatalf("parity mismatch slot %d vs 0 for %s: %+v vs %+v", i, id, results[i][id], results[0][id])
			}
		}
	}
	// 10 floors * ((50 + 75) EXP, (37 + 25) Gold) * 1.1 = 10 * (137 EXP, 68 Gold) = 1370 EXP, 680 Gold
	want := corebattle.Reward{Experience: 1370, Currency: 680}
	if results[0][leader.ID] != want || results[0][member.ID] != want {
		t.Fatalf("unexpected earnings: %+v, want %+v", results[0], want)
	}
}
