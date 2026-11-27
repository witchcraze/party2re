package ranking

import (
	"context"
	"errors"
	"fmt"
)

// GetJobPopularityRanking returns distribution statistics for all jobs from the published snapshot.
func (s *Service) GetJobPopularityRanking(ctx context.Context) (RankingPage[JobPopularityEntry], error) {
	data, total, calcTime, err := s.getOrLoadSnapshot(ctx, RankingTypeJobPopularity)
	if err != nil {
		if errors.Is(err, ErrSnapshotNotFound) {
			return RankingPage[JobPopularityEntry]{
				RankingType: RankingTypeJobPopularity,
				Entries:     []JobPopularityEntry{},
			}, nil
		}
		return RankingPage[JobPopularityEntry]{}, err
	}
	entries, ok := data.([]JobPopularityEntry)
	if !ok {
		return RankingPage[JobPopularityEntry]{}, fmt.Errorf("unexpected snapshot data type %T for %s", data, RankingTypeJobPopularity)
	}
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
