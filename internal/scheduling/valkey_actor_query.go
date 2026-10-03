package scheduling

import (
	"context"
	"encoding/json"
	"fmt"

	core "github.com/witchcraze/party2re/internal/core/scheduling"
)

// FindPendingByActorID reads the existing actor Set and bulk-loads its payloads.
// Pending and Processing remain active until settlement, irrespective of ExecuteAt.
// This read does not modify indexes, including when a payload has disappeared.
func (r *ValkeyRepository) FindPendingByActorID(ctx context.Context, actorID string) ([]core.ScheduledAction, error) {
	if actorID == "" {
		return nil, nil
	}
	ids, err := r.client.Do(ctx, r.client.B().Smembers().Key(actorKeyPrefix+actorID).Build()).AsStrSlice()
	if err != nil {
		return nil, fmt.Errorf("read scheduled actor index: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = actionKeyPrefix + id
	}
	values, err := r.client.Do(ctx, r.client.B().Mget().Key(keys...).Build()).ToArray()
	if err != nil {
		return nil, fmt.Errorf("read scheduled action payloads: %w", err)
	}
	if len(values) != len(ids) {
		return nil, fmt.Errorf("%w: scheduled payload count mismatch", core.ErrInvalidAction)
	}
	var actions []core.ScheduledAction
	for i, value := range values {
		if value.IsNil() {
			continue
		}
		data, err := value.ToString()
		if err != nil {
			return nil, fmt.Errorf("read scheduled action %q: %w", ids[i], err)
		}
		var action core.ScheduledAction
		if err := json.Unmarshal([]byte(data), &action); err != nil {
			return nil, fmt.Errorf("decode scheduled action %q: %w", ids[i], err)
		}
		if err := action.Validate(); err != nil {
			return nil, fmt.Errorf("validate scheduled action %q: %w", ids[i], err)
		}
		if action.ID != ids[i] || action.ActorID != actorID {
			return nil, fmt.Errorf("%w: scheduled action %q identity mismatch", core.ErrInvalidAction, ids[i])
		}
		if action.State == core.StatePending || action.State == core.StateProcessing {
			actions = append(actions, action)
		}
	}
	return actions, nil
}
