package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/contest"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/tavern"
)

type appWiring struct {
	handler *http.Handler
	worker  *scheduling.Worker
}

// wireApp coordinates all service groups, hooks, and HTTP handler construction.
func wireApp(
	db *sql.DB,
	valkeyClient valkeygo.Client,
	cfg Config,
	logger logging.Logger,
) (*appWiring, error) {
	core, err := newCoreServices(db, valkeyClient)
	if err != nil {
		return nil, err
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		return nil, err
	}
	soc, err := newSocServices(db, core, econ, valkeyClient, logger)
	if err != nil {
		return nil, err
	}
	if err := core.initPlayerAndChar(soc.guildRepo, soc.guild, econ.fleamarketRepo, soc.notification, logger); err != nil {
		return nil, err
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, valkeyClient)
	if err != nil {
		return nil, err
	}
	misc, err := newMiscServices(db, core, soc, econ, valkeyClient)
	if err != nil {
		return nil, err
	}

	econ.initStore(core, soc.guildRepo, soc.timer, valkeyClient)
	wireHooks(core, cmbt, econ, misc, soc)

	apiHandler, err := newHTTPHandler(cfg, core, econ, cmbt, soc, misc)
	if err != nil {
		return nil, err
	}

	return &appWiring{
		handler: apiHandler,
		worker:  soc.worker,
	}, nil
}

