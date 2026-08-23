package main

import (
	"context"
	"os"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/valkey"
)

func main() {
	logger := logging.NewJSON(os.Stderr)
	if err := run(); err != nil {
		logger.Error(context.Background(), "application.startup", err)
		os.Exit(1)
	}
	logger.Info(context.Background(), "application.ready")
}

func run() error {
	db, err := database.OpenFromEnvironment()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Ping(db); err != nil {
		return err
	}

	valkeyClient, err := valkey.NewClient()
	if err != nil {
		// Log warning but continue if Valkey is optional or fallback is acceptable.
		// For now, if we can't connect, we just won't have a scheduler.
	} else {
		defer valkeyClient.Close()

		// Setup repositories
		charRepo, err := database.NewCharacterRepository(db)
		if err != nil {
			return err
		}
		activityRepo, err := database.NewActivityRepository(db)
		if err != nil {
			return err
		}
		schedRepo := scheduling.NewValkeyRepository(valkeyClient)

		// Setup Scheduler & Worker
		schedService := scheduling.NewService(schedRepo)

		// Note: logger parameter uses nop logger for now as standard pkg logger isn't typed for it.
		// In a real app we would adapt logging.Logger to activity.Logger.
		activityService, err := activity.NewService(activityRepo, charRepo, schedService, nil)
		if err != nil {
			return err
		}

		worker := scheduling.NewWorker(schedRepo, 5*time.Second, logging.NewJSON(os.Stderr))
		worker.RegisterHandler(activity.ActivityActionTypeTrainingComplete, activity.NewTrainingHandler(activityService))

		// In a real entrypoint we would run worker.Run(ctx, interval) in a goroutine.
		// For now, we just wire it up.
	}

	return nil
}
