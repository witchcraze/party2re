package ranking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	rankingSnapshotKeyPrefix = "party2:ranking:snapshot:"
)

// ValkeySnapshotCache implements SnapshotCache using Valkey.
type ValkeySnapshotCache struct {
	client valkey.Client
}

// NewValkeySnapshotCache creates a new ValkeySnapshotCache instance.
func NewValkeySnapshotCache(client valkey.Client) *ValkeySnapshotCache {
	return &ValkeySnapshotCache{
		client: client,
	}
}

func (c *ValkeySnapshotCache) key(rankingType RankingType) string {
	return rankingSnapshotKeyPrefix + string(rankingType)
}

func (c *ValkeySnapshotCache) Get(ctx context.Context, rankingType RankingType) (RankingSnapshot, bool, error) {
	if c.client == nil {
		return RankingSnapshot{}, false, nil
	}

	cmd := c.client.B().Get().Key(c.key(rankingType)).Build()
	resp := c.client.Do(ctx, cmd)
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return RankingSnapshot{}, false, nil
		}
		// Graceful fail-open on connection error
		return RankingSnapshot{}, false, nil
	}

	val, err := resp.ToString()
	if err != nil {
		return RankingSnapshot{}, false, nil
	}

	var snapshot RankingSnapshot
	if err := json.Unmarshal([]byte(val), &snapshot); err != nil {
		return RankingSnapshot{}, false, nil
	}

	return snapshot, true, nil
}

func (c *ValkeySnapshotCache) Set(ctx context.Context, rankingType RankingType, snapshot RankingSnapshot, ttl time.Duration) error {
	if c.client == nil {
		return nil
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	cmd := c.client.B().Set().Key(c.key(rankingType)).Value(string(data)).Ex(ttl).Build()
	if err := c.client.Do(ctx, cmd).Error(); err != nil {
		// Graceful fail-open on write error
		return nil
	}
	return nil
}

func (c *ValkeySnapshotCache) Delete(ctx context.Context, rankingType RankingType) error {
	if c.client == nil {
		return nil
	}

	cmd := c.client.B().Del().Key(c.key(rankingType)).Build()
	_ = c.client.Do(ctx, cmd)
	return nil
}
