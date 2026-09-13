package ranking

import (
	"context"
)

// CharacterRankingRepository defines data access operations for individual character leaderboards.
type CharacterRankingRepository interface {
	GetLevelRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetCharacterWealthRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetBattleVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetBossDefeatRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetAdventureVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetJobMasteryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetJobPopularityRanking(ctx context.Context) ([]JobPopularityEntry, error)
	GetHelperRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetSmallMedalRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
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

// Repository composes leaderboard queries and snapshot persistence operations.
type Repository interface {
	LeaderboardRepository
	SnapshotRepository
}
