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
// It traverses the queue in bounded batches, excluding preserved Processing
// records from execution candidates while leaving their authoritative storage and
// actor discovery intact.
// If an action key is genuinely missing (valkey.IsValkeyNil), the stale entry
// is removed from the pending queue. If reading the payload fails due to a
// transient storage error or context cancellation, the queued entry is preserved
// and the error is returned immediately to the worker. Malformed or invalid
// actions are purged from the queue to prevent repeated processing.
// Valid terminal records are returned for metadata-only worker finalization.
func (r *ValkeyRepository) FetchDue(ctx context.Context, upTo time.Time, limit int) ([]core_scheduling.ScheduledAction, error) {
	if limit <= 0 {
		return nil, nil
	}

	scoreStr := strconv.FormatInt(upTo.Unix(), 10)
	batchSize := int64(limit)
	if batchSize < 50 {
		batchSize = 50
	}
	maxScan := int64(limit) * 20
	if maxScan < 1000 {
		maxScan = 1000
	}

	var actions []core_scheduling.ScheduledAction
	offset := int64(0)
	totalScanned := int64(0)

	for totalScanned < maxScan && len(actions) < limit {
		toFetch := batchSize
		if totalScanned+toFetch > maxScan {
			toFetch = maxScan - totalScanned
		}
		if toFetch <= 0 {
			break
		}

		cmd := r.client.B().Zrangebyscore().Key(pendingQueueKey).Min("-inf").Max(scoreStr).Limit(offset, toFetch).Build()
		resp := r.client.Do(ctx, cmd)
		if resp.Error() != nil {
			return nil, resp.Error()
		}

		ids, err := resp.AsStrSlice()
		if err != nil {
			return nil, err
		}

		if len(ids) == 0 {
			break
		}

		retainedInBatch := int64(0)
		for _, id := range ids {
			totalScanned++
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

			retainedInBatch++

			// Processing records represent uncertain outcomes that must not be replayed.
			// They are preserved in the queue and actor index, but excluded from execution candidates.
			if action.State == core_scheduling.StateProcessing {
				continue
			}

			actions = append(actions, action)
			if len(actions) == limit {
				break
			}
		}

		// Adjust offset by the entries that were retained in the queue.
		offset += retainedInBatch

		// If fewer entries than requested were returned, the due queue is exhausted.
		if int64(len(ids)) < toFetch {
			break
		}
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

// Save persists a terminal outcome before cleanup. Cleanup stops on the first
// error and removes queue membership last, allowing metadata-only worker retries.
// Failed payload writes leave unfinished discovery and coordination untouched.
func (r *ValkeyRepository) Save(ctx context.Context, action core_scheduling.ScheduledAction) error {
	data, err := json.Marshal(action)
	if err != nil {
		return err
	}

	actionKey := actionKeyPrefix + action.ID

	if action.State == core_scheduling.StateCompleted || action.State == core_scheduling.StateFailed {
		// Even expired outcomes must replace Processing before destructive cleanup.
		// Otherwise a failed DEL could leave unfinished work looking settled.
		cmd := r.client.B().Set().Key(actionKey).Value(string(data)).Build()
		expired := false
		if !action.RetainUntil.IsZero() {
			ttlSecs := int64(time.Until(action.RetainUntil).Seconds())
			if ttlSecs > 0 {
				cmd = r.client.B().Set().Key(actionKey).Value(string(data)).ExSeconds(ttlSecs).Build()
			} else {
				expired = true
			}
		}
		if err := r.client.Do(ctx, cmd).Error(); err != nil {
			return err
		}
		if err := r.client.Do(ctx, r.client.B().Del().Key(lockKeyPrefix+action.ID).Build()).Error(); err != nil {
			return err
		}
		if action.ActorID != "" {
			if err := r.client.Do(ctx, r.client.B().Srem().Key(actorKeyPrefix+action.ActorID).Member(action.ID).Build()).Error(); err != nil {
				return err
			}
		}
		if expired {
			if err := r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build()).Error(); err != nil {
				return err
			}
		}
		return r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(action.ID).Build()).Error()
	}

	// Just update the action data without TTL
	return r.client.Do(ctx, r.client.B().Set().Key(actionKey).Value(string(data)).Build()).Error()
}

// CancelByActorID cancels and removes all scheduled actions for the specified actor.
// If removing any action from the pending queue or deleting action/lock payloads fails,
// dependent destructive cleanup halts immediately, preserving the actor index and
// remaining entries so unresolved work remains discoverable for recovery.
// Returns the number of successfully removed actions and any storage error encountered.
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
	if len(ids) == 0 {
		return 0, nil
	}

	cancelled := 0
	for _, id := range ids {
		actionKey := actionKeyPrefix + id
		lockKey := lockKeyPrefix + id

		if err := r.client.Do(ctx, r.client.B().Zrem().Key(pendingQueueKey).Member(id).Build()).Error(); err != nil && !valkey.IsValkeyNil(err) {
			return cancelled, err
		}
		if err := r.client.Do(ctx, r.client.B().Del().Key(actionKey).Build()).Error(); err != nil && !valkey.IsValkeyNil(err) {
			return cancelled, err
		}
		if err := r.client.Do(ctx, r.client.B().Del().Key(lockKey).Build()).Error(); err != nil && !valkey.IsValkeyNil(err) {
			return cancelled, err
		}
		cancelled++
	}

	if err := r.client.Do(ctx, r.client.B().Del().Key(actorKey).Build()).Error(); err != nil && !valkey.IsValkeyNil(err) {
		return cancelled, err
	}
	return cancelled, nil
}
