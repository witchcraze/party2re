package scheduling

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/logging"
)

type Worker struct {
	repo     core_scheduling.ScheduledActionRepository
	handlers map[string]ActionHandler
	interval time.Duration
	logger   logging.Logger
}

func NewWorker(repo core_scheduling.ScheduledActionRepository, interval time.Duration, logger logging.Logger) *Worker {
	return &Worker{
		repo:     repo,
		handlers: make(map[string]ActionHandler),
		interval: interval,
		logger:   logger,
	}
}

func (w *Worker) RegisterHandler(actionType string, handler ActionHandler) {
	w.handlers[actionType] = handler
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info(ctx, "Starting scheduled action worker", slog.String("interval", w.interval.String()))

	for {
		select {
		case <-ctx.Done():
			w.logger.Info(ctx, "Stopping scheduled action worker")
			return
		case <-ticker.C:
			w.processActions(ctx)
		}
	}
}

func (w *Worker) processActions(ctx context.Context) {
	// Fetch actions due up to now
	actions, err := w.repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		w.logger.Error(ctx, "Failed to fetch due actions", err)
		return
	}

	for _, action := range actions {
		w.processAction(ctx, action)
	}
}

func (w *Worker) processAction(ctx context.Context, action core_scheduling.ScheduledAction) {
	// Attempt to acquire lock to prevent duplicate processing
	acquired, err := w.repo.AcquireLock(ctx, action.ID, 5*time.Minute)
	if err != nil {
		w.logger.Error(ctx, "Failed to acquire lock for action", err, slog.String("action_id", action.ID))
		return
	}

	if !acquired {
		return // Someone else is processing this or it's already processed
	}

	// Lock acquired, mark as processing
	if err := action.MarkProcessing(); err != nil {
		w.logger.Error(ctx, "Failed to mark action as processing", err, slog.String("action_id", action.ID))
		return
	}
	
	if err := w.repo.Save(ctx, action); err != nil {
		w.logger.Error(ctx, "Failed to save action state as processing", err, slog.String("action_id", action.ID))
		return
	}

	// Dispatch to handler
	handler, exists := w.handlers[action.ActionType]
	var handleErr error
	if !exists {
		handleErr = fmt.Errorf("no handler registered for action type: %s", action.ActionType)
	} else {
		handleErr = handler.Handle(ctx, action)
	}

	// Mark outcome
	if handleErr != nil {
		w.logger.Error(ctx, "Failed to process action", handleErr, slog.String("action_id", action.ID), slog.String("type", action.ActionType))
		action.MarkFailed()
	} else {
		action.MarkCompleted(24 * time.Hour) // Retain completed actions for 24 hours
	}

	if err := w.repo.Save(ctx, action); err != nil {
		w.logger.Error(ctx, "Failed to save action final state", err, slog.String("action_id", action.ID))
	}
}
