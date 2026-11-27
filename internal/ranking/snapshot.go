package ranking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type preparedSnapshot struct {
	snapshot RankingSnapshot
	data     any
}

func makePreparedSnapshot(rankingType RankingType, data any, totalCount int, now time.Time) (preparedSnapshot, error) {
	rawJSON, err := json.Marshal(data)
	if err != nil {
		return preparedSnapshot{}, fmt.Errorf("marshal ranking snapshot data: %w", err)
	}
	return preparedSnapshot{
		snapshot: RankingSnapshot{
			RankingType:  rankingType,
			SnapshotData: string(rawJSON),
			TotalCount:   totalCount,
			CalculatedAt: now,
			UpdatedAt:    now,
		},
		data: data,
	}, nil
}

func (s *Service) buildPairSnapshots(ctx context.Context, pair pairedBoardDef, now time.Time) ([]preparedSnapshot, error) {
	leaders, dependent, err := s.loadPairedBoards(ctx, pair.leaderQuery, pair.dependentQuery)
	if err != nil {
		return nil, fmt.Errorf("calculate ranking snapshot pair (%s, %s): %w", pair.leaderType, pair.dependentType, err)
	}
	leaderSnap, err := makePreparedSnapshot(pair.leaderType, leaders, len(leaders), now)
	if err != nil {
		return nil, err
	}
	dependentSnap, err := makePreparedSnapshot(pair.dependentType, dependent, len(dependent), now)
	if err != nil {
		return nil, err
	}
	return []preparedSnapshot{leaderSnap, dependentSnap}, nil
}

func (s *Service) buildStandaloneSnapshot(ctx context.Context, rankingType RankingType, now time.Time) (preparedSnapshot, error) {
	if rankingType == RankingTypeJobPopularity {
		entries, err := s.repo.GetJobPopularityRanking(ctx)
		if err != nil {
			return preparedSnapshot{}, fmt.Errorf("calculate ranking snapshot %s: %w", rankingType, err)
		}
		if entries == nil {
			entries = []JobPopularityEntry{}
		}
		return makePreparedSnapshot(rankingType, entries, len(entries), now)
	}
	query, ok := s.standaloneBoardQuery(rankingType)
	if !ok {
		return preparedSnapshot{}, fmt.Errorf("calculate ranking snapshot %s: %w", rankingType, ErrInvalidRankingType)
	}
	entries, err := s.loadSingleBoard(ctx, query)
	if err != nil {
		return preparedSnapshot{}, fmt.Errorf("calculate ranking snapshot %s: %w", rankingType, err)
	}
	return makePreparedSnapshot(rankingType, entries, len(entries), now)
}

func (s *Service) refreshBatch(ctx context.Context, build func(ctx context.Context, now time.Time) ([]preparedSnapshot, error)) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	now := s.nowFunc().UTC()
	var prepared []preparedSnapshot
	if err := s.runInTx(ctx, func(txCtx context.Context) error {
		var err error
		prepared, err = build(txCtx, now)
		if err != nil {
			return err
		}
		for _, item := range prepared {
			if err := s.repo.SaveSnapshot(txCtx, item.snapshot); err != nil {
				return fmt.Errorf("save ranking snapshot %s: %w", item.snapshot.RankingType, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, item := range prepared {
		if s.valkeyCache != nil {
			_ = s.valkeyCache.Set(ctx, item.snapshot.RankingType, item.snapshot, s.cacheTTL)
		}
	}
	cacheNow := s.nowFunc().UTC()
	s.mu.Lock()
	for _, item := range prepared {
		if existing, ok := s.cache[item.snapshot.RankingType]; ok && existing.updatedAt.After(item.snapshot.CalculatedAt) {
			continue
		}
		s.cache[item.snapshot.RankingType] = cacheEntry{
			data:      item.data,
			total:     item.snapshot.TotalCount,
			expiresAt: cacheNow.Add(s.cacheTTL),
			updatedAt: item.snapshot.CalculatedAt,
		}
	}
	s.mu.Unlock()
	return nil
}

// RefreshSnapshot calculates and persists a snapshot for the specified ranking type.
// Refreshing either board of a paired leaderboard publishes the whole pair atomically.
func (s *Service) RefreshSnapshot(ctx context.Context, rankingType RankingType) error {
	rankingType = NormalizeRankingType(rankingType)
	if rankingType == RankingTypeWeeklyJobChange {
		return nil // Weekly rotation is the only writer of the finalized weekly snapshot.
	}
	if !IsValidRankingType(rankingType) {
		return fmt.Errorf("calculate ranking snapshot %s: %w", rankingType, ErrInvalidRankingType)
	}

	if pair, ok := s.pairedBoardFor(rankingType); ok {
		return s.refreshBatch(ctx, func(txCtx context.Context, now time.Time) ([]preparedSnapshot, error) {
			return s.buildPairSnapshots(txCtx, pair, now)
		})
	}
	return s.refreshBatch(ctx, func(txCtx context.Context, now time.Time) ([]preparedSnapshot, error) {
		item, err := s.buildStandaloneSnapshot(txCtx, rankingType, now)
		if err != nil {
			return nil, err
		}
		return []preparedSnapshot{item}, nil
	})
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

var standaloneDailyRefreshTypes = []RankingType{
	RankingTypeJobPopularity,
	RankingTypeHelper,
	RankingTypeCasinoWins,
	RankingTypeAlchemy,
}

// RefreshAllSnapshots recalculates and updates snapshots for all daily-published ranking types.
func (s *Service) RefreshAllSnapshots(ctx context.Context) error {
	return s.refreshBatch(ctx, func(txCtx context.Context, now time.Time) ([]preparedSnapshot, error) {
		prepared := make([]preparedSnapshot, 0, len(dailyRefreshTypes))
		for _, pair := range s.pairedBoardDefs() {
			items, err := s.buildPairSnapshots(txCtx, pair, now)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, items...)
		}
		for _, t := range standaloneDailyRefreshTypes {
			item, err := s.buildStandaloneSnapshot(txCtx, t, now)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, item)
		}
		return prepared, nil
	})
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
