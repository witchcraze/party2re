package ranking

import (
	"context"
	"errors"
)

// GetJobPopularityRanking returns distribution statistics for all jobs from the published snapshot.
func (s *Service) GetJobPopularityRanking(ctx context.Context) (RankingPage[JobPopularityEntry], error) {
	if entries, total, calcTime, found := s.getCachedJobPopularityRanking(ctx); found {
		if entries == nil {
			entries = []JobPopularityEntry{}
		}
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

	return RankingPage[JobPopularityEntry]{
		RankingType: RankingTypeJobPopularity,
		Entries:     []JobPopularityEntry{},
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
