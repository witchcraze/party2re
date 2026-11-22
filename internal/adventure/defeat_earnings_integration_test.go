package adventure_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
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

type deterministicCoreRNG struct{}

func (deterministicCoreRNG) Intn(int) int     { return 0 }
func (deterministicCoreRNG) IntN(int) int     { return 0 }
func (deterministicCoreRNG) Float64() float64 { return 0 }

type deterministicBattleRand struct{}

func (deterministicBattleRand) Intn(int) (int, error) { return 0, nil }
func (deterministicBattleRand) Float64() float64      { return 0 }

type deterministicCoreEngine struct{ corebattle.Engine }

func (e deterministicCoreEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	req.RNG = deterministicCoreRNG{}
	return e.Engine.ResolvePartyBattle(req)
}

type sqlTestBlessingProvider struct {
	blessings map[string]string
	errFor    map[string]error
}

func (p sqlTestBlessingProvider) GetActiveBlessing(_ context.Context, charID string) (string, error) {
	if err := p.errFor[charID]; err != nil {
		return "", err
	}
	return p.blessings[charID], nil
}

func TestAdventure_BatchCrawlBlessingSQLIntegration(t *testing.T) {
	db, chars, invs, history, _ := earnedRewardStorage(t)
	tx := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	runCase := func(t *testing.T, fallback bool, count int, fallenLeader bool, roll int, blessings map[string]string) (adventure.DungeonCrawlResult, []corecharacter.Character) {
		t.Helper()
		var before []corecharacter.Character
		var ids []string
		for i := 0; i < count; i++ {
			c, err := database.CreateTestCharacterWithFunds(ctx, db, "BlessSQL", 100)
			if err != nil {
				t.Fatal(err)
			}
			c.Level = 50
			c.Stats = corecharacter.Stats{HP: 5000, MaxHP: 5000, Attack: 500, Defense: 100, Agility: 500}
			if fallenLeader && i == 0 {
				c.Stats = corecharacter.Stats{HP: 1, MaxHP: 1, Attack: 1, Defense: 0, Agility: 1}
			} else if fallenLeader && i == 1 {
				c.Stats = corecharacter.Stats{HP: 5000, MaxHP: 5000, Attack: 500, Defense: 100, Agility: 5}
			}
			if err := chars.Update(ctx, c); err != nil {
				t.Fatal(err)
			}
			database.RegisterCharacterCleanup(t, db, c)
			before, ids = append(before, c), append(ids, c.ID)
		}
		prov := sqlTestBlessingProvider{blessings: map[string]string{}}
		for i, id := range ids {
			if b, ok := blessings[fmt.Sprintf("c%d", i)]; ok {
				prov.blessings[id] = b
			}
		}
		opts := []adventure.Option{
			adventure.WithTransactionProvider(tx),
			adventure.WithBlessingProvider(prov),
		}
		if !fallback {
			settler := battle.NewService(
				battle.WithCharacterRepository(chars),
				battle.WithInventoryRepository(invs),
				battle.WithTransactionProvider(tx),
				battle.WithTransactionRunner(runner),
				battle.WithBlessingProvider(prov),
				battle.WithRandomSource(deterministicBattleRand{}),
			)
			opts = append(opts, adventure.WithPostBattleSettler(settler))
		}
		svc, err := adventure.NewService(history, chars, deterministicCoreEngine{}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, opt := range opts {
			opt(svc)
		}
		res, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{
			CharacterIDs: ids,
			StageID:      adventure.StarterAdventure,
			Rng: func(n int) int {
				if n == 4 {
					return roll
				}
				return 0
			},
			RngFloat: func() float64 { return 0 },
		})
		if err != nil {
			t.Fatal(err)
		}
		var after []corecharacter.Character
		for i, c := range before {
			got, err := chars.FindByID(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			a, err := history.FindByID(ctx, res.AdventureIDs[c.ID])
			if err != nil {
				t.Fatal(err)
			}
			want := res.RewardForCharacter(c.ID)
			moneyOK := got.Money == c.Money+want.Currency
			if count == 1 {
				moneyOK = got.Money >= c.Money+want.Currency
			}
			if !moneyOK || got.Experience != c.Experience+want.Experience || got.Crystal != c.Crystal+want.Crystals ||
				a.ExperienceReward != want.Experience || a.BattleResult.Reward.Experience != want.Experience || a.BattleResult.Reward.Currency != want.Currency {
				t.Fatalf("member %d mismatch: stored=%+v hist=%+v want=%+v", i, got, a.BattleResult.Reward, want)
			}
			after = append(after, got)
		}
		return res, after
	}

	// Base (no blessing) reference across adapter & fallback
	baseRes, _ := runCase(t, false, 2, false, 0, nil)
	baseA, baseB := baseRes.RewardForCharacter(baseRes.FloorResults[0].BattleResult.AlliesSurvived[0]), baseRes.RewardForCharacter(baseRes.FloorResults[0].BattleResult.AlliesSurvived[1])
	if baseA != baseB || baseA.Currency == 0 || baseA.Experience == 0 {
		t.Fatalf("unexpected base rewards: a=%+v b=%+v", baseA, baseB)
	}

	// GOLD lucky & EXP lucky & GOLD unlucky across adapter and fallback
	for _, fallback := range []bool{false, true} {
		goldRes, _ := runCase(t, fallback, 2, false, 0, map[string]string{"c0": "GOLD", "c1": "EXP"})
		unluckyRes, _ := runCase(t, fallback, 2, false, 1, map[string]string{"c0": "GOLD", "c1": "EXP"})
		expRes, _ := runCase(t, fallback, 2, false, 0, map[string]string{"c0": "EXP"})

		for _, id := range []string{goldRes.FloorResults[0].BattleResult.AlliesSurvived[0], goldRes.FloorResults[0].BattleResult.AlliesSurvived[1]} {
			g := goldRes.RewardForCharacter(id)
			if g.Currency <= baseA.Currency || g.Experience != baseA.Experience {
				t.Fatalf("fallback=%v GOLD blessing mismatch for %s: got=%+v base=%+v", fallback, id, g, baseA)
			}
		}
		for _, id := range []string{unluckyRes.FloorResults[0].BattleResult.AlliesSurvived[0], unluckyRes.FloorResults[0].BattleResult.AlliesSurvived[1]} {
			if unluckyRes.RewardForCharacter(id) != baseA {
				t.Fatalf("fallback=%v unlucky blessing changed rewards: %+v vs %+v", fallback, unluckyRes.RewardForCharacter(id), baseA)
			}
		}
		for _, id := range []string{expRes.FloorResults[0].BattleResult.AlliesSurvived[0], expRes.FloorResults[0].BattleResult.AlliesSurvived[1]} {
			e := expRes.RewardForCharacter(id)
			if e.Experience <= baseA.Experience || e.Currency != baseA.Currency {
				t.Fatalf("fallback=%v EXP blessing mismatch for %s: got=%+v base=%+v", fallback, id, e, baseA)
			}
		}
	}

	// Surviving vs fallen recipient difference when leader falls on floor 1 and non-leader member acts with GOLD blessing
	fallenRes, _ := runCase(t, false, 2, true, 0, map[string]string{"c0": "EXP", "c1": "GOLD"})
	survivorID := fallenRes.FloorResults[0].BattleResult.AlliesSurvived[0]
	fallenID := fallenRes.FloorResults[0].BattleResult.AlliesFallen[0]
	if fallenRes.RewardForCharacter(fallenID) != (corebattle.Reward{}) || fallenRes.RewardForCharacter(survivorID).Currency <= baseA.Currency {
		t.Fatalf("expected fallen=0 and survivor GOLD-boosted: fallen=%+v survivor=%+v base=%+v", fallenRes.RewardForCharacter(fallenID), fallenRes.RewardForCharacter(survivorID), baseA)
	}

	// Solo crawl applies Gold blessing once via battle.Service (no duplication)
	soloRes, soloAfter := runCase(t, false, 1, false, 0, map[string]string{"c0": "GOLD"})
	wantSoloGold := int(float64(soloRes.TotalGold) * 1.5)
	if soloAfter[0].Money != 100+wantSoloGold {
		t.Fatalf("solo Gold mismatch: got=%d, want=%d (total=%d)", soloAfter[0].Money-100, wantSoloGold, soloRes.TotalGold)
	}

	// Non-leader blessing read failure propagates without partial DB writes
	c1, err := database.CreateTestCharacterWithFunds(ctx, db, "BlessFail1", 100)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := database.CreateTestCharacterWithFunds(ctx, db, "BlessFail2", 100)
	if err != nil {
		t.Fatal(err)
	}
	database.RegisterCharacterCleanup(t, db, c1)
	database.RegisterCharacterCleanup(t, db, c2)
	readErr := errors.New("member blessing unavailable")
	failSvc, err := adventure.NewService(history, chars, deterministicCoreEngine{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	adventure.WithTransactionProvider(tx)(failSvc)
	adventure.WithBlessingProvider(sqlTestBlessingProvider{errFor: map[string]error{c2.ID: readErr}})(failSvc)
	if _, err := failSvc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{CharacterIDs: []string{c1.ID, c2.ID}, StageID: adventure.StarterAdventure}); !errors.Is(err, readErr) {
		t.Fatalf("expected %v, got %v", readErr, err)
	}
	if _, total, err := history.ListByCharacterID(ctx, c1.ID, 10, 0); err != nil || total != 0 {
		t.Fatalf("expected 0 history rows after blessing read error, got %d (%v)", total, err)
	}
}
