package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/medal"
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
	if err := core.initPlayerAndChar(soc.guildRepo, econ.fleamarketRepo, soc.notification, logger); err != nil {
		return nil, err
	}
	cmbt, err := newCmbtServices(db, core, soc, valkeyClient)
	if err != nil {
		return nil, err
	}
	misc, err := newMiscServices(db, core, soc, econ, valkeyClient)
	if err != nil {
		return nil, err
	}

	econ.initStore(core, soc.guildRepo, soc.timer)
	wireHooks(cmbt, econ, misc, soc)

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
		_, hookErr := misc.eventplaza.RecordVictoryBanquet(ctx, bossID, bossName, slayerID, slayerName, tier)
		return hookErr
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
	}
	if misc.helper != nil {
		econ.shop.SetHelperProvider(misc.helper)
	}
	if misc.tavern != nil {
		soc.home.SetFullnessResetter(misc.tavern)
		cmbt.adv.SetPostAdventureHook(func(ctx context.Context, characterID string) error {
			_, err := misc.tavern.ClaimDelivery(ctx, characterID)
			if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
				return nil
			}
			return err
		})
	}
	if misc.chapel != nil {
		soc.home.SetBlessingCleaner(misc.chapel)
	}
	if econ.alchemy != nil {
		soc.home.SetAlchemyCompleter(econ.alchemy)
	}

	soc.registerWorkerHandlers(misc.activity, misc.chapel)

	if soc.sched != nil && misc.chapel != nil {
		wireChapelDailyReset(soc.sched)
	}
}

// wireChapelDailyReset enqueues the daily JST midnight reset action if not already scheduled.
func wireChapelDailyReset(sched *scheduling.Service) {
	ctx := context.Background()
	next := chapel.NextMidnightJST(time.Now())
	_ = sched.ScheduleWithID(ctx, chapel.DailyResetActionID(next), chapel.ActionTypeChapelReset, "system", nil, next)
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
		http.WithGod(misc.god),
		http.WithMonster(misc.monster),
		http.WithContest(misc.contest),
		http.WithParty(cmbt.party),
		http.WithAltar(misc.altar),
		http.WithWishingWell(misc.wishingwell),
		http.WithMaintenance(misc.maint),
	}

	return http.NewHandler(core.playerService, core.charService, cmbt.adv, econ.shop, opts...)
}
