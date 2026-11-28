package ranking

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	rankingSnapshotKeyPrefix = "party2:ranking:snapshot:"
	rankingPairKeyPrefix     = "party2:ranking:snapshot:pair:"
)

//go:embed lua/set_snapshot.lua
var setSnapshotLua string

type cachedEntry struct {
	Timestamp string           `json:"ts"`
	Snapshot  *RankingSnapshot `json:"snapshot,omitempty"`
	Pair      *SnapshotPair    `json:"pair,omitempty"`
}

// ValkeySnapshotCache implements SnapshotCache using Valkey.
type ValkeySnapshotCache struct {
	client    valkey.Client
	setScript *valkey.Lua
}

// NewValkeySnapshotCache creates a new ValkeySnapshotCache instance.
func NewValkeySnapshotCache(client valkey.Client) *ValkeySnapshotCache {
	return &ValkeySnapshotCache{
		client:    client,
		setScript: valkey.NewLuaScript(setSnapshotLua),
	}
}

func (c *ValkeySnapshotCache) pairKey(leaderType, dependentType RankingType) string {
	return rankingPairKeyPrefix + string(leaderType) + ":" + string(dependentType)
}

func (c *ValkeySnapshotCache) key(rankingType RankingType) string {
	if pair, ok := exclusionPairFor(rankingType); ok {
		return c.pairKey(pair.leaderType, pair.dependentType)
	}
	return rankingSnapshotKeyPrefix + string(rankingType)
}

func formatCacheTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func laterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (c *ValkeySnapshotCache) getEntry(ctx context.Context, key string) (cachedEntry, bool) {
	if c.client == nil {
		return cachedEntry{}, false
	}

	cmd := c.client.B().Get().Key(key).Build()
	resp := c.client.Do(ctx, cmd)
	if err := resp.Error(); err != nil {
		// Graceful fail-open on missing key or connection error.
		return cachedEntry{}, false
	}

	val, err := resp.ToString()
	if err != nil {
		return cachedEntry{}, false
	}

	var entry cachedEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return cachedEntry{}, false
	}
	return entry, true
}

func (c *ValkeySnapshotCache) execSet(ctx context.Context, key string, entry cachedEntry, ttl time.Duration, mode CacheWriteMode, ts time.Time, kind string) (bool, error) {
	if c.client == nil || ttl <= 0 {
		return false, nil
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return false, err
	}

	ttlMs := ttl.Milliseconds()
	if ttlMs < 1 {
		ttlMs = 1
	}
	modeStr := "refresh"
	if mode == CacheWriteBackfill {
		modeStr = "backfill"
	}

	res := c.setScript.Exec(ctx, c.client, []string{key}, []string{
		string(data),
		strconv.FormatInt(ttlMs, 10),
		modeStr,
		formatCacheTimestamp(ts),
		kind,
	})
	if err := res.Error(); err != nil {
		return false, err
	}
	written, err := res.AsInt64()
	if err != nil {
		return false, err
	}
	return written == 1, nil
}

func (c *ValkeySnapshotCache) Get(ctx context.Context, rankingType RankingType) (RankingSnapshot, bool, error) {
	if pair, ok := exclusionPairFor(rankingType); ok {
		sp, found, err := c.GetPair(ctx, pair.leaderType, pair.dependentType)
		if err != nil || !found {
			return RankingSnapshot{}, false, err
		}
		if rankingType == pair.leaderType {
			return sp.Leader, true, nil
		}
		return sp.Dependent, true, nil
	}

	entry, ok := c.getEntry(ctx, c.key(rankingType))
	if !ok || entry.Snapshot == nil {
		return RankingSnapshot{}, false, nil
	}
	return *entry.Snapshot, true, nil
}

func (c *ValkeySnapshotCache) Set(ctx context.Context, rankingType RankingType, snapshot RankingSnapshot, ttl time.Duration, mode CacheWriteMode) (bool, error) {
	if _, ok := exclusionPairFor(rankingType); ok {
		return false, fmt.Errorf("paired ranking %s must be cached as a pair", rankingType)
	}
	entry := cachedEntry{
		Timestamp: formatCacheTimestamp(snapshot.CalculatedAt),
		Snapshot:  &snapshot,
	}
	return c.execSet(ctx, c.key(rankingType), entry, ttl, mode, snapshot.CalculatedAt, "single")
}

func (c *ValkeySnapshotCache) GetPair(ctx context.Context, leaderType, dependentType RankingType) (SnapshotPair, bool, error) {
	entry, ok := c.getEntry(ctx, c.pairKey(leaderType, dependentType))
	if !ok || entry.Pair == nil {
		return SnapshotPair{}, false, nil
	}
	return *entry.Pair, true, nil
}

func (c *ValkeySnapshotCache) SetPair(ctx context.Context, pair SnapshotPair, ttl time.Duration, mode CacheWriteMode) (bool, error) {
	spec, ok := exclusionPairFor(pair.Leader.RankingType)
	if !ok || spec.leaderType != pair.Leader.RankingType || spec.dependentType != pair.Dependent.RankingType {
		return false, errors.New("invalid ranking exclusion pair")
	}
	ts := laterTime(pair.Leader.CalculatedAt, pair.Dependent.CalculatedAt)
	entry := cachedEntry{
		Timestamp: formatCacheTimestamp(ts),
		Pair:      &pair,
	}
	return c.execSet(ctx, c.pairKey(spec.leaderType, spec.dependentType), entry, ttl, mode, ts, "pair")
}

func (c *ValkeySnapshotCache) Delete(ctx context.Context, rankingType RankingType) error {
	if c.client == nil {
		return nil
	}

	cmd := c.client.B().Del().Key(c.key(rankingType)).Build()
	_ = c.client.Do(ctx, cmd)
	return nil
}
