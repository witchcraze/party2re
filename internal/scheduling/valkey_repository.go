package scheduling

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

type ValkeyRepository struct {
	client valkey.Client
}

func NewValkeyRepository(client valkey.Client) *ValkeyRepository {
	return &ValkeyRepository{
		client: client,
	}
}

const (
	pendingQueueKey = "party2:scheduled:pending"
	actionKeyPrefix = "party2:scheduled:action:"
	lockKeyPrefix   = "party2:scheduled:lock:"
)

func (r *ValkeyRepository) Schedule(ctx context.Context, action core_scheduling.ScheduledAction) error {
	data, err := json.Marshal(action)
	if err != nil {
		return err
	}

	actionKey := actionKeyPrefix + action.ID

	// Save action data
	err = r.client.Do(ctx, r.client.B().Set().Key(actionKey).Value(string(data)).Build()).Error()
	if err != nil {
		return err
	}

	// Add to pending queue sorted set
	score := float64(action.ExecuteAt.Unix())
	err = r.client.Do(ctx, r.client.B().Zadd().Key(pendingQueueKey).ScoreMember().ScoreMember(score, action.ID).Build()).Error()
	return err
}

func (r *ValkeyRepository) FetchDue(ctx context.Context, upTo time.Time, limit int) ([]core_scheduling.ScheduledAction, error) {
	scoreStr := strconv.FormatInt(upTo.Unix(), 10)

	// Get IDs from pending queue
	cmd := r.client.B().Zrangebyscore().Key(pendingQueueKey).Min("-inf").Max(scoreStr).Limit(0, int64(limit)).Build()
	resp := r.client.Do(ctx, cmd)
	if resp.Error() != nil {
		return nil, resp.Error()
	}

	ids, err := resp.AsStrSlice()
	if err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return nil, nil
	}

	var actions []core_scheduling.ScheduledAction

	for _, id := range ids {
		actionKey := actionKeyPrefix + id
		val, err := r.client.Do(ctx, r.client.B().Get().Key(actionKey).Build()).AsBytes()
		if err != nil {
			// Key missing: remove stale queue entry
			r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build())
			continue
		}

		var action core_scheduling.ScheduledAction
		if err := json.Unmarshal(val, &action); err != nil {
			// Malformed JSON: remove from queue and delete key to prevent re-fetch
			r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build())
			r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build())
			continue
		}

		// Reject actions that fail domain-level invariants (e.g. unknown state,
		// oversized fields). Remove from queue to prevent repeated processing.
		if err := action.Validate(); err != nil {
			r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build())
			continue
		}

		actions = append(actions, action)
	}

	return actions, nil
}

func (r *ValkeyRepository) AcquireLock(ctx context.Context, actionID string, lockTTL time.Duration) (bool, error) {
	lockKey := lockKeyPrefix + actionID
	ttlSecs := int64(lockTTL.Seconds())
	if ttlSecs < 1 {
		ttlSecs = 1
	}

	resp := r.client.Do(ctx, r.client.B().Set().Key(lockKey).Value("1").Nx().ExSeconds(ttlSecs).Build())
	if resp.Error() != nil {
		if valkey.IsValkeyNil(resp.Error()) {
			return false, nil
		}
		return false, resp.Error()
	}
	return true, nil
}

func (r *ValkeyRepository) Save(ctx context.Context, action core_scheduling.ScheduledAction) error {
	data, err := json.Marshal(action)
	if err != nil {
		return err
	}

	actionKey := actionKeyPrefix + action.ID

	if action.State == core_scheduling.StateCompleted || action.State == core_scheduling.StateFailed {
		// Remove from pending queue
		r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(action.ID).Build())

		// Delete lock
		r.client.Do(ctx, r.client.B().Del().Key(lockKeyPrefix+action.ID).Build())

		// Save updated action with TTL based on RetainUntil
		if !action.RetainUntil.IsZero() {
			ttlSecs := int64(time.Until(action.RetainUntil).Seconds())
			if ttlSecs > 0 {
				return r.client.Do(ctx, r.client.B().Set().Key(actionKey).Value(string(data)).ExSeconds(ttlSecs).Build()).Error()
			}
			// If TTL is negative, just delete it immediately
			return r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build()).Error()
		}
	}

	// Just update the action data without TTL
	return r.client.Do(ctx, r.client.B().Set().Key(actionKey).Value(string(data)).Build()).Error()
}
