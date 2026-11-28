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
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
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
	home.WithBaseSleepDuration(cfg.Home.SleepBase)(soc.home)
	soc.placeLog = placelog.New(valkeyClient, cfg.PlaceLog)
	soc.placeMembers = placelog.NewMembers(valkeyClient, cfg.PlaceMemberTTL)
	referrals := referralDeliveryAdapter{depots: econ.depotRepo, home: soc.home, items: core.itemCatalog}
	adventureRecords, err := database.NewAdventureRepository(db)
	if err != nil {
		return nil, err
	}
	if err := core.initPlayerAndChar(soc.guildRepo, soc.guild, econ.fleamarketRepo, soc.notification, soc.exileRepo, referrals.DeliverReferral, imageUnlocks{adventureRecords}, cfg.Player.MaxPlayers, logger); err != nil {
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
	wireHooks(core, cmbt, econ, misc, soc)
	if err := registerWorkers(soc, cmbt, misc); err != nil {
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
) {
	cmbt.boss.SetBanquet(buildVictoryBanquetHook(misc.eventplaza))
	misc.rescue.SetActivityReader(newContextActivityReaders(cmbt, misc))

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
			if cmbt.dungeon != nil {
				cmbt.dungeon.SetBlessingProvider(chapelBlessingAdapter{misc.chapel})
			}
		}
		if misc.collection != nil {
			cmbt.battle.SetMonsterDefeatRecorder(battleMonsterDefeatAdapter{svc: misc.collection})
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
		if cmbt.normal != nil {
			cmbt.normal.SetExitEffects(misc.tavern.ResetFullness)
		}
		if cmbt.boss != nil {
			cmbt.boss.SetExitEffects(misc.tavern.ResetFullness)
		}
		if cmbt.party != nil {
			cmbt.party.SetTavernExitSettler(partyTavernExitAdapter{tavern: misc.tavern})
		}
		if cmbt.gvg != nil {
			cmbt.gvg.SetPostExitHook(buildRoomExitHook(misc.tavern, func(itemName string, paid bool) *gvg.DeliveryOutcome {
				return &gvg.DeliveryOutcome{ItemName: itemName, Paid: paid}
			}))
		}
		if cmbt.pvp != nil {
			cmbt.pvp.SetPostExitHook(buildRoomExitHook(misc.tavern, func(itemName string, paid bool) *pvp.DeliveryOutcome {
				return &pvp.DeliveryOutcome{ItemName: itemName, Paid: paid}
			}))
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
		if cmbt.boss != nil {
			cmbt.boss.SetBlessings(chapelBlessingAdapter{svc: misc.chapel})
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
	if misc.casino != nil {
		misc.casino.SetActionWait(pc)
	}
	if cmbt.pvp != nil {
		cmbt.pvp.SetActionWait(pc)
	}
	if cmbt.party != nil {
		cmbt.party.SetActionWait(pc)
	}
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
		http.WithCollection(misc.collection, core.itemCatalog),
		http.WithDepot(econ.depot),
		http.WithBank(econ.bank),
		http.WithLottery(misc.lottery),
		http.WithCasino(misc.casino),
		http.WithChallenge(cmbt.challenge),
		http.WithDungeon(cmbt.dungeon),
		http.WithPvP(cmbt.pvp),
		http.WithGvG(cmbt.gvg),
		http.WithReplay(cmbt.replay),
		http.WithPlayerInspect(playerInspectAdapter{status: character.Inspector{Characters: core.charRepo, Equipment: econ.equipRepo, Inventory: core.invRepo, Items: core.itemCatalog}, guild: soc.guild, skills: cmbt.customSkill, presence: core.presence, location: pc, gems: cmbt.gems, now: time.Now}),
		http.WithNotification(soc.notification),
		http.WithHome(soc.home),
		http.WithCustomSkill(cmbt.customSkill),
		http.WithHomeImage(core.charService),
		http.WithEventPlaza(misc.eventplaza),
		http.WithSecretShop(misc.secretshop),
		http.WithTavern(misc.tavern),
		http.WithAlchemy(econ.alchemy),
		http.WithAdventureRecord(cmbt.record),
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
		http.WithQuestStages(cmbt.stages),
		http.WithQuestRuns(cmbt.normal),
		http.WithNormalRuns(cmbt.normal),
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
