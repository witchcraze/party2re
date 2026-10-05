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
	actorKeyPrefix  = "party2:scheduled:actor:"
)

// Schedule stores action data, indexes it by ActorID (if non-empty), and adds it
// to the pending queue sorted set. If indexing by ActorID or enqueueing fails,
// any partially written data or index entry is cleaned up on a best-effort basis
// and the error is returned to prevent orphan un-indexed or un-enqueued work.
// Subsequent retries safely overwrite any leftover payload via idempotent SET,
// SADD, and ZADD operations.
func (r *ValkeyRepository) Schedule(ctx context.Context, action core_scheduling.ScheduledAction) error {
	data, err := json.Marshal(action)
	if err != nil {
		return err
	}

	actionKey := actionKeyPrefix + action.ID

	// Save action data
	if err := r.client.Do(ctx, r.client.B().Set().Key(actionKey).Value(string(data)).Build()).Error(); err != nil {
		return err
	}

	// Index by actor ID if present
	if action.ActorID != "" {
		if err := r.client.Do(ctx, r.client.B().Sadd().Key(actorKeyPrefix+action.ActorID).Member(action.ID).Build()).Error(); err != nil {
			r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build())
			return err
		}
	}

	// Add to pending queue sorted set
	score := float64(action.ExecuteAt.Unix())
	if err := r.client.Do(ctx, r.client.B().Zadd().Key(pendingQueueKey).ScoreMember().ScoreMember(score, action.ID).Build()).Error(); err != nil {
		if action.ActorID != "" {
			r.client.Do(ctx, r.client.B().Srem().Key(actorKeyPrefix+action.ActorID).Member(action.ID).Build())
		}
		r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build())
		return err
	}
	return nil
}

// FetchDue queries actions due up to upTo from the pending queue sorted set.
// If an action key is genuinely missing (valkey.IsValkeyNil), the stale entry
// is removed from the pending queue. If reading the payload fails due to a
// transient storage error or context cancellation, the queued entry is preserved
// and the error is returned immediately to the worker. Malformed or invalid
// actions are purged from the queue to prevent repeated processing.
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
			if valkey.IsValkeyNil(err) {
				// Key missing: remove stale queue entry
				if remErr := r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build()).Error(); remErr != nil && !valkey.IsValkeyNil(remErr) {
					return nil, remErr
				}
				continue
			}
			return nil, err
		}

		var action core_scheduling.ScheduledAction
		if err := json.Unmarshal(val, &action); err != nil {
			// Malformed JSON: remove from queue and delete key to prevent re-fetch
			if remErr := r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build()).Error(); remErr != nil && !valkey.IsValkeyNil(remErr) {
				return nil, remErr
			}
			if delErr := r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build()).Error(); delErr != nil && !valkey.IsValkeyNil(delErr) {
				return nil, delErr
			}
			continue
		}

		// Reject actions that fail domain-level invariants (e.g. unknown state,
		// oversized fields). Remove from queue to prevent repeated processing.
		if err := action.Validate(); err != nil {
			if remErr := r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build()).Error(); remErr != nil && !valkey.IsValkeyNil(remErr) {
				return nil, remErr
			}
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

		// Remove from actor index
		if action.ActorID != "" {
			r.client.Do(ctx, r.client.B().Srem().Key(actorKeyPrefix+action.ActorID).Member(action.ID).Build())
		}

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

func (r *ValkeyRepository) CancelByActorID(ctx context.Context, actorID string) (int, error) {
	if actorID == "" {
		return 0, nil
	}
	actorKey := actorKeyPrefix + actorID
	membersResp := r.client.Do(ctx, r.client.B().Smembers().Key(actorKey).Build())
	if membersResp.Error() != nil {
		if valkey.IsValkeyNil(membersResp.Error()) {
			return 0, nil
		}
		return 0, membersResp.Error()
	}

	ids, err := membersResp.AsStrSlice()
	if err != nil {
		return 0, err
	}

	for _, id := range ids {
		actionKey := actionKeyPrefix + id
		lockKey := lockKeyPrefix + id
		_ = r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build())
		_ = r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build())
		_ = r.client.Do(ctx, r.client.B().Del().Key(lockKey).Build())
	}

	_ = r.client.Do(ctx, r.client.B().Del().Key(actorKey).Build())
	return len(ids), nil
}
