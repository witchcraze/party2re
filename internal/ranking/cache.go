package ranking

import (
	"context"
	"time"
)

// CacheWriteMode distinguishes authoritative snapshot refreshes from SQL fallback backfills.
type CacheWriteMode uint8

const (
	// CacheWriteRefresh publishes a newly committed snapshot generation.
	CacheWriteRefresh CacheWriteMode = iota
	// CacheWriteBackfill populates an absent or unusable cache entry from SQL without overwriting a valid generation.
	CacheWriteBackfill
)

// SnapshotPair stores a leader board and its exclusion-dependent board as one cache unit.
type SnapshotPair struct {
	Leader    RankingSnapshot `json:"leader"`
	Dependent RankingSnapshot `json:"dependent"`
}

// SnapshotCache provides distributed or external caching for ranking snapshots.
type SnapshotCache interface {
	Get(ctx context.Context, rankingType RankingType) (RankingSnapshot, bool, error)
	Set(ctx context.Context, rankingType RankingType, snapshot RankingSnapshot, ttl time.Duration, mode CacheWriteMode) (bool, error)
	Delete(ctx context.Context, rankingType RankingType) error
	GetPair(ctx context.Context, leaderType, dependentType RankingType) (SnapshotPair, bool, error)
	SetPair(ctx context.Context, pair SnapshotPair, ttl time.Duration, mode CacheWriteMode) (bool, error)
}
