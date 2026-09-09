package main

import (
	"database/sql"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/ratelimit"
	"github.com/witchcraze/party2re/internal/scheduling"
)

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
}

func newSocServices(
	db *sql.DB,
	core *coreServices,
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
	homeService, _ := home.NewService(
		homeRepo,
		core.charRepo,
		home.WithVisitorLimiter(limiter, 24*time.Hour),
		home.WithTimer(timerService),
		home.WithCharacterUpdater(core.charRepo),
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
