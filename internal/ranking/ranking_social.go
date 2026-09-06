package ranking

import (
	"context"
)

// GetAdventureVictoryRanking returns character rankings sorted by adventure monster defeats.
func (s *Service) GetAdventureVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeAdventureVictory, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeAdventureVictory,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetAdventureVictoryRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeAdventureVictory,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetHelperRanking returns character rankings sorted by completed helper quests.
func (s *Service) GetHelperRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeHelper, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeHelper,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetHelperRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeHelper,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetSmallMedalRanking returns character rankings sorted by collected small medals.
func (s *Service) GetSmallMedalRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeSmallMedals, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeSmallMedals,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetSmallMedalRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeSmallMedals,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}
