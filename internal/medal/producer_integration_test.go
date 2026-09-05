package medal_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/casino"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
)

// In-memory repositories for producers

type statefulInvRepo struct {
	mu          sync.Mutex
	inventories map[string]coreinventory.Inventory
}

func newStatefulInvRepo() *statefulInvRepo {
	return &statefulInvRepo{inventories: make(map[string]coreinventory.Inventory)}
}

func (r *statefulInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.inventories[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (r *statefulInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *statefulInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inventories[inv.CharacterID] = inv
	return nil
}

// Adventure test stubs
type integrationAdvRepo struct {
	mu       sync.Mutex
	adv      adventure.Adventure
	charRepo *mockCharRepo
}

func (r *integrationAdvRepo) Save(_ context.Context, val adventure.Adventure) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adv = val
	return nil
}

func (r *integrationAdvRepo) FindByID(_ context.Context, _ string) (adventure.Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adv.ID == "" {
		return adventure.Adventure{}, adventure.ErrNotFound
	}
	return r.adv, nil
}

func (r *integrationAdvRepo) ClaimAndApply(_ context.Context, val adventure.Adventure, char corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adv = val
	return r.charRepo.Update(context.Background(), char)
}

func (r *integrationAdvRepo) ListByCharacterID(_ context.Context, charID string, _, _ int) ([]adventure.Adventure, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adv.CharacterID == charID {
		return []adventure.Adventure{r.adv}, 1, nil
	}
	return nil, 0, nil
}

func (r *integrationAdvRepo) ListByCharacterIDByCursor(_ context.Context, charID string, _ int, _ time.Time, _ string) ([]adventure.Adventure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adv.CharacterID == charID {
		return []adventure.Adventure{r.adv}, nil
	}
	return nil, nil
}

func (r *integrationAdvRepo) GetAggregatedStats(_ context.Context, _ string) (adventure.AggregatedStats, error) {
	return adventure.AggregatedStats{}, nil
}

type integrationClock struct{ now time.Time }

func (c *integrationClock) Now() time.Time { return c.now }

type integrationScheduler struct{}

func (s *integrationScheduler) Schedule(_ context.Context, _, _ string, _ map[string]string, _ time.Time) (string, error) {
	return "sched-1", nil
}

// Boss test stub
type integrationBossRepo struct {
	records map[string]boss.CharacterBossRecord
}

func newIntegrationBossRepo() *integrationBossRepo {
	return &integrationBossRepo{records: make(map[string]boss.CharacterBossRecord)}
}

func (b *integrationBossRepo) GetOrCreateRecord(_ context.Context, charID string) (boss.CharacterBossRecord, error) {
	rec, ok := b.records[charID]
	if !ok {
		rec = boss.CharacterBossRecord{
			CharacterID: charID,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		b.records[charID] = rec
	}
	return rec, nil
}

func (b *integrationBossRepo) RecordChallenge(_ context.Context, _ boss.BossChallengeHistory, record boss.CharacterBossRecord, _ corecharacter.Character, _ *coreitem.Instance) error {
	b.records[record.CharacterID] = record
	return nil
}

func (b *integrationBossRepo) GetHistory(_ context.Context, _ string, _ int) ([]boss.BossChallengeHistory, error) {
	return nil, nil
}

func (b *integrationBossRepo) GetLeaderboard(_ context.Context, _ int) ([]boss.BossLeaderboardEntry, error) {
	return nil, nil
}

func (b *integrationBossRepo) GetBoss(id string) (boss.Boss, error) {
	for _, x := range boss.DefaultBossCatalog() {
		if x.ID == id {
			return x, nil
		}
	}
	return boss.Boss{}, boss.ErrBossNotFound
}

func (b *integrationBossRepo) ListBosses() []boss.Boss {
	return boss.DefaultBossCatalog()
}

// PvP test stub
type integrationPvPRepo struct {
	ratings map[string]pvp.ArenaRating
}

func newIntegrationPvPRepo() *integrationPvPRepo {
	return &integrationPvPRepo{ratings: make(map[string]pvp.ArenaRating)}
}

func (p *integrationPvPRepo) GetOrCreateRating(_ context.Context, charID string) (pvp.ArenaRating, error) {
	r, ok := p.ratings[charID]
	if !ok {
		r = pvp.ArenaRating{CharacterID: charID, Rating: pvp.DefaultRating}
		p.ratings[charID] = r
	}
	return r, nil
}

func (p *integrationPvPRepo) RecordMatchAndUpdateRatings(_ context.Context, _ pvp.MatchRecord, att, def pvp.ArenaRating, _ corecharacter.Character) error {
	p.ratings[att.CharacterID] = att
	p.ratings[def.CharacterID] = def
	return nil
}

func (p *integrationPvPRepo) FindOpponents(_ context.Context, _ string, _ int) ([]pvp.OpponentCandidate, error) {
	return nil, nil
}

func (p *integrationPvPRepo) GetMatchHistory(_ context.Context, _ string, _ int) ([]pvp.MatchRecord, error) {
	return nil, nil
}

func (p *integrationPvPRepo) GetDefenseLogs(_ context.Context, _ string, _ int) ([]pvp.MatchRecord, error) {
	return nil, nil
}

func (p *integrationPvPRepo) GetLeaderboard(_ context.Context, _ int) ([]pvp.OpponentCandidate, error) {
	return nil, nil
}

// Casino test stub
type integrationCasinoRepo struct {
	account casino.Account
}

func (c *integrationCasinoRepo) GetAccount(_ context.Context, charID string) (casino.Account, error) {
	return c.account, nil
}

func (c *integrationCasinoRepo) ExchangeGoldToCoins(_ context.Context, charID string, coins int64, _ int) (casino.Account, corecharacter.Character, error) {
	c.account.Coins += coins
	return c.account, corecharacter.Character{ID: charID}, nil
}

func (c *integrationCasinoRepo) ExchangeCoinsToGold(_ context.Context, charID string, coins int64, _ int) (casino.Account, corecharacter.Character, error) {
	c.account.Coins -= coins
	return c.account, corecharacter.Character{ID: charID}, nil
}

func (c *integrationCasinoRepo) AdjustCoins(_ context.Context, _ string, delta int64) (casino.Account, error) {
	c.account.Coins += delta
	return c.account, nil
}

func (c *integrationCasinoRepo) DeductBetAndCreditPayout(_ context.Context, _ string, bet, payout int64) (casino.Account, error) {
	c.account.Coins = c.account.Coins - bet + payout
	return c.account, nil
}

func (c *integrationCasinoRepo) GetAccountForUpdate(ctx context.Context, charID string) (casino.Account, error) {
	return c.GetAccount(ctx, charID)
}

func (c *integrationCasinoRepo) SavePokerGame(_ context.Context, _ casino.IndianPokerGame) error {
	return nil
}

func (c *integrationCasinoRepo) GetActivePokerGame(_ context.Context, _ string) (*casino.IndianPokerGame, error) {
	return nil, nil
}

func (c *integrationCasinoRepo) GetActivePokerGameForUpdate(_ context.Context, _ string) (*casino.IndianPokerGame, error) {
	return nil, nil
}

// Dungeon test stub
type integrationDungeonRepo struct {
	expedition *dungeon.ActiveExpedition
}

func (d *integrationDungeonRepo) SaveActiveExpedition(_ context.Context, exp dungeon.ActiveExpedition) error {
	d.expedition = &exp
	return nil
}

func (d *integrationDungeonRepo) GetActiveExpedition(_ context.Context, charID string) (*dungeon.ActiveExpedition, error) {
	return d.expedition, nil
}

func (d *integrationDungeonRepo) DeleteActiveExpedition(_ context.Context, charID string) error {
	d.expedition = nil
	return nil
}

func (d *integrationDungeonRepo) SaveRecord(_ context.Context, rec dungeon.CharacterDungeonRecord) error {
	return nil
}

func (d *integrationDungeonRepo) GetRecord(_ context.Context, charID string) (dungeon.CharacterDungeonRecord, error) {
	return dungeon.CharacterDungeonRecord{CharacterID: charID}, nil
}

func (d *integrationDungeonRepo) FinalizeExpedition(_ context.Context, _ dungeon.DungeonExpeditionHistory, _ dungeon.CharacterDungeonRecord, _ *corecharacter.Character, _ []coreitem.Instance) error {
	return nil
}

func (d *integrationDungeonRepo) GetHistory(_ context.Context, charID string, limit int) ([]dungeon.DungeonExpeditionHistory, error) {
	return nil, nil
}

type integrationBattleEngine struct {
	result corebattle.Result
}

func (e integrationBattleEngine) Resolve(_ corebattle.Request) (corebattle.Result, error) {
	return e.result, nil
}

func TestProducerHooks_MilestoneProgressAndClaim(t *testing.T) {
	ctx := context.Background()

	// 1. Shared Character and Inventory Repositories
	charRepo := newMockCharRepo()
	hero := corecharacter.Character{
		ID:       "hero-1",
		PlayerID: "player-1",
		Name:     "Hero",
		Level:    20,
		Money:    10000,
		Stats: corecharacter.Stats{
			HP:      500,
			MaxHP:   500,
			Attack:  100,
			Defense: 80,
			Agility: 50,
		},
	}
	charRepo.chars[hero.ID] = hero

	opponent := corecharacter.Character{
		ID:       "opp-1",
		PlayerID: "player-2",
		Name:     "Rival",
		Level:    8,
		Money:    500,
		Stats: corecharacter.Stats{
			HP:      100,
			MaxHP:   100,
			Attack:  20,
			Defense: 15,
			Agility: 15,
		},
	}
	charRepo.chars[opponent.ID] = opponent

	invRepo := newStatefulInvRepo()
	heroInv, _ := coreinventory.New(hero.ID)
	// Give alchemy materials: 5 herbs
	herbItem, err := coreitem.NewInstance("herb", 5)
	if err != nil {
		t.Fatalf("failed to create herb instance: %v", err)
	}
	if err := heroInv.Add(herbItem); err != nil {
		t.Fatalf("failed to add herb to inventory: %v", err)
	}
	_ = invRepo.Save(ctx, heroInv)

	// 2. Initialize Medal Service with test achievement catalog
	achRepo := newMockAchievementRepo()
	catalog := []medal.Achievement{
		{
			ID:                "test_adv_victories",
			Name:              "冒険の第一歩",
			Metric:            medal.MetricAdventureVictories,
			Threshold:         1,
			MedalID:           "medal_adv",
			MedalName:         "冒険者の勲章",
			SmallMedalsReward: 2,
		},
		{
			ID:                "test_boss_slain",
			Name:              "ボス討伐者",
			Metric:            medal.MetricBossesSlain,
			Threshold:         1,
			MedalID:           "medal_boss",
			MedalName:         "討伐者の勲章",
			SmallMedalsReward: 5,
		},
		{
			ID:                "test_pvp_victories",
			Name:              "闘技場の覇者",
			Metric:            medal.MetricPvPVictories,
			Threshold:         1,
			MedalID:           "medal_pvp",
			MedalName:         "闘士の勲章",
			SmallMedalsReward: 3,
		},
		{
			ID:                "test_casino_games",
			Name:              "カジノ通",
			Metric:            medal.MetricCasinoGames,
			Threshold:         1,
			MedalID:           "medal_casino",
			MedalName:         "勝負師の勲章",
			SmallMedalsReward: 2,
		},
		{
			ID:                "test_alchemy_crafts",
			Name:              "錬金術見習い",
			Metric:            medal.MetricAlchemyCrafts,
			Threshold:         1,
			MedalID:           "medal_alchemy",
			MedalName:         "錬金術士の勲章",
			SmallMedalsReward: 4,
		},
		{
			ID:                "test_monsters_slain",
			Name:              "魔物ハンター",
			Metric:            medal.MetricMonstersSlain,
			Threshold:         2, // 1 from adventure + 1 from dungeon
			MedalID:           "medal_monster",
			MedalName:         "狩人の勲章",
			SmallMedalsReward: 3,
		},
	}

	medalService, err := medal.NewService(
		charRepo,
		invRepo,
		"",
		medal.WithAchievementRepository(achRepo, catalog...),
	)
	if err != nil {
		t.Fatalf("failed to create medal service: %v", err)
	}

	// 3. Instantiate and Wire Producers (mirroring cmd/party2/main.go)
	// --- Adventure ---
	advRepo := &integrationAdvRepo{charRepo: charRepo}
	clock := &integrationClock{now: time.Now().UTC()}
	advService, err := adventure.NewServiceWithClock(
		advRepo,
		charRepo,
		integrationBattleEngine{result: corebattle.Result{Outcome: corebattle.OutcomeWin, Reward: corebattle.Reward{Experience: 50}}},
		&integrationScheduler{},
		nil,
		clock,
	)
	if err != nil {
		t.Fatalf("failed to create adventure service: %v", err)
	}
	advService.SetVictoryHook(func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error {
		_ = medalService.RecordProgress(ctx, characterID, medal.MetricAdventureVictories, 1)
		if monstersDefeated > 0 {
			_ = medalService.RecordProgress(ctx, characterID, medal.MetricMonstersSlain, monstersDefeated)
		}
		if goldEarned > 0 {
			_ = medalService.RecordProgress(ctx, characterID, medal.MetricGoldEarned, goldEarned)
		}
		return nil
	})

	// --- Boss ---
	bossRepo := newIntegrationBossRepo()
	bossService, err := boss.NewService(
		bossRepo,
		charRepo,
		integrationBattleEngine{result: corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: hero.ID, Reward: corebattle.Reward{Experience: 200}}},
	)
	if err != nil {
		t.Fatalf("failed to create boss service: %v", err)
	}
	bossService.SetVictoryHook(func(ctx context.Context, characterID string, bossID string, tier int) error {
		return medalService.RecordProgress(ctx, characterID, medal.MetricBossesSlain, 1)
	})

	// --- PvP ---
	pvpRepo := newIntegrationPvPRepo()
	pvpService, err := pvp.NewService(
		pvpRepo,
		charRepo,
		integrationBattleEngine{result: corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: hero.ID, LoserID: opponent.ID}},
	)
	if err != nil {
		t.Fatalf("failed to create pvp service: %v", err)
	}
	pvpService.SetVictoryHook(func(ctx context.Context, winnerID string, loserID string) error {
		return medalService.RecordProgress(ctx, winnerID, medal.MetricPvPVictories, 1)
	})

	// --- Casino ---
	casinoRepo := &integrationCasinoRepo{account: casino.Account{CharacterID: hero.ID, Coins: 500}}
	casinoService, err := casino.NewService(casinoRepo)
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}
	casinoService.SetGamePlayedHook(func(ctx context.Context, characterID string, gameType string) error {
		return medalService.RecordProgress(ctx, characterID, medal.MetricCasinoGames, 1)
	})

	// --- Alchemy ---
	herbDef, _ := coreitem.NewDefinition("herb", "Herb", 10)
	potionDef, _ := coreitem.NewDefinition("potion", "Potion", 50)
	itemCatalog, _ := coreitem.NewCatalog([]coreitem.Definition{herbDef, potionDef})
	recipe, _ := alchemy.NewRecipe("rec-potion", "Craft Potion", "potion", 1, []alchemy.Ingredient{{DefinitionID: "herb", Quantity: 2}}, 10)
	recipeCatalog, _ := alchemy.NewRecipeCatalog([]alchemy.Recipe{recipe})
	alchemyService, err := alchemy.NewService(charRepo, invRepo, recipeCatalog, itemCatalog)
	if err != nil {
		t.Fatalf("failed to create alchemy service: %v", err)
	}
	alchemyService.SetSynthesisHook(func(ctx context.Context, characterID string, recipeID string) error {
		return medalService.RecordProgress(ctx, characterID, medal.MetricAlchemyCrafts, 1)
	})

	// --- Dungeon ---
	dungeonRepo := &integrationDungeonRepo{}
	dungeonService, err := dungeon.NewService(
		dungeonRepo,
		charRepo,
		integrationBattleEngine{result: corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: hero.ID, Reward: corebattle.Reward{Experience: 50}}},
	)
	if err != nil {
		t.Fatalf("failed to create dungeon service: %v", err)
	}
	dungeonService.SetMonsterDefeatedHook(func(ctx context.Context, characterID string, count int) error {
		return medalService.RecordProgress(ctx, characterID, medal.MetricMonstersSlain, count)
	})

	// 4. Trigger Gameplay Actions & Observe Achievement Progress Tracking

	// (a) Adventure Victory
	adv, err := advService.Start(ctx, hero.ID)
	if err != nil {
		t.Fatalf("failed to start adventure: %v", err)
	}
	clock.now = clock.now.Add(2 * time.Hour)
	if _, err := advService.Claim(ctx, adv.ID); err != nil {
		t.Fatalf("failed to claim adventure: %v", err)
	}

	// (b) Boss Victory
	if _, err := bossService.ChallengeBoss(ctx, hero.ID, "king-01"); err != nil {
		t.Fatalf("failed to challenge boss: %v", err)
	}

	// (c) PvP Victory
	if _, err := pvpService.Challenge(ctx, hero.ID, opponent.ID); err != nil {
		t.Fatalf("failed pvp challenge: %v", err)
	}

	// (d) Casino Game
	if _, _, err := casinoService.SpinSlot(ctx, hero.ID, 10); err != nil {
		t.Fatalf("failed casino spin: %v", err)
	}

	// (e) Alchemy Synthesis
	if _, err := alchemyService.Synthesize(ctx, hero.ID, "rec-potion"); err != nil {
		t.Fatalf("failed alchemy synthesize: %v", err)
	}

	// (f) Dungeon Monster Slain
	if _, err := dungeonService.StartExpedition(ctx, hero.ID, "dungeon-01"); err != nil {
		t.Fatalf("failed start dungeon expedition: %v", err)
	}
	// Move East to (1, 0) which has a monster encounter
	stepResult, err := dungeonService.Move(ctx, hero.ID, dungeon.DirectionEast)
	if err != nil {
		t.Fatalf("failed dungeon move: %v", err)
	}
	if stepResult.EventType != dungeon.EventBattle {
		t.Fatalf("expected EventBattle on dungeon move, got %v", stepResult.EventType)
	}

	// 5. Verify All Milestone Achievements Are Unlocked
	achievements, err := medalService.GetAchievements(ctx, hero.ID)
	if err != nil {
		t.Fatalf("failed to get achievements: %v", err)
	}

	achMap := make(map[string]medal.AchievementProgress)
	for _, a := range achievements {
		achMap[a.ID] = a
	}

	expectedMilestones := []string{
		"test_adv_victories",
		"test_boss_slain",
		"test_pvp_victories",
		"test_casino_games",
		"test_alchemy_crafts",
		"test_monsters_slain",
	}

	for _, achID := range expectedMilestones {
		rec, found := achMap[achID]
		if !found {
			t.Errorf("expected achievement %s to be present in progress list", achID)
			continue
		}
		if !rec.IsCompleted {
			t.Errorf("achievement %s should be completed, progress: %d/%d", achID, rec.CurrentProgress, rec.Threshold)
		}
		if rec.IsClaimed {
			t.Errorf("achievement %s should not yet be claimed", achID)
		}
	}

	// 6. Verify Claiming Milestone Rewards
	for _, achID := range expectedMilestones {
		claimResult, err := medalService.ClaimAchievement(ctx, hero.ID, achID)
		if err != nil {
			t.Fatalf("ClaimAchievement(%s) failed: %v", achID, err)
		}
		if claimResult.AchievementID != achID {
			t.Errorf("expected achievement ID %s, got %s", achID, claimResult.AchievementID)
		}
		if claimResult.Medal.MedalID == "" || claimResult.Medal.MedalName == "" {
			t.Errorf("expected commemorative medal to be awarded, got %+v", claimResult)
		}
		if claimResult.SmallMedalsAwarded <= 0 {
			t.Errorf("expected small medals reward > 0, got %d", claimResult.SmallMedalsAwarded)
		}

		// Verify duplicate claim prevention
		_, err = medalService.ClaimAchievement(ctx, hero.ID, achID)
		if !errors.Is(err, medal.ErrAchievementAlreadyClaimed) {
			t.Fatalf("expected ErrAchievementAlreadyClaimed on duplicate claim, got %v", err)
		}
	}

	// Verify character medals collection
	medals, err := medalService.GetCharacterMedals(ctx, hero.ID)
	if err != nil {
		t.Fatalf("failed to get character medals: %v", err)
	}
	if len(medals) != len(expectedMilestones) {
		t.Errorf("expected %d medals, got %d", len(expectedMilestones), len(medals))
	}
}

