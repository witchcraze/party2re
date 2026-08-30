package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	nethttp "net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/collection"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/custom_skill"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/farm"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/helper"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/inn"
	"github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/ratelimit"
	"github.com/witchcraze/party2re/internal/replay"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/shop"
	"github.com/witchcraze/party2re/internal/valkey"
)

func main() {
	logger := logging.NewJSON(os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error(context.Background(), "application.startup", err)
		os.Exit(1)
	}
}

func resolveServerAddr() string {
	if addr := os.Getenv("PARTY2_ADDR"); addr != "" {
		return addr
	}
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr
	}
	port := os.Getenv("PARTY2_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		return ":" + port
	}
	return port
}

func run(ctx context.Context, logger logging.Logger) error {
	if logger == nil {
		logger = logging.Nop()
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Ping(db); err != nil {
		return err
	}

	// 1. Core Player & Character Repositories and Services
	playerRepo, err := database.NewPlayerRepository(db)
	if err != nil {
		return err
	}
	sessionRepo, err := database.NewSessionRepository(db)
	if err != nil {
		return err
	}
	playerService, err := player.NewService(playerRepo, sessionRepo)
	if err != nil {
		return err
	}

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		return err
	}
	charService, err := character.NewService(charRepo)
	if err != nil {
		return err
	}

	invRepo, err := database.NewInventoryRepository(db)
	if err != nil {
		return err
	}
	shopRepo, err := database.NewShopRepository(db)
	if err != nil {
		return err
	}
	itemCatalog, err := coreitem.InitialCatalog()
	if err != nil {
		return err
	}
	shopService, err := shop.NewServiceWithTransaction(charRepo, invRepo, shopRepo, itemCatalog)
	if err != nil {
		return err
	}

	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		return err
	}
	_, err = depot.NewServiceWithTransaction(depotRepo, charRepo, invRepo, depotRepo)
	if err != nil {
		return err
	}

	bsRepo, err := database.NewBlacksmithRepository(db)
	if err != nil {
		return err
	}
	_, err = blacksmith.NewServiceWithTransaction(charRepo, invRepo, bsRepo, itemCatalog, nil)
	if err != nil {
		return err
	}

	recipeCatalog, err := alchemy.InitialRecipeCatalog()
	if err != nil {
		return err
	}
	alcRepo, err := database.NewAlchemyRepository(db)
	if err != nil {
		return err
	}
	_, err = alchemy.NewServiceWithTransaction(charRepo, invRepo, alcRepo, recipeCatalog, itemCatalog)
	if err != nil {
		return err
	}

	bankRepo, err := database.NewBankRepository(db)
	if err != nil {
		return err
	}
	_, err = bank.NewService(bankRepo)
	if err != nil {
		return err
	}

	pvpRepo, err := database.NewPvPRepository(db)
	if err != nil {
		return err
	}
	pvpService, err := pvp.NewService(pvpRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		return err
	}
	_, err = guild.NewService(guildRepo)
	if err != nil {
		return err
	}

	gvgRepo, err := database.NewGvGRepository(db)
	if err != nil {
		return err
	}
	_, err = gvg.NewService(gvgRepo, guildRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}

	bossRepo, err := database.NewBossRepository(db)
	if err != nil {
		return err
	}
	bossService, err := boss.NewService(bossRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}

	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		return err
	}
	dungeonService, err := dungeon.NewService(dungeonRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}

	replayRepo, err := database.NewReplayRepository(db)
	if err != nil {
		return err
	}
	_, err = replay.NewService(replayRepo)
	if err != nil {
		return err
	}

	challengeRepo, err := database.NewChallengeRepository(db)
	if err != nil {
		return err
	}
	challengeService, err := challenge.NewService(challengeRepo, charRepo, corebattle.Engine{})
	if err != nil {
		return err
	}

	customSkillRepo, err := database.NewCustomSkillRepository(db)
	if err != nil {
		return err
	}
	charJobRepo, err := database.NewCharacterJobRepository(db)
	if err != nil {
		return err
	}
	customSkillService, err := custom_skill.NewService(customSkillRepo, charRepo, charJobRepo)
	if err != nil {
		return err
	}

	parkRepo, err := database.NewParkRepository(db)
	if err != nil {
		return err
	}

	txProvider := database.NewTransactionProvider(db)
	medalService, err := medal.NewService(charRepo, invRepo, nil, "", medal.WithTransactionProvider(txProvider))
	if err != nil {
		return err
	}

	eventplazaRepo, err := database.NewEventPlazaRepository(db)
	if err != nil {
		return err
	}
	eventplazaService, err := eventplaza.NewService(eventplazaRepo, charRepo, invRepo, eventplaza.WithTransactionProvider(txProvider))
	if err != nil {
		return err
	}
	bossService.SetVictoryBanquetHook(func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error {
		_, hookErr := eventplazaService.RecordVictoryBanquet(ctx, bossID, bossName, slayerID, slayerName, tier)
		return hookErr
	})

	rescueRepo, err := database.NewRescueRepository(db)
	if err != nil {
		return err
	}

	helperRepo, err := database.NewHelperRepository(db)
	if err != nil {
		return err
	}

	notificationRepo, err := database.NewNotificationRepository(db)
	if err != nil {
		return err
	}
	notificationService, err := notification.NewService(notificationRepo, notificationRepo)
	if err != nil {
		return err
	}

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		return err
	}

	rankingRepo, err := database.NewRankingRepository(db)
	if err != nil {
		return err
	}

	jobCatalog, err := corejob.InitialCatalog()
	if err != nil {
		return err
	}
	jobService, err := job.NewService(charJobRepo, job.WithCatalog(jobCatalog), job.WithCharacterRepository(charRepo))
	if err != nil {
		return err
	}

	innService, err := inn.NewService(charRepo)
	if err != nil {
		return err
	}

	chapelRepo, err := database.NewChapelRepository(db)
	if err != nil {
		return err
	}
	chapelService, err := chapel.NewService(chapelRepo)
	if err != nil {
		return err
	}

	farmRepo, err := database.NewFarmRepository(db)
	if err != nil {
		return err
	}
	farmService, err := farm.NewService(farmRepo)
	if err != nil {
		return err
	}

	collectionRepo, err := database.NewCollectionRepository(db)
	if err != nil {
		return err
	}
	collectionService, err := collection.NewService(collectionRepo, 100, 100)
	if err != nil {
		return err
	}

	lotteryRepo, err := database.NewLotteryRepository(db)
	if err != nil {
		return err
	}
	lotteryService, err := lottery.NewService(lotteryRepo)
	if err != nil {
		return err
	}

	casinoRepo, err := database.NewCasinoRepository(db)
	if err != nil {
		return err
	}
	casinoService, err := casino.NewService(casinoRepo)
	if err != nil {
		return err
	}

	auctionRepo, err := database.NewAuctionRepository(db)
	if err != nil {
		return err
	}
	auctionService, err := auction.NewService(auctionRepo)
	if err != nil {
		return err
	}

	activityRepo, err := database.NewActivityRepository(db)
	if err != nil {
		return err
	}
	adventureRepo, err := database.NewAdventureRepository(db)
	if err != nil {
		return err
	}
	adventureStages, err := adventure.InitialStageCatalog()
	if err != nil {
		return err
	}
	adventureMonsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		return err
	}

	// 2. Cache, Limiter, Scheduler & Worker (with in-memory fallback)
	var (
		limiter        http.RateLimiter = ratelimit.NewMemoryLimiter()
		schedService   *scheduling.Service
		worker         *scheduling.Worker
		rankingService *ranking.Service
		parkService    *park.Service
		homeService    *home.Service
		advService     *adventure.Service
		rescueService  = rescue.NewService(rescueRepo, charRepo, nil)
		helperService  = helper.NewService(helperRepo, charRepo, invRepo, nil, txProvider)
	)

	valkeyClient, err := valkey.NewClient()
	if err != nil {
		logger.Warn(ctx, "valkey.connect.failed", slog.String("detail", err.Error()), slog.String("fallback", "in_memory"))
		rankingService, _ = ranking.NewService(rankingRepo)
		parkService, _ = park.NewService(parkRepo, charRepo, park.WithRateLimiter(limiter))
		homeService, _ = home.NewService(homeRepo, charRepo, home.WithVisitorLimiter(limiter, 24*time.Hour))
		advService, err = adventure.NewServiceWithCatalogs(adventureRepo, charRepo, invRepo, adventureStages, adventureMonsters, corebattle.Engine{}, nil, nil, adventure.RealClock{})
		if err != nil {
			return err
		}
	} else {
		defer valkeyClient.Close()
		limiter = ratelimit.NewValkeyLimiter(valkeyClient)
		rankingCache := ranking.NewValkeySnapshotCache(valkeyClient)
		schedRepo := scheduling.NewValkeyRepository(valkeyClient)
		schedService = scheduling.NewService(schedRepo)

		rescueService = rescue.NewService(rescueRepo, charRepo, schedService)
		rankingService, _ = ranking.NewService(rankingRepo, ranking.WithSnapshotCache(rankingCache))
		parkService, _ = park.NewService(parkRepo, charRepo, park.WithRateLimiter(limiter))
		homeService, _ = home.NewService(homeRepo, charRepo, home.WithVisitorLimiter(limiter, 24*time.Hour))

		activityService, err := activity.NewService(activityRepo, charRepo, schedService, nil)
		if err != nil {
			return err
		}

		advService, err = adventure.NewServiceWithCatalogs(adventureRepo, charRepo, invRepo, adventureStages, adventureMonsters, corebattle.Engine{}, schedService, nil, adventure.RealClock{})
		if err != nil {
			return err
		}

		worker = scheduling.NewWorker(schedRepo, 5*time.Second, logger)
		worker.RegisterHandler(activity.ActivityActionTypeTrainingComplete, activity.NewTrainingHandler(activityService))
		worker.RegisterHandler(adventure.AdventureActionTypeComplete, adventure.NewAdventureCompletionHandler(advService))
		worker.RegisterHandler(ranking.RankingActionTypeRefresh, ranking.NewRefreshHandler(rankingService))
	}

	// 3. HTTP API Handler construction
	opts := []http.Option{
		http.WithRateLimiter(limiter),
		http.WithAdminAPIKeyFromEnv("PARTY2_ADMIN_API_KEY"),
		http.WithAllowedOriginsFromEnv("PARTY2_CORS_ORIGINS"),
		http.WithHelper(helperService),
		http.WithRescue(rescueService),
		http.WithMedal(medalService),
		http.WithPark(parkService),
		http.WithRanking(rankingService),
		http.WithJob(jobService),
		http.WithInn(innService),
		http.WithChapel(chapelService),
		http.WithFarm(farmService),
		http.WithCollection(collectionService),
		http.WithLottery(lotteryService),
		http.WithCasino(casinoService),
		http.WithChallenge(challengeService),
		http.WithBoss(bossService),
		http.WithDungeon(dungeonService),
		http.WithPvP(pvpService),
		http.WithAuction(auctionService),
		http.WithNotification(notificationService),
		http.WithHome(homeService),
		http.WithCustomSkill(customSkillService),
		http.WithEventPlaza(eventplazaService),
	}

	apiHandler, err := http.NewHandler(playerService, charService, advService, shopService, opts...)
	if err != nil {
		return err
	}

	// 4. Server Binding & Lifecycle Orchestration
	addr := resolveServerAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	defer ln.Close()

	server := &nethttp.Server{
		Addr:              ln.Addr().String(),
		Handler:           apiHandler.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	var workerWg sync.WaitGroup
	if worker != nil {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			worker.Run(workerCtx)
		}()
	}

	logger.Info(ctx, "server.ready", slog.String("addr", ln.Addr().String()))

	select {
	case <-ctx.Done():
		logger.Info(context.Background(), "server.shutdown.started", slog.String("reason", "signal"))
	case err := <-serverErr:
		if err != nil {
			cancelWorker()
			workerWg.Wait()
			return fmt.Errorf("server error: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error(context.Background(), "server.shutdown.error", err)
	}
	cancelWorker()
	workerWg.Wait()
	logger.Info(context.Background(), "server.shutdown.completed")

	return nil
}
