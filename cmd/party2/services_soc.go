package main

import (
	"context"
	"database/sql"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/inventory"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/ratelimit"
	"github.com/witchcraze/party2re/internal/scheduling"
)

type depotManagerAdapter struct {
	repo *database.DepotRepository
}

func (a *depotManagerAdapter) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	return a.repo.FindByCharacterID(ctx, characterID)
}

func (a *depotManagerAdapter) Consume(ctx context.Context, characterID, itemInstanceID string, quantity int) error {
	dp, err := a.repo.FindByCharacterIDForUpdate(ctx, characterID)
	if err != nil {
		return err
	}
	if _, err := dp.Consume(itemInstanceID, quantity); err != nil {
		return err
	}
	return a.repo.Save(ctx, dp)
}

func (a *depotManagerAdapter) ConsumeOne(ctx context.Context, characterID, itemInstanceID string) error {
	return a.Consume(ctx, characterID, itemInstanceID, 1)
}

type socServices struct {
	guildRepo    *database.GuildRepository
	guild        *guild.Service
	ranking      *ranking.Service
	park         *park.Service
	home         *home.Service
	notification *notification.Service
	sched        *scheduling.Service
	limiter      http.RateLimiter
	worker       *scheduling.Worker
	timer        timer.Service
}

func newSocServices(
	db *sql.DB,
	core *coreServices,
	econ *econServices,
	valkeyClient valkeygo.Client,
	logger logging.Logger,
) (*socServices, error) {
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		return nil, err
	}
	guildService, err := guild.NewService(guildRepo)
	if err != nil {
		return nil, err
	}

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		return nil, err
	}

	parkRepo, err := database.NewParkRepository(db)
	if err != nil {
		return nil, err
	}

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		return nil, err
	}

	notificationRepo, err := database.NewNotificationRepository(db)
	if err != nil {
		return nil, err
	}
	notificationService, err := notification.NewService(notificationRepo, notificationRepo)
	if err != nil {
		return nil, err
	}

	var (
		limiter        http.RateLimiter = ratelimit.NewMemoryLimiter()
		schedService   *scheduling.Service
		worker         *scheduling.Worker
		rankingService *ranking.Service
	)

	if valkeyClient == nil {
		rankingService, _ = ranking.NewService(rankingRepo)
	} else {
		limiter = ratelimit.NewValkeyLimiter(valkeyClient)
		rankingCache := ranking.NewValkeySnapshotCache(valkeyClient)
		schedRepo := scheduling.NewValkeyRepository(valkeyClient)
		schedService = scheduling.NewService(schedRepo)
		rankingService, _ = ranking.NewService(rankingRepo, ranking.WithSnapshotCache(rankingCache))
		worker = scheduling.NewWorker(schedRepo, 5*time.Second, logger)
	}

	timerService := timer.NewService(valkeyClient)
	parkService, _ := park.NewService(parkRepo, core.charRepo, park.WithRateLimiter(limiter))

	invService, _ := inventory.NewService(core.invRepo)
	var depotMgr home.DepotManager
	if econ != nil && econ.depotRepo != nil {
		depotMgr = &depotManagerAdapter{repo: econ.depotRepo}
	}
	homeService, _ := home.NewService(
		homeRepo,
		core.charRepo,
		home.WithTimer(timerService),
		home.WithCharacterUpdater(core.charRepo),
		home.WithInventoryManager(invService),
		home.WithDepotManager(depotMgr),
		home.WithItemCatalog(core.itemCatalog),
		home.WithEconomy(core.economy),
	)

	return &socServices{
		guildRepo:    guildRepo,
		guild:        guildService,
		ranking:      rankingService,
		park:         parkService,
		home:         homeService,
		notification: notificationService,
		sched:        schedService,
		limiter:      limiter,
		worker:       worker,
		timer:        timerService,
	}, nil
}

func (s *socServices) registerWorkerHandlers(activityService *activity.Service, advService *adventure.Service, chapelService *chapel.Service) {
	if s.worker != nil {
		if activityService != nil {
			s.worker.RegisterHandler(activity.ActivityActionTypeTrainingComplete, activity.NewTrainingHandler(activityService))
		}
		if advService != nil {
			s.worker.RegisterHandler(adventure.AdventureActionTypeComplete, adventure.NewAdventureCompletionHandler(advService))
		}
		s.worker.RegisterHandler(ranking.RankingActionTypeRefresh, ranking.NewRefreshHandler(s.ranking))
		if chapelService != nil {
			s.worker.RegisterHandler(chapel.ActionTypeChapelReset, chapel.NewResetHandler(chapelService, chapel.WithScheduler(s.sched)))
		}
	}
}
