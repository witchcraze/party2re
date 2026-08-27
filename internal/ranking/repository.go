package ranking

import (
	"context"
)

// Repository defines data access operations for leaderboards and ranking snapshots.
type Repository interface {
	GetLevelRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetPlayerWealthRanking(ctx context.Context, limit, offset int) ([]PlayerWealthRankingEntry, int, error)
	GetCharacterWealthRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetBattleVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetBossDefeatRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetAdventureVictoryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetJobMasteryRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetJobPopularityRanking(ctx context.Context) ([]JobPopularityEntry, error)
	GetHelperRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetRebirthRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)
	GetSmallMedalRanking(ctx context.Context, limit, offset int) ([]CharacterRankingEntry, int, error)

	SaveSnapshot(ctx context.Context, snapshot RankingSnapshot) error
	GetSnapshot(ctx context.Context, rankingType RankingType) (RankingSnapshot, error)
	GetAllSnapshots(ctx context.Context) (map[RankingType]RankingSnapshot, error)
}
