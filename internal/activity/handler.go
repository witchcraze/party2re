package activity

import (
	"context"
	"errors"
	"fmt"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

// TrainingHandler implements scheduling.ActionHandler.
type TrainingHandler struct {
	service *Service
}

func NewTrainingHandler(service *Service) *TrainingHandler {
	return &TrainingHandler{
		service: service,
	}
}

func (h *TrainingHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	activityID, ok := action.Params["activity_id"]
	if !ok || activityID == "" {
		return errors.New("missing activity_id param")
	}

	_, err := h.service.Claim(ctx, activityID)
	if err != nil {
		if errors.Is(err, ErrAlreadyClaimed) {
			// Idempotent: already handled
			return nil
		}
		return fmt.Errorf("claim training: %w", err)
	}

	return nil
}
