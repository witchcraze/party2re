package adventure_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
)

type defeatEarningsEngine struct {
	corebattle.Engine
	calls int
}

func (e *defeatEarningsEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls++
	r := corebattle.PartyBattleResult{Outcome: corebattle.OutcomeDefeat, RemainingHP: map[string]int{},
		DefeatEarnings: []corebattle.DefeatEarnings{{Reward: corebattle.Reward{Experience: 10, Currency: 20, Crystals: 1}, RecipientIDs: []string{req.Allies[0].ID}}}}
	for _, ally := range req.Allies {
		r.RemainingHP[ally.ID] = 0
	}
	if e.calls == 1 {
		leader, member := req.Allies[0].ID, req.Allies[1].ID
		r.Outcome, r.RemainingHP[leader], r.BonusPercent = corebattle.OutcomeWin, 80, 10
		r.TotalReward = corebattle.Reward{Experience: 22, Currency: 33, Crystals: 2}
		r.DefeatEarnings = []corebattle.DefeatEarnings{
			{Reward: corebattle.Reward{Experience: 7, Currency: 11, Crystals: 1}, RecipientIDs: []string{leader, member}},
			{Reward: corebattle.Reward{Experience: 13, Currency: 19, Crystals: 1}, RecipientIDs: []string{leader}},
		}
	}
	return r, nil
}

func TestAdventure_PerRecipientDefeatIntegration(t *testing.T) {
	db, chars, _, history, settler := earnedRewardStorage(t)
	ctx := context.Background()
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback=%v", fallback), func(t *testing.T) {
			var before []corecharacter.Character
			var ids []string
			for i := 0; i < 2; i++ {
				c, err := database.CreateTestCharacterWithFunds(ctx, db, "", 100)
				if err != nil {
					t.Fatal(err)
				}
				c.Level = 10
				if err := chars.Update(ctx, c); err != nil {
					t.Fatal(err)
				}
				database.RegisterCharacterCleanup(t, db, c)
				before, ids = append(before, c), append(ids, c.ID)
			}
			svc, err := adventure.NewService(history, chars, &defeatEarningsEngine{}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			adventure.WithTransactionProvider(database.NewTransactionProvider(db))(svc)
			if !fallback {
				adventure.WithPostBattleSettler(settler)(svc)
			}
			r, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{CharacterIDs: ids, StageID: adventure.StarterAdventure, Rng: func(int) int { return 0 }})
			if err != nil {
				t.Fatal(err)
			}
			if r.Outcome != corebattle.OutcomeDefeat {
				t.Fatal("expected run defeat", r.Outcome)
			}
			for i, c := range before {
				want := []corebattle.Reward{{Experience: 32, Currency: 53, Crystals: 3}, {Experience: 7, Currency: 12, Crystals: 1}}[i]
				after, err := chars.FindByID(ctx, c.ID)
				if err != nil {
					t.Fatal(err)
				}
				if after.Experience != c.Experience+want.Experience || after.Money != c.Money+want.Currency || after.Crystal != c.Crystal+want.Crystals {
					t.Fatal("incorrect per-recipient SQL assets", after, want)
				}
				a, err := history.FindByID(ctx, r.AdventureIDs[c.ID])
				if err != nil {
					t.Fatal(err)
				}
				if a.ExperienceReward != want.Experience || a.BattleResult.Reward.Experience != want.Experience || a.BattleResult.Reward.Currency != want.Currency {
					t.Fatal("history differs from per-recipient earnings", a)
				}
			}
		})
	}
}