// wireHooks consolidates all domain event hook registrations across services.
func wireHooks(
	core *coreServices,
	cmbt *cmbtServices,
	econ *econServices,
	misc *miscServices,
	soc *socServices,
) {
	cmbt.boss.SetVictoryHook(func(ctx context.Context, characterID string, bossID string, tier int) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricBossesSlain, 1)
	})

	cmbt.pvp.SetVictoryHook(func(ctx context.Context, winnerID, loserID string) error {
		return misc.medal.RecordProgress(ctx, winnerID, medal.MetricPvPVictories, 1)
	})

	cmbt.dungeon.SetMonsterDefeatedHook(func(ctx context.Context, characterID string, count int) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricMonstersSlain, count)
	})

	econ.alchemy.SetSynthesisHook(func(ctx context.Context, characterID string, recipeID string) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricAlchemyCrafts, 1)
	})

	cmbt.boss.SetVictoryBanquetHook(func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error {
		_, err := misc.eventplaza.RecordVictoryBanquet(ctx, bossID, bossName, slayerID, slayerName, tier)
		return err
	})

	misc.casino.SetGamePlayedHook(func(ctx context.Context, characterID string, gameName string) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricCasinoGames, 1)
	})

	cmbt.adv.SetVictoryHook(func(ctx context.Context, characterID string, monstersDefeated int, goldEarned int) error {
		_ = misc.medal.RecordProgress(ctx, characterID, medal.MetricAdventureVictories, 1)
		if monstersDefeated > 0 {
			_ = misc.medal.RecordProgress(ctx, characterID, medal.MetricMonstersSlain, monstersDefeated)
		}
		if goldEarned > 0 {
			_ = misc.medal.RecordProgress(ctx, characterID, medal.MetricGoldEarned, goldEarned)
		}
		return nil
	})

	cmbt.party.SetVictoryHook(func(ctx context.Context, characterIDs []string, monstersDefeated int, goldEarned int) error {
		for _, cID := range characterIDs {
			_ = misc.medal.RecordProgress(ctx, cID, medal.MetricAdventureVictories, 1)
			if monstersDefeated > 0 {
				_ = misc.medal.RecordProgress(ctx, cID, medal.MetricMonstersSlain, monstersDefeated)
			}
			if goldEarned > 0 {
				_ = misc.medal.RecordProgress(ctx, cID, medal.MetricGoldEarned, goldEarned)
			}
		}
		return nil
	})

	if misc.collection != nil {
		econ.depot.SetCollectionRecorder(misc.collection)
		econ.shop.SetCollectionRecorder(misc.collection)
		econ.store.SetCollectionRecorder(misc.collection)
		if econ.fleamarket != nil {
			econ.fleamarket.SetCollectionRecorder(misc.collection)
		}
		if misc.eventplaza != nil {
			misc.eventplaza.SetCollectionRecorder(misc.collection)
		}
		if misc.secretshop != nil {
			misc.secretshop.SetCollectionRecorder(misc.collection)
		}
	}
	if cmbt.battle != nil {
		if misc.monster != nil {
			cmbt.battle.SetMonsterTamer(monsterTamerAdapter{misc.monster})
		}
		if misc.chapel != nil {
			cmbt.battle.SetBlessingProvider(chapelBlessingAdapter{misc.chapel})
			if cmbt.adv != nil {
				cmbt.adv.SetBlessingProvider(chapelBlessingAdapter{misc.chapel})
			}
			if cmbt.party != nil {
				cmbt.party.SetBlessingProvider(chapelBlessingAdapter{misc.chapel})
			}
		}
		if misc.collection != nil {
			cmbt.battle.SetMonsterDefeatRecorder(misc.collection)
			if cmbt.boss != nil {
				cmbt.boss.SetMonsterDefeatRecorder(misc.collection)
			}
		}
	}
	if misc.helper != nil {
		econ.shop.SetHelperProvider(misc.helper)
		econ.store.SetHelperProvider(misc.helper)
		if misc.eventplaza != nil {
			misc.eventplaza.SetHelperProvider(misc.helper)
		}
	}
	if misc.tavern != nil {
		soc.home.SetFullnessResetter(misc.tavern)
		postAdventureHook := func(ctx context.Context, characterID string) error {
			_ = misc.tavern.ResetFullness(ctx, characterID)
			_, err := misc.tavern.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
				return nil
			}
			return err
		}
		if cmbt.adv != nil {
			cmbt.adv.SetPostAdventureHook(postAdventureHook)
		}
		if cmbt.party != nil {
			cmbt.party.SetPostAdventureHook(postAdventureHook)
		}
	}
	if misc.monsterRepo != nil || misc.homeMemberRepo != nil {
		soc.home.SetHomePetReader(&homePetAdapter{
			monsterRepo:    misc.monsterRepo,
			homeMemberRepo: misc.homeMemberRepo,
		})
	}
	if misc.chapel != nil {
		soc.home.SetBlessingCleaner(misc.chapel)
	}
	if econ.alchemy != nil {
		soc.home.SetAlchemyCompleter(econ.alchemy)
		soc.home.SetRecipeLearner(alchemyRecipeLearnerAdapter{
			alchemy: econ.alchemy,
			items:   core.itemCatalog,
		})
	}
	if econ.store != nil {
		soc.home.SetCostumeResetter(econ.store)
		soc.home.SetCostumeApplier(econ.store)
		if misc.job != nil {
			misc.job.SetCostumeResetter(econ.store)
		}
	}
	if misc.job != nil {
		soc.home.SetJobStateRestorer(misc.job)
		if soc.ranking != nil {
			misc.job.SetJobChangeTracker(soc.ranking)
		}
	}
	if soc.ranking != nil {
		legendInductor := legendInductorAdapter{ranking: soc.ranking}
		if misc.collection != nil {
			misc.collection.SetLegendInductor(legendInductor)
		}
		if misc.job != nil {
			misc.job.SetLegendInductor(legendInductor)
		}
		if econ.alchemy != nil {
			econ.alchemy.SetLegendInductor(legendInductor)
		}
	}

	soc.registerWorkerHandlers(misc.activity, misc.chapel, misc.lottery, misc.contest)

	if soc.sched != nil && misc.lottery != nil {
		wireTakarakujiDrawing(soc.sched)
	}
	if soc.sched != nil && soc.guild != nil {
		wireGuildInactivityCheck(soc.sched)
		wireGuildPointDecay(soc.sched)
	}
	if soc.sched != nil && misc.contest != nil {
		wireContestSettlement(soc.sched, misc.contest)
	}
	if soc.sched != nil && soc.ranking != nil {
		wireRankingWeeklyRotation(soc.sched)
	}
}

// wireRankingWeeklyRotation enqueues the weekly Sunday midnight job change ranking rotation if not already scheduled.
func wireRankingWeeklyRotation(sched *scheduling.Service) {
	ctx := context.Background()
	next := ranking.NextSundayMidnightJST(time.Now())
	_ = sched.ScheduleWithID(ctx, ranking.WeeklyRotateActionID(next), ranking.RankingActionTypeRotateWeekly, "system", nil, next)
}

