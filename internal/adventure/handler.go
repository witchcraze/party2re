package adventure

import (
	"context"
	"errors"
	"fmt"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

// AdventureCompletionHandler implements scheduling.ActionHandler.
type AdventureCompletionHandler struct {
	service *Service
}

func NewAdventureCompletionHandler(service *Service) *AdventureCompletionHandler {
	return &AdventureCompletionHandler{
		service: service,
	}
}

func (h *AdventureCompletionHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	adventureID, ok := action.Params["adventure_id"]
	if !ok || adventureID == "" {
		return errors.New("missing adventure_id param")
	}

	_, err := h.service.Claim(ctx, adventureID)
	if err != nil {
		if errors.Is(err, ErrAlreadyClaimed) {
			// Idempotent: already handled
			return nil
		}
		return fmt.Errorf("claim adventure: %w", err)
	}

	return nil
}
