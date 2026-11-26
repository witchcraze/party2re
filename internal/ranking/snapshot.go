package ranking

import (
	"context"
	"encoding/json"
	"fmt"
)

// RefreshSnapshot calculates and persists a snapshot for the specified ranking type.
func (s *Service) RefreshSnapshot(ctx context.Context, rankingType RankingType) error {
	rankingType = NormalizeRankingType(rankingType)
	now := s.nowFunc().UTC()
	var (
		data       any
		totalCount int
		err        error
	)

	switch rankingType {
	case RankingTypeWeeklyJobChange:
		return nil // Weekly rotation is the only writer of the finalized weekly snapshot.
	case RankingTypeJobPopularity:
		var entries []JobPopularityEntry
		entries, err = s.repo.GetJobPopularityRanking(ctx)
		totalCount = len(entries)
		data = entries
	default:
		var entries []CharacterRankingEntry
		entries, err = s.loadBoard(ctx, rankingType)
		totalCount = len(entries)
		data = entries
	}

	if err != nil {
		return fmt.Errorf("calculate ranking snapshot %s: %w", rankingType, err)
	}

	rawJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal ranking snapshot data: %w", err)
	}

	snapshot := RankingSnapshot{
		RankingType:  rankingType,
		SnapshotData: string(rawJSON),
		TotalCount:   totalCount,
		CalculatedAt: now,
		UpdatedAt:    now,
	}

	if err := s.repo.SaveSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("save ranking snapshot %s: %w", rankingType, err)
	}

	if s.valkeyCache != nil {
		_ = s.valkeyCache.Set(ctx, rankingType, snapshot, s.cacheTTL)
	}

	s.mu.Lock()
	s.cache[rankingType] = cacheEntry{
		data:      data,
		total:     totalCount,
		expiresAt: now.Add(s.cacheTTL),
		updatedAt: now,
	}
	s.mu.Unlock()

	return nil
}

var dailyRefreshTypes = []RankingType{
	RankingTypePvPVictory,
	RankingTypeJobPopularity,
	RankingTypeHelper,
	RankingTypeCasinoWins,
	RankingTypeAlchemy,
	RankingTypeMonsterKills,
	RankingTypeMaoCount,
	RankingTypeHeroCount,
}

// RefreshAllSnapshots recalculates and updates snapshots for all daily-published ranking types.
func (s *Service) RefreshAllSnapshots(ctx context.Context) error {
	for _, t := range dailyRefreshTypes {
		if err := s.RefreshSnapshot(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// GetSnapshot retrieves the raw persistent snapshot from the database repository.
func (s *Service) GetSnapshot(ctx context.Context, rankingType RankingType) (RankingSnapshot, error) {
	if !IsValidRankingType(rankingType) {
		return RankingSnapshot{}, ErrInvalidRankingType
	}
	return s.repo.GetSnapshot(ctx, rankingType)
}

// GetAllSnapshots retrieves all raw persistent snapshots from the database repository.
func (s *Service) GetAllSnapshots(ctx context.Context) (map[RankingType]RankingSnapshot, error) {
	return s.repo.GetAllSnapshots(ctx)
}
