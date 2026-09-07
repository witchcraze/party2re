package ranking

import (
	"context"
	"encoding/json"
	"fmt"
)

// RefreshSnapshot calculates and persists a snapshot for the specified ranking type.
func (s *Service) RefreshSnapshot(ctx context.Context, rankingType RankingType) error {
	now := s.nowFunc().UTC()
	var (
		data       any
		totalCount int
		err        error
	)

	switch rankingType {
	case RankingTypeLevel:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetLevelRanking(ctx, 100, 0)
		data = entries
	case RankingTypePlayerWealth:
		var entries []PlayerWealthRankingEntry
		entries, totalCount, err = s.repo.GetPlayerWealthRanking(ctx, 100, 0)
		data = entries
	case RankingTypeCharacterWealth:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetCharacterWealthRanking(ctx, 100, 0)
		data = entries
	case RankingTypeBattleVictory:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetBattleVictoryRanking(ctx, 100, 0)
		data = entries
	case RankingTypePvPVictory:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetPvPVictoryRanking(ctx, 100, 0)
		data = entries
	case RankingTypeBossDefeat:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetBossDefeatRanking(ctx, 100, 0)
		data = entries
	case RankingTypeAdventureVictory:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetAdventureVictoryRanking(ctx, 100, 0)
		data = entries
	case RankingTypeJobMastery:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetJobMasteryRanking(ctx, 100, 0)
		data = entries
	case RankingTypeJobPopularity:
		var entries []JobPopularityEntry
		entries, err = s.repo.GetJobPopularityRanking(ctx)
		totalCount = len(entries)
		data = entries
	case RankingTypeHelper:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetHelperRanking(ctx, 100, 0)
		data = entries
	case RankingTypeSmallMedals:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetSmallMedalRanking(ctx, 100, 0)
		data = entries
	default:
		return ErrInvalidRankingType
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

// RefreshAllSnapshots recalculates and updates snapshots for all supported ranking types.
func (s *Service) RefreshAllSnapshots(ctx context.Context) error {
	types := []RankingType{
		RankingTypeLevel,
		RankingTypePlayerWealth,
		RankingTypeCharacterWealth,
		RankingTypeBattleVictory,
		RankingTypePvPVictory,
		RankingTypeBossDefeat,
		RankingTypeAdventureVictory,
		RankingTypeJobMastery,
		RankingTypeJobPopularity,
		RankingTypeHelper,
		RankingTypeSmallMedals,
	}

	for _, t := range types {
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

// WarmupCache preloads all persistent snapshots from the database repository into the in-memory cache and Valkey cache.
func (s *Service) WarmupCache(ctx context.Context) error {
	snapshots, err := s.repo.GetAllSnapshots(ctx)
	if err != nil {
		return err
	}
	now := s.nowFunc().UTC()
	for rankingType, snapshot := range snapshots {
		if snapshot.SnapshotData == "" {
			continue
		}
		var parsedData any
		switch rankingType {
		case RankingTypePlayerWealth:
			var list []PlayerWealthRankingEntry
			if err := json.Unmarshal([]byte(snapshot.SnapshotData), &list); err == nil {
				parsedData = list
			}
		case RankingTypeJobPopularity:
			var list []JobPopularityEntry
			if err := json.Unmarshal([]byte(snapshot.SnapshotData), &list); err == nil {
				parsedData = list
			}
		default:
			var list []CharacterRankingEntry
			if err := json.Unmarshal([]byte(snapshot.SnapshotData), &list); err == nil {
				parsedData = list
			}
		}

		if parsedData != nil {
			s.mu.Lock()
			s.cache[rankingType] = cacheEntry{
				data:      parsedData,
				total:     snapshot.TotalCount,
				expiresAt: now.Add(s.cacheTTL),
				updatedAt: snapshot.CalculatedAt,
			}
			s.mu.Unlock()
			if s.valkeyCache != nil {
				_ = s.valkeyCache.Set(ctx, rankingType, snapshot, s.cacheTTL)
			}
		}
	}
	return nil
}
