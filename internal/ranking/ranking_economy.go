package ranking

import (
	"context"
)

// GetPlayerWealthRanking returns player rankings sorted by total combined wealth.
func (s *Service) GetPlayerWealthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[PlayerWealthRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedPlayerWealthRanking(ctx, RankingTypePlayerWealth, limit, offset)
		if found {
			return RankingPage[PlayerWealthRankingEntry]{
				RankingType:  RankingTypePlayerWealth,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetPlayerWealthRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[PlayerWealthRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[PlayerWealthRankingEntry]{
		RankingType:  RankingTypePlayerWealth,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}

// GetCharacterWealthRanking returns character rankings sorted by held gold.
func (s *Service) GetCharacterWealthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeCharacterWealth, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeCharacterWealth,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetCharacterWealthRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeCharacterWealth,
		Entries:      entries,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		CalculatedAt: now,
		IsSnapshot:   false,
	}, nil
}
