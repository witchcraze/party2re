package ranking

import (
	"context"
	"time"
)

// ProgressionRankingRepository defines queries for job popularity tracking and leaderboards.
type ProgressionRankingRepository interface {
	GetJobPopularityRanking(ctx context.Context) ([]JobPopularityEntry, error)
	RecordJobPopularity(ctx context.Context, jobID string, gender string, points int) error
}

// CombatRankingRepository defines queries for battle, PvP, and boss-sealing leaderboards.
type CombatRankingRepository interface {
	GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetMonsterKillsRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetMaoCountRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetHeroCountRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
}

// ActivityRankingRepository defines queries for auxiliary activities and side-system leaderboards.
type ActivityRankingRepository interface {
	GetHelperRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetCasinoWinsRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetAlchemyRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetWeeklyJobChangeRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
}

// CharacterRankingRepository composes data access operations for individual character leaderboards.
type CharacterRankingRepository interface {
	ProgressionRankingRepository
	CombatRankingRepository
	ActivityRankingRepository
}

// LeaderboardRepository composes ranking query operations.
type LeaderboardRepository interface {
	CharacterRankingRepository
}

// SnapshotRepository defines persistence and retrieval operations for ranking snapshots.
type SnapshotRepository interface {
	SaveSnapshot(ctx context.Context, snapshot RankingSnapshot) error
	GetSnapshot(ctx context.Context, rankingType RankingType) (RankingSnapshot, error)
	GetAllSnapshots(ctx context.Context) (map[RankingType]RankingSnapshot, error)
}

// LegendRepository defines data access for permanent Hall of Fame records.
type LegendRepository interface {
	RecordLegend(ctx context.Context, entry LegendEntry) (bool, error)
	GetLegendInductees(ctx context.Context, category LegendCategory) ([]LegendEntry, error)
	GetLegendCategoryCounts(ctx context.Context) (map[LegendCategory]int, error)
}

// WeeklyJobChangeRepository defines data access for active weekly job change tracking.
type WeeklyJobChangeRepository interface {
	IncrementJobChangeCount(ctx context.Context, characterID string) error
	GetActiveWeeklyJobChangeRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	ResetWeeklyJobChanges(ctx context.Context) error
	LockWeeklyJobChangesForUpdate(ctx context.Context) error
	GetWeeklyRotationDue(ctx context.Context) (time.Time, error)
	SaveWeeklyRotationDue(ctx context.Context, due time.Time) error
}

// Repository composes leaderboard queries, snapshot persistence, and hall of fame operations.
type Repository interface {
	LeaderboardRepository
	SnapshotRepository
	LegendRepository
	WeeklyJobChangeRepository
}
