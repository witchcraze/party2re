package ranking

import (
	"context"
)

// GetLevelRanking returns character rankings sorted by Level and Experience.
func (s *Service) GetLevelRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeLevel, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeLevel,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetLevelRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeLevel,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetJobMasteryRanking returns character rankings sorted by count of mastered jobs.
func (s *Service) GetJobMasteryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeJobMastery, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeJobMastery,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetJobMasteryRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeJobMastery,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetJobPopularityRanking returns distribution statistics for all jobs.
func (s *Service) GetJobPopularityRanking(ctx context.Context, useSnapshot bool) (RankingPage[JobPopularityEntry], error) {
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedJobPopularityRanking(ctx)
		if found {
			return RankingPage[JobPopularityEntry]{
				RankingType:  RankingTypeJobPopularity,
				Entries:      entries,
				Total:        total,
				Limit:        len(entries),
				Offset:       0,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, err := s.repo.GetJobPopularityRanking(ctx)
	if err != nil {
		return RankingPage[JobPopularityEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[JobPopularityEntry]{
		RankingType:  RankingTypeJobPopularity,
		Entries:      entries,
		Total:        len(entries),
		Limit:        len(entries),
		Offset:       0,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}