// wireContestSettlement enqueues the contest settlement action if not already scheduled.
func wireContestSettlement(sched *scheduling.Service, contestSvc *contest.Service) {
	ctx := context.Background()
	activeRound, err := contestSvc.GetActiveRound(ctx)
	if err != nil {
		return
	}
	next := activeRound.EndTime
	_ = sched.ScheduleWithID(ctx, contest.SettlementActionID(activeRound.Round, next), contest.ActionTypeContestSettlement, "system", nil, next)
}

// wireGuildInactivityCheck enqueues the daily JST midnight inactivity check if not already scheduled.
func wireGuildInactivityCheck(sched *scheduling.Service) {
	ctx := context.Background()
	next := guild.NextMidnightJST(time.Now())
	_ = sched.ScheduleWithID(ctx, guild.DailyInactivityCheckActionID(next), guild.ActionTypeGuildInactivityCheck, "system", nil, next)
}

// wireGuildPointDecay enqueues the daily JST midnight guild point decay if not already scheduled.
func wireGuildPointDecay(sched *scheduling.Service) {
	ctx := context.Background()
	next := guild.NextMidnightJST(time.Now())
	_ = sched.ScheduleWithID(ctx, guild.DailyPointDecayActionID(next), guild.ActionTypeGuildPointDecay, "system", nil, next)
}

// wireTakarakujiDrawing enqueues the recurring Takarakuji drawing action if not already scheduled.
func wireTakarakujiDrawing(sched *scheduling.Service) {
	ctx := context.Background()
	next := lottery.NextDrawDateJST(time.Now())
	_ = sched.ScheduleWithID(ctx, lottery.DrawActionID(next), lottery.ActionTypeTakarakujiDraw, "system", nil, next)
}

func newHTTPHandler(
	cfg Config,
	core *coreServices,
	econ *econServices,
	cmbt *cmbtServices,
	soc *socServices,
	misc *miscServices,
) (*http.Handler, error) {
	opts := []http.Option{
		http.WithRateLimiter(soc.limiter),
		http.WithAdminAPIKey(cfg.Admin.APIKey),
		http.WithAllowedOrigins(http.ParseCORSOrigins(cfg.CORS.AllowedOrigins)...),
	}
	if cfg.Server.TrustedProxies != "" {
		prefixes, err := http.ParseTrustedProxies(cfg.Server.TrustedProxies)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxies: %w", err)
		}
		opts = append(opts, http.WithTrustedProxies(prefixes...))
	}
	if soc.schedRepo != nil {
		opts = append(opts, http.WithPlayerContext(newPlayerContext(core, soc, econ, cmbt, misc)))
	}
	opts = append(opts,
		http.WithHelper(misc.helper),
		http.WithRescue(misc.rescue),
		http.WithMedal(misc.medal),
		http.WithPark(soc.park),
		http.WithRanking(soc.ranking),
		http.WithJob(misc.job),
		http.WithChapel(misc.chapel),
		http.WithCollection(misc.collection),
		http.WithDepot(econ.depot),
		http.WithBank(econ.bank),
		http.WithLottery(misc.lottery),
		http.WithCasino(misc.casino),
		http.WithChallenge(cmbt.challenge),
		http.WithBoss(cmbt.boss),
		http.WithDungeon(cmbt.dungeon),
		http.WithPvP(cmbt.pvp),
		http.WithGvG(cmbt.gvg),
		http.WithReplay(cmbt.replay),
		http.WithAuction(econ.auction),
		http.WithNotification(soc.notification),
		http.WithHome(soc.home),
		http.WithCustomSkill(cmbt.customSkill),
		http.WithEventPlaza(misc.eventplaza),
		http.WithSecretShop(misc.secretshop),
		http.WithTavern(misc.tavern),
		http.WithAlchemy(econ.alchemy),
		http.WithPlantation(econ.plantation),
		http.WithBlackMarket(misc.blackmarket),
		http.WithFleaMarket(econ.fleamarket),
		http.WithGemStore(econ.gemStore),
		http.WithStore(econ.store),
		http.WithOracleShop(econ.store),
		http.WithGod(misc.god),
		http.WithMonster(misc.monster),
		http.WithContest(misc.contest),
		http.WithGuild(soc.guild),
		http.WithParty(cmbt.party),
		http.WithAltar(misc.altar),
		http.WithWishingWell(misc.wishingwell),
		http.WithBlacksmith(econ.blacksmith),
		http.WithMaintenance(misc.maint),
	)

	return http.NewHandler(core.playerService, core.charService, cmbt.adv, econ.shop, opts...)
}

