package scheduling

import (
	"context"
	"time"

	"github.com/google/uuid"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

// Service provides methods to enqueue and manage scheduled actions.
type Service struct {
	repo core_scheduling.ScheduledActionRepository
}

func NewService(repo core_scheduling.ScheduledActionRepository) *Service {
	return &Service{
		repo: repo,
	}
}

// Schedule adds a new action to be executed at a specific time.
func (s *Service) Schedule(ctx context.Context, actionType, actorID string, params map[string]string, executeAt time.Time) (string, error) {
	id := uuid.New().String()
	action := core_scheduling.ScheduledAction{
		ID:          id,
		ActionType:  actionType,
		ActorID:     actorID,
		Params:      params,
		ScheduledAt: time.Now(),
		ExecuteAt:   executeAt,
		State:       core_scheduling.StatePending,
	}

	err := s.repo.Schedule(ctx, action)
	if err != nil {
		return "", err
	}

	return id, nil
}

// CancelByActorID cancels and removes all scheduled actions for the specified actor.
func (s *Service) CancelByActorID(ctx context.Context, actorID string) error {
	return s.repo.CancelByActorID(ctx, actorID)
}

// ClearActiveActions clears all active and pending scheduled actions for the character,
// fulfilling the rescue.ActionCleaner interface.
func (s *Service) ClearActiveActions(ctx context.Context, characterID string) error {
	return s.CancelByActorID(ctx, characterID)
}
