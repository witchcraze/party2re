package main

import (
	"context"
	"database/sql"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/altar"
	"github.com/witchcraze/party2re/internal/blackmarket"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/delivery"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/farm"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/helper"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/maintenance"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/secretshop"
	"github.com/witchcraze/party2re/internal/tavern"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

type miscServices struct {
	medal       *medal.Service
	eventplaza  *eventplaza.Service
	secretshop  *secretshop.Service
	rescue      *rescue.Service
	helper      *helper.Service
	job         *job.Service
	chapel      *chapel.Service
	farm        *farm.Service
	collection  *collection.Service
	lottery     *lottery.Service
	tavern      *tavern.Service
	blackmarket *blackmarket.Service
	delivery    *delivery.Service
	casino      *casino.Service
	god         *god.Service
	monster     *monster.Service
	contest     *contest.Service
	altar       *altar.Service
	wishingwell *wishingwell.Service
	maint       *maintenance.Service
	activity    *activity.Service
}

func newMiscServices(
	db *sql.DB,
	core *coreServices,
	soc *socServices,
	econ *econServices,
	valkeyClient valkeygo.Client,
) (*miscServices, error) {
	achievementRepo, err := database.NewAchievementRepository(db)
	if err != nil {
		return nil, err
	}
	medalService, err := medal.NewService(
		core.charRepo,
		econ.depotRepo,
		"",
		medal.WithTransactionProvider(core.txProvider),
		medal.WithAchievementRepository(achievementRepo),
	)
	if err != nil {
		return nil, err
	}

	eventplazaRepo, err := database.NewEventPlazaRepository(db)
	if err != nil {
		return nil, err
	}
	eventplazaService, err := eventplaza.NewService(
		eventplazaRepo,
		core.charRepo,
		core.invRepo,
		eventplaza.WithTransactionProvider(core.txProvider),
	)
	if err != nil {
		return nil, err
	}

	secretshopCatalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		return nil, err
	}
	secretshopService, err := secretshop.NewService(
		core.charRepo,
		core.invRepo,
		secretshopCatalog,
		secretshop.WithTransactionProvider(core.txProvider),
	)
	if err != nil {
		return nil, err
	}

	rescueRepo, err := database.NewRescueRepository(db)
	if err != nil {
		return nil, err
	}
	rescueService := rescue.NewService(rescueRepo, core.charRepo, soc.sched)

	helperRepo, err := database.NewHelperRepository(db)
	if err != nil {
		return nil, err
	}
	helperService := helper.NewService(helperRepo, core.charRepo, core.invRepo, nil, core.txProvider)

	futureMemoryRepo, err := database.NewFutureMemoryRepository(db)
	if err != nil {
		return nil, err
	}
	jobService, err := job.NewService(
		core.charJobRepo,
		job.WithCatalog(core.jobCatalog),
		job.WithCharacterRepository(core.charRepo),
		job.WithInventoryRepository(core.invRepo),
		job.WithEconomy(core.economy),
		job.WithFutureMemoryRepository(futureMemoryRepo),
		job.WithNewsPublisher(job.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, pubAt time.Time) error {
			if soc.notification != nil {
				_, err := soc.notification.PublishNews(ctx, cat, title, content, author, pubAt)
				return err
			}
			return nil
		})),
	)
	if err != nil {
		return nil, err
	}

	chapelRepo, err := database.NewChapelRepository(db)
	if err != nil {
		return nil, err
	}
	chapelService, err := chapel.NewService(chapelRepo)
	if err != nil {
		return nil, err
	}

	farmRepo, err := database.NewFarmRepository(db)
	if err != nil {
		return nil, err
	}
	farmService, err := farm.NewService(farmRepo)
	if err != nil {
		return nil, err
	}

	collectionRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		return nil, err
	}
	collectionService, err := collection.NewService(collectionRepo, 100, 100)
	if err != nil {
		return nil, err
	}

	lotteryRepo, err := database.NewLotteryRepository(db)
	if err != nil {
		return nil, err
	}
	lotteryService, err := lottery.NewService(lotteryRepo)
	if err != nil {
		return nil, err
	}

	tavernCatalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		return nil, err
	}
	tavernRepo, err := database.NewTavernRepository(db)
	if err != nil {
		return nil, err
	}
	tavernService, err := tavern.NewService(
		tavernCatalog,
		tavernRepo,
		core.charRepo,
		core.txProvider,
		tavern.WithLotteryRepository(lotteryRepo),
	)
	if err != nil {
		return nil, err
	}

	blackmarketCatalog, err := blackmarket.LoadDefaultCatalog()
	if err != nil {
		return nil, err
	}
	blackmarketRepo, err := database.NewBlackMarketRepository(db)
	if err != nil {
		return nil, err
	}
	blackmarketService, err := blackmarket.NewService(
		core.charRepo,
		core.invRepo,
		blackmarketRepo,
		blackmarketCatalog,
		blackmarket.WithItemDefinitionProvider(core.itemCatalog),
		blackmarket.WithTransactionProvider(core.txProvider),
	)
	if err != nil {
		return nil, err
	}

	deliveryRepo, err := database.NewDeliveryRepository(db)
	if err != nil {
		return nil, err
	}
	deliveryService, err := delivery.NewService(
		deliveryRepo,
		core.charRepo,
		core.invRepo,
		delivery.WithItemDefinitionProvider(core.itemCatalog),
		delivery.WithTransactionProvider(core.txProvider),
	)
	if err != nil {
		return nil, err
	}

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		return nil, err
	}
	casinoService, err := casino.NewService(
		casinoRepo,
		casino.WithTransactionProvider(core.txProvider),
		casino.WithEconomy(core.economy),
	)
	if err != nil {
		return nil, err
	}

	homeMemberRepo, err := database.NewHomeMemberRepository(db)
	if err != nil {
		return nil, err
	}

	godService, err := god.NewService(
		core.charRepo,
		god.WithDepotRepository(econ.depotRepo),
		god.WithInventoryRepository(core.invRepo),
		god.WithTransactionProvider(core.txProvider),
		god.WithCasinoRepository(casinoRepo),
		god.WithLotteryRepository(lotteryRepo),
		god.WithGuildRepository(soc.guildRepo),
		god.WithHomeMemberRepository(homeMemberRepo),
		god.WithProfileRepository(homeMemberRepo),
		god.WithHomeRepository(homeMemberRepo),
	)
	if err != nil {
		return nil, err
	}

	monsterRepo, err := database.NewMonsterRepository(db)
	if err != nil {
		return nil, err
	}
	monsterService := monster.NewService(
		core.charRepo,
		monsterRepo,
		monster.WithTransactionProvider(core.txProvider),
	)

	contestRepo, err := database.NewContestRepository(db)
	if err != nil {
		return nil, err
	}
	contestService, err := contest.NewService(
		core.charRepo,
		contestRepo,
		contest.WithTransactionProvider(core.txProvider),
		contest.WithNewsPublisher(contest.NewsPublisherFunc(func(ctx context.Context, cat, title, content, author string, pubAt time.Time) error {
			_, err := soc.notification.PublishNews(ctx, cat, title, content, author, pubAt)
			return err
		})),
	)
	if err != nil {
		return nil, err
	}

	valkeyMaintRepo := maintenance.NewValkeyRepository(valkeyClient, maintenance.WithFallback(core.dbMaintRepo))
	maintService, err := maintenance.NewService(valkeyMaintRepo)
	if err != nil {
		return nil, err
	}

	altarRepo, err := database.NewAltarRepository(db)
	if err != nil {
		return nil, err
	}
	altarService, err := altar.NewService(
		core.charRepo,
		core.invRepo,
		econ.depotRepo,
		altarRepo,
		core.itemCatalog,
		core.txProvider,
		altar.WithCollectionRecorder(collectionService),
	)
	if err != nil {
		return nil, err
	}

	wishingwellService, err := wishingwell.NewService(
		core.charRepo,
		wishingwell.WithTransactionProvider(core.txProvider),
	)
	if err != nil {
		return nil, err
	}

	var activityService *activity.Service
	if valkeyClient != nil {
		activityRepo, err := database.NewActivityRepository(db)
		if err != nil {
			return nil, err
		}
		activityService, err = activity.NewService(activityRepo, core.charRepo, soc.sched, nil)
		if err != nil {
			return nil, err
		}
	}

	return &miscServices{
		medal:       medalService,
		eventplaza:  eventplazaService,
		secretshop:  secretshopService,
		rescue:      rescueService,
		helper:      helperService,
		job:         jobService,
		chapel:      chapelService,
		farm:        farmService,
		collection:  collectionService,
		lottery:     lotteryService,
		tavern:      tavernService,
		blackmarket: blackmarketService,
		delivery:    deliveryService,
		casino:      casinoService,
		god:         godService,
		monster:     monsterService,
		contest:     contestService,
		maint:       maintService,
		activity:    activityService,
		altar:       altarService,
		wishingwell: wishingwellService,
	}, nil
}
