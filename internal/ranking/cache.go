package ranking

import (
	"context"
	"time"
)

// SnapshotCache provides distributed or external caching for ranking snapshots.
type SnapshotCache interface {
	Get(ctx context.Context, rankingType RankingType) (RankingSnapshot, bool, error)
	Set(ctx context.Context, rankingType RankingType, snapshot RankingSnapshot, ttl time.Duration) error
	Delete(ctx context.Context, rankingType RankingType) error
}
