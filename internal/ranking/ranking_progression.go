package ranking

import (
	"context"
	"errors"
)

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

// RecordJobPopularity increments cumulative popularity points for a job from a job change.
func (s *Service) RecordJobPopularity(ctx context.Context, jobID string, gender string, points int) error {
	if jobID == "" {
		return errors.New("job ID required")
	}
	if points < 0 {
		points = 0
	}
	return s.repo.RecordJobPopularity(ctx, jobID, gender, points)
}
