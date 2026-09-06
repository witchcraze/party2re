package ranking

import (
	"context"
)

// GetBattleVictoryRanking returns character rankings sorted by total recorded battle victories.
func (s *Service) GetBattleVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeBattleVictory, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeBattleVictory,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetBattleVictoryRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeBattleVictory,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetPvPVictoryRanking returns character rankings sorted by arena PvP victories.
func (s *Service) GetPvPVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypePvPVictory, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypePvPVictory,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetPvPVictoryRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypePvPVictory,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetBossDefeatRanking returns character rankings sorted by World Boss defeats.
func (s *Service) GetBossDefeatRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeBossDefeat, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeBossDefeat,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetBossDefeatRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeBossDefeat,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}
