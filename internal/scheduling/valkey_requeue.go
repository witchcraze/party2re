package scheduling

import (
	"context"

	core "github.com/witchcraze/party2re/internal/core/scheduling"
)

// RequeuePending repairs only the queue index; a stale snapshot cannot restore payload state.
func (r *ValkeyRepository) RequeuePending(ctx context.Context, action core.ScheduledAction) error {
	if err := action.Validate(); err != nil {
		return err
	}
	if action.State != core.StatePending {
		return nil
	}
	return r.client.Do(ctx, r.client.B().Zadd().Key(pendingQueueKey).Nx().ScoreMember().ScoreMember(float64(action.ExecuteAt.Unix()), action.ID).Build()).Error()
}
