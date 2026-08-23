package main

import (
	"context"
	"os"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/logging"
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

	return nil
}
