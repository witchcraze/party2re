package ranking

import (
	"context"
)

// ProgressionRankingRepository defines queries for level, wealth, and job progression leaderboards.
type ProgressionRankingRepository interface {
	GetLevelRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetCharacterWealthRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetJobMasteryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetJobPopularityRanking(ctx context.Context) ([]JobPopularityEntry, error)
}

// CombatRankingRepository defines queries for battle, PvP, boss, and adventure leaderboards.
type CombatRankingRepository interface {
	GetBattleVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetBossDefeatRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetAdventureVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
}

// ActivityRankingRepository defines queries for auxiliary activities and side-system leaderboards.
type ActivityRankingRepository interface {
	GetHelperRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetSmallMedalRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
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

// PlayerWealthRankingRepository defines data access operations for account-level player wealth leaderboards.
type PlayerWealthRankingRepository interface {
	GetPlayerWealthRanking(ctx context.Context, limit, offset int) ([]PlayerWealthRankingEntry, int, error)
}

// LeaderboardRepository composes character and player ranking query operations.
type LeaderboardRepository interface {
	CharacterRankingRepository
	PlayerWealthRankingRepository
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
}

// Repository composes leaderboard queries, snapshot persistence, and hall of fame operations.
type Repository interface {
	LeaderboardRepository
	SnapshotRepository
	LegendRepository
	WeeklyJobChangeRepository
}