type dummyPartyLogRepo struct {
	mu   sync.Mutex
	logs []party.PartyAdventureLog
}

func (r *dummyPartyLogRepo) SaveAdventureLog(_ context.Context, log party.PartyAdventureLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, log)
	return nil
}

func (r *dummyPartyLogRepo) ListAdventureLogsByPartyID(_ context.Context, _ string, _, _ int) ([]party.PartyAdventureLog, int, error) {
	return nil, 0, nil
}

type integrationPartyStageProvider struct{}

func (p integrationPartyStageProvider) FindByID(id string) (adventure.Stage, error) {
	return adventure.Stage{
		ID:         "stage-forest",
		Name:       "はじまりの森",
		MinLevel:   1,
		MonsterIDs: []string{"slime-01", "goblin-01"},
	}, nil
}

type integrationPartyMonsterProvider struct{}

func (p integrationPartyMonsterProvider) FindByID(id string) (adventure.Monster, error) {
	return adventure.Monster{
		ID:               id,
		Name:             "モンスター",
		HP:               20,
		Attack:           5,
		Defense:          2,
		ExperienceReward: 40,
		GoldReward:       30,
	}, nil
}

type integrationPartyBattleEngine struct{}

func (e integrationPartyBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return corebattle.PartyBattleResult{
		Outcome: corebattle.OutcomeWin,
		Turns:   2,
		TotalReward: corebattle.Reward{
			Experience: 80,
			Currency:   60,
		},
	}, nil
}

