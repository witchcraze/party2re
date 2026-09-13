package eventplaza

import (
	"context"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	ValkeyPresenceKey = "party2:eventplaza:presence"
	ValkeyPresenceTTL = 3600 // 1 hour sliding TTL
)

// ValkeyPresenceTracker tracks character presence in Event Plaza using a Valkey Sorted Set.
type ValkeyPresenceTracker struct {
	client valkey.Client
}

// NewValkeyPresenceTracker creates a new ValkeyPresenceTracker.
func NewValkeyPresenceTracker(client valkey.Client) *ValkeyPresenceTracker {
	return &ValkeyPresenceTracker{
		client: client,
	}
}

// RecordPresence records a character's presence at the specified time.
func (t *ValkeyPresenceTracker) RecordPresence(ctx context.Context, characterID string, at time.Time) error {
	if t.client == nil {
		return nil
	}

	score := float64(at.Unix())
	cmd := t.client.B().Zadd().Key(ValkeyPresenceKey).ScoreMember().ScoreMember(score, characterID).Build()
	if err := t.client.Do(ctx, cmd).Error(); err != nil {
		return err
	}

	expireCmd := t.client.B().Expire().Key(ValkeyPresenceKey).Seconds(ValkeyPresenceTTL).Build()
	_ = t.client.Do(ctx, expireCmd)
	return nil
}

// CountActiveParticipants returns the number of active participants seen at or after cutoff.
func (t *ValkeyPresenceTracker) CountActiveParticipants(ctx context.Context, cutoff time.Time) (int, error) {
	if t.client == nil {
		return 0, nil
	}

	// Purge stale members older than cutoff
	purgeCmd := t.client.B().Zremrangebyscore().Key(ValkeyPresenceKey).Min("-inf").Max(strconv.FormatInt(cutoff.Unix()-1, 10)).Build()
	_ = t.client.Do(ctx, purgeCmd)

	cardCmd := t.client.B().Zcard().Key(ValkeyPresenceKey).Build()
	resp := t.client.Do(ctx, cardCmd)
	if err := resp.Error(); err != nil {
		return 0, err
	}

	count, err := resp.AsInt64()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
