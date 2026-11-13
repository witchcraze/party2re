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
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/contest"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
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
	soc.home.SetCompanionPhraseLimit(cfg.Home.CompanionPhraseLimit)
	soc.home.SetLetterLimits(cfg.Home.LetterLogLimit, cfg.Home.LetterMaxLength)
	soc.placeLog = placelog.New(valkeyClient, cfg.PlaceLog)
	soc.placeMembers = placelog.NewMembers(valkeyClient, cfg.PlaceMemberTTL)
	referrals := referralDeliveryAdapter{depots: econ.depotRepo, home: soc.home, items: core.itemCatalog}
	if err := core.initPlayerAndChar(soc.guildRepo, soc.guild, econ.fleamarketRepo, soc.notification, soc.exileRepo, referrals.DeliverReferral, cfg.Player.MaxPlayers, logger); err != nil {
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

	econ.initStore(core, soc.guildRepo, soc.timer, valkeyClient, storeDelivery{econ.depotRepo, soc.home, core.itemCatalog})
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		return nil, err
	}

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
) error {
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
	cmbt.boss.SetVictoryBanquetHook(buildVictoryBanquetHook(misc.eventplaza))
	misc.rescue.SetActivityReader(newContextActivityReaders(cmbt, misc))

	misc.casino.SetGamePlayedHook(func(ctx context.Context, characterID string, gameName string) error {
		return misc.medal.RecordProgress(ctx, characterID, medal.MetricCasinoGames, 1)
	})

	cmbt.adv.SetVictoryHook(buildAdventureVictoryHook(misc.medal.RecordProgress))
	cmbt.party.SetVictoryHook(buildPartyVictoryHook(misc.medal.RecordProgress))

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
			cmbt.battle.SetMonsterDefeatRecorder(battleMonsterDefeatAdapter{svc: misc.collection})
			if cmbt.boss != nil {
				cmbt.boss.SetMonsterDefeatRecorder(bossMonsterDefeatAdapter{svc: misc.collection})
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
		if cmbt.challenge != nil {
			cmbt.challenge.ConfigureFullnessResetter(misc.tavern)
		}
		soc.home.SetFullnessResetter(misc.tavern)
		postAdventureHook := buildPostAdventureHook(misc.tavern)
		if cmbt.adv != nil {
			cmbt.adv.SetPostAdventureHook(postAdventureHook)
		}
		if cmbt.normal != nil {
			cmbt.normal.SetExitEffects(misc.tavern.ResetFullness)
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
		if cmbt.normal != nil {
			cmbt.normal.SetBlessings(chapelBlessingAdapter{svc: misc.chapel})
		}
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
	// Item 058: login message from presence, profile personality and news from the character service.
	soc.home.SetPersonalityRecorder(struct {
		*character.PresenceService
		*character.Service
	}{core.presence, core.charService})
	if misc.job != nil {
		soc.home.SetJobStateRestorer(misc.job)
		soc.home.SetFutureMemorySaver(misc.job)
		if soc.ranking != nil {
			misc.job.SetJobChangeTracker(soc.ranking)
			misc.job.SetJobPopularityTracker(soc.ranking)
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
	if econ.alchemy != nil && soc.notification != nil {
		econ.alchemy.SetNewsPublisher(alchemy.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, pubAt time.Time) error {
			_, err := soc.notification.PublishNews(ctx, cat, title, content, author, pubAt)
			return err
		}))
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
		if err := wireContestSettlement(soc.sched, misc.contest); err != nil {
			return err
		}
	}
	return nil
}

// wireContestSettlement enqueues the contest settlement action if not already scheduled.
func wireContestSettlement(sched *scheduling.Service, contestSvc *contest.Service) error {
	if contestSvc == nil {
		return nil
	}
	if sched != nil {
		contestSvc.SetScheduler(sched)
	}
	return contestSvc.ReconcileSettlementSchedule(context.Background())
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
	if soc.placeLog != nil {
		opts = append(opts, http.WithPlaceLog(soc.placeLog), http.WithCharacterNames(core.charRepo))
	}
	var players http.PlayerService = core.playerService
	if soc.placeMembers != nil {
		opts = append(opts, http.WithPlaceMembers(soc.placeMembers))
		players = placeMemberLogout{PlayerService: core.playerService, characters: core.charRepo, members: soc.placeMembers}
	}
	pc := newPlayerContext(core, soc, econ, cmbt, misc, playercontext.WithActionWait(soc.timer, cfg.ActionWait))
	if soc.schedRepo != nil {
		opts = append(opts, http.WithPlayerContext(pc))
	}
	opts = append(opts,
		http.WithHelper(misc.helper),
		http.WithRescue(misc.rescue),
		http.WithExile(soc.exile),
		http.WithMedal(misc.medal),
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
		http.WithPlayerInspect(playerInspectAdapter{status: econ.auction, guild: soc.guild, skills: cmbt.customSkill, presence: core.presence, location: pc, gems: cmbt.gems, now: time.Now}),
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

	return http.NewHandler(players, core.charService, cmbt.adv, econ.shop, opts...)
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