func TestParty_MilestoneIntegration(t *testing.T) {
	ctx := context.Background()

	// 1. Setup characters
	charRepo := newMockCharRepo()
	invRepo := newStatefulInvRepo()
	achRepo := newMockAchievementRepo()

	leader := corecharacter.Character{
		ID:    "char-leader",
		Name:  "LeaderHero",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 30, Defense: 15},
	}
	member := corecharacter.Character{
		ID:    "char-member",
		Name:  "MemberHero",
		Level: 10,
		Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 25, Defense: 12},
	}
	charRepo.chars[leader.ID] = leader
	charRepo.chars[member.ID] = member

	// 2. Setup Achievement Catalog
	catalog := []medal.Achievement{
		{
			ID:                "test_party_adv_victories",
			Name:              "パーティ冒険王",
			Metric:            medal.MetricAdventureVictories,
			Threshold:         1,
			MedalID:           "medal_party_adv",
			MedalName:         "結束の勲章",
			SmallMedalsReward: 5,
		},
		{
			ID:                "test_party_monsters_slain",
			Name:              "共闘モンスターバスター",
			Metric:            medal.MetricMonstersSlain,
			Threshold:         2,
			MedalID:           "medal_party_slayer",
			MedalName:         "討伐隊の勲章",
			SmallMedalsReward: 3,
		},
		{
			ID:                "test_party_gold_earned",
			Name:              "パーティ富豪",
			Metric:            medal.MetricGoldEarned,
			Threshold:         50,
			MedalID:           "medal_party_wealth",
			MedalName:         "協調財産の勲章",
			SmallMedalsReward: 2,
		},
	}

	medalService, err := medal.NewService(
		charRepo,
		invRepo,
		"",
		medal.WithAchievementRepository(achRepo, catalog...),
	)
	if err != nil {
		t.Fatalf("failed to create medal service: %v", err)
	}

	// 3. Setup Party Service and wire VictoryHook (mirroring cmd/party2/main.go)
	partyRepo := party.NewValkeyRepository(nil, party.WithDurableLogRepository(&dummyPartyLogRepo{}))
	partyService, err := party.NewService(
		partyRepo,
		charRepo,
		invRepo,
		integrationPartyStageProvider{},
		integrationPartyMonsterProvider{},
		integrationPartyBattleEngine{},
	)
	if err != nil {
		t.Fatalf("failed to create party service: %v", err)
	}

	partyService.SetVictoryHook(func(ctx context.Context, characterIDs []string, monstersDefeated int, goldEarned int) error {
		for _, cID := range characterIDs {
			_ = medalService.RecordProgress(ctx, cID, medal.MetricAdventureVictories, 1)
			if monstersDefeated > 0 {
				_ = medalService.RecordProgress(ctx, cID, medal.MetricMonstersSlain, monstersDefeated)
			}
			if goldEarned > 0 {
				_ = medalService.RecordProgress(ctx, cID, medal.MetricGoldEarned, goldEarned)
			}
		}
		return nil
	})

	// 4. Create party, join member, mark ready, and start adventure
	detail, err := partyService.CreateParty(ctx, leader.ID, party.CreatePartyRequest{
		Name:    "VictoryCoop",
		StageID: "stage-forest",
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	partyID := detail.Party.ID

	if _, err := partyService.JoinParty(ctx, partyID, member.ID, ""); err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}
	if _, err := partyService.SetReady(ctx, partyID, member.ID, true); err != nil {
		t.Fatalf("SetReady failed: %v", err)
	}

	res, err := partyService.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected victory outcome, got %s", res.Outcome)
	}

	// 5. Verify milestone achievements for both leader and member
	expectedMilestones := []string{
		"test_party_adv_victories",
		"test_party_monsters_slain",
		"test_party_gold_earned",
	}

	for _, charID := range []string{leader.ID, member.ID} {
		achievements, err := medalService.GetAchievements(ctx, charID)
		if err != nil {
			t.Fatalf("failed to get achievements for %s: %v", charID, err)
		}

		achMap := make(map[string]medal.AchievementProgress)
		for _, a := range achievements {
			achMap[a.ID] = a
		}

		for _, achID := range expectedMilestones {
			rec, found := achMap[achID]
			if !found {
				t.Errorf("[%s] expected achievement %s to be present", charID, achID)
				continue
			}
			if !rec.IsCompleted {
				t.Errorf("[%s] achievement %s should be completed, progress: %d/%d", charID, achID, rec.CurrentProgress, rec.Threshold)
			}
			if rec.IsClaimed {
				t.Errorf("[%s] achievement %s should not yet be claimed", charID, achID)
			}

			// Claim achievement
			claimResult, err := medalService.ClaimAchievement(ctx, charID, achID)
			if err != nil {
				t.Fatalf("[%s] ClaimAchievement(%s) failed: %v", charID, achID, err)
			}
			if claimResult.AchievementID != achID {
				t.Errorf("[%s] expected achievement ID %s, got %s", charID, achID, claimResult.AchievementID)
			}
			if claimResult.Medal.MedalID == "" {
				t.Errorf("[%s] expected commemorative medal to be awarded", charID)
			}
			if claimResult.SmallMedalsAwarded <= 0 {
				t.Errorf("[%s] expected small medals reward > 0, got %d", charID, claimResult.SmallMedalsAwarded)
			}
		}

		// Verify commemorative medals awarded
		medals, err := medalService.GetCharacterMedals(ctx, charID)
		if err != nil {
			t.Fatalf("[%s] failed to get character medals: %v", charID, err)
		}
		if len(medals) != len(expectedMilestones) {
			t.Errorf("[%s] expected %d medals, got %d", charID, len(expectedMilestones), len(medals))
		}
	}
}
