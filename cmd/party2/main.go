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
	"sync"
	"syscall"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/logging"
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

func run(ctx context.Context, logger logging.Logger) error {
	cfg, err := ConfigFromEnv()
	if err != nil {
		return err
	}
	return runWithConfig(ctx, cfg, logger)
}

func runWithConfig(ctx context.Context, cfg Config, logger logging.Logger) error {
	if logger == nil {
		logger = logging.Nop()
	}

	db, err := database.OpenWithConfig(cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Ping(db); err != nil {
		return err
	}
	logger.Info(ctx, "database.connected", slog.Int("max_open_conns", db.Stats().MaxOpenConnections))

	valkeyClient, err := valkey.NewClientWithConfig(cfg.Valkey)
	if err != nil {
		logger.Warn(ctx, "valkey.connect.failed", slog.String("detail", err.Error()), slog.String("fallback", "in_memory"))
		valkeyClient = nil
	} else {
		defer valkeyClient.Close()
	}

	// 1. Wire all application services, hooks, and HTTP handler
	app, err := wireApp(db, valkeyClient, cfg, logger)
	if err != nil {
		return err
	}

	// 2. Server Binding & Lifecycle Orchestration
	ln, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Server.Addr, err)
	}
	defer ln.Close()

	server := &nethttp.Server{
		Addr:              ln.Addr().String(),
		Handler:           app.handler.Router(),
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
	if app.worker != nil {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			app.worker.Run(workerCtx)
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