type homePetAdapter struct {
	monsterRepo    *database.MonsterRepository
	homeMemberRepo *database.HomeMemberRepository
}

func (a *homePetAdapter) ListHomePets(ctx context.Context, characterID string) ([]home.HomePet, error) {
	var res []home.HomePet

	if a.monsterRepo != nil {
		monsters, err := a.monsterRepo.ListByCharacterIDAndLocation(ctx, characterID, monster.LocationHome)
		if err != nil {
			return nil, err
		}
		for _, m := range monsters {
			res = append(res, home.HomePet{
				ID:         m.ID,
				CustomName: m.CustomName,
				MonsterID:  m.MonsterID,
			})
		}
	}

	if a.homeMemberRepo != nil {
		members, err := a.homeMemberRepo.FindByCharacterID(ctx, characterID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			res = append(res, home.HomePet{
				ID:         m.ID,
				CustomName: m.Name,
				MonsterID:  m.Icon,
			})
		}
	}

	return res, nil
}

type monsterTamerAdapter struct {
	svc *monster.Service
}

func (a monsterTamerAdapter) TameMonster(ctx context.Context, characterID, monsterID, customName string) error {
	_, err := a.svc.TameMonster(ctx, characterID, monsterID, customName)
	if errors.Is(err, monster.ErrBoxFull) {
		return battle.ErrMonsterBoxFull
	}
	return err
}

type chapelBlessingAdapter struct {
	svc *chapel.Service
}

func (a chapelBlessingAdapter) GetActiveBlessing(ctx context.Context, characterID string) (string, error) {
	b, err := a.svc.GetBlessing(ctx, characterID)
	if err != nil {
		return "", err
	}
	return string(b.ActiveBlessing), nil
}

type legendInductorAdapter struct {
	ranking *ranking.Service
}

func (a legendInductorAdapter) RecordLegend(ctx context.Context, category, characterID string) error {
	if a.ranking == nil {
		return nil
	}
	_, err := a.ranking.RecordLegend(ctx, ranking.LegendEntry{
		Category:    ranking.LegendCategory(category),
		CharacterID: characterID,
	})
	return err
}

type alchemyRecipeLearnerAdapter struct {
	alchemy *alchemy.Service
	items   coreitem.DefinitionProvider
}

func (a alchemyRecipeLearnerAdapter) LearnRecipe(ctx context.Context, characterID string, pool []string) (home.LearnedRecipe, error) {
	if a.alchemy == nil {
		return home.LearnedRecipe{}, home.ErrRecipeLearnerUnavailable
	}
	r, err := a.alchemy.LearnRecipe(ctx, characterID, pool)
	if err != nil {
		if errors.Is(err, alchemy.ErrNoRecipesToLearn) {
			return home.LearnedRecipe{}, home.ErrNoRecipesToLearn
		}
		return home.LearnedRecipe{}, err
	}
	baseName := ""
	materialName := ""
	if len(r.Ingredients) > 0 {
		if a.items != nil {
			if def, err := a.items.FindByID(r.Ingredients[0].DefinitionID); err == nil {
				baseName = def.Name
			}
		}
		if baseName == "" {
			baseName = r.Ingredients[0].DefinitionID
		}

		if len(r.Ingredients) > 1 {
			if a.items != nil {
				if def, err := a.items.FindByID(r.Ingredients[1].DefinitionID); err == nil {
					materialName = def.Name
				}
			}
			if materialName == "" {
				materialName = r.Ingredients[1].DefinitionID
			}
		} else {
			materialName = baseName
		}
	}
	return home.LearnedRecipe{
		BaseName:     baseName,
		MaterialName: materialName,
	}, nil
}
