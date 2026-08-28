package ranking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultCacheTTL = 5 * time.Minute
)

type cacheEntry struct {
	data      any
	total     int
	expiresAt time.Time
	updatedAt time.Time
}

// Service provides high-level ranking and leaderboard operations.
type Service struct {
	repo        Repository
	valkeyCache SnapshotCache
	sf          group
	nowFunc     func() time.Time
	cacheTTL    time.Duration
	mu          sync.RWMutex
	cache       map[RankingType]cacheEntry
}

// ServiceOption configures optional parameters for Service.
type ServiceOption func(*Service)

// WithNowFunc configures custom clock for testing.
func WithNowFunc(fn func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

// WithCacheTTL configures TTL for cached ranking snapshots.
func WithCacheTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		s.cacheTTL = ttl
	}
}

// WithSnapshotCache configures a distributed snapshot cache (e.g. Valkey) for the Service.
func WithSnapshotCache(cache SnapshotCache) ServiceOption {
	return func(s *Service) {
		s.valkeyCache = cache
	}
}

// NewService creates a new ranking Service instance.
func NewService(repo Repository, opts ...ServiceOption) (*Service, error) {
	if repo == nil {
		return nil, errors.New("ranking repository is required")
	}
	s := &Service{
		repo:     repo,
		nowFunc:  func() time.Time { return time.Now().UTC() },
		cacheTTL: DefaultCacheTTL,
		cache:    make(map[RankingType]cacheEntry),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

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

// GetRebirthRanking returns character rankings sorted by rebirth count.
func (s *Service) GetRebirthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	if useSnapshot {
		entries, total, calcTime, found := s.getCachedCharacterRanking(ctx, RankingTypeRebirth, limit, offset)
		if found {
			return RankingPage[CharacterRankingEntry]{
				RankingType:  RankingTypeRebirth,
				Entries:      entries,
				Total:        total,
				Limit:        limit,
				Offset:       offset,
				CalculatedAt: calcTime,
				IsSnapshot:   true,
			}, nil
		}
	}

	entries, total, err := s.repo.GetRebirthRanking(ctx, limit, offset)
	if err != nil {
		return RankingPage[CharacterRankingEntry]{}, err
	}

	now := s.nowFunc().UTC()
	return RankingPage[CharacterRankingEntry]{
		RankingType:  RankingTypeRebirth,
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

// GetRankingByType retrieves ranking for a dynamic type string.
func (s *Service) GetRankingByType(ctx context.Context, rankingType RankingType, limit, offset int, useSnapshot bool) (any, error) {
	switch rankingType {
	case RankingTypeLevel:
		return s.GetLevelRanking(ctx, limit, offset, useSnapshot)
	case RankingTypePlayerWealth:
		return s.GetPlayerWealthRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeCharacterWealth:
		return s.GetCharacterWealthRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeBattleVictory:
		return s.GetBattleVictoryRanking(ctx, limit, offset, useSnapshot)
	case RankingTypePvPVictory:
		return s.GetPvPVictoryRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeBossDefeat:
		return s.GetBossDefeatRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeAdventureVictory:
		return s.GetAdventureVictoryRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeJobMastery:
		return s.GetJobMasteryRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeJobPopularity:
		return s.GetJobPopularityRanking(ctx, useSnapshot)
	case RankingTypeHelper:
		return s.GetHelperRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeRebirth:
		return s.GetRebirthRanking(ctx, limit, offset, useSnapshot)
	case RankingTypeSmallMedals:
		return s.GetSmallMedalRanking(ctx, limit, offset, useSnapshot)
	default:
		return nil, ErrInvalidRankingType
	}
}

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
	case RankingTypeRebirth:
		var entries []CharacterRankingEntry
		entries, totalCount, err = s.repo.GetRebirthRanking(ctx, 100, 0)
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
		RankingTypeRebirth,
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

func (s *Service) getCachedCharacterRanking(ctx context.Context, t RankingType, limit, offset int) ([]CharacterRankingEntry, int, time.Time, bool) {
	s.mu.RLock()
	cached, exists := s.cache[t]
	s.mu.RUnlock()

	if exists && cached.expiresAt.After(s.nowFunc()) {
		if list, ok := cached.data.([]CharacterRankingEntry); ok {
			return paginateSlice(list, limit, offset), cached.total, cached.updatedAt, true
		}
	}

	// 1. Check Valkey distributed cache
	if s.valkeyCache != nil {
		if snap, found, err := s.valkeyCache.Get(ctx, t); err == nil && found && snap.SnapshotData != "" {
			var list []CharacterRankingEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				now := s.nowFunc().UTC()
				s.mu.Lock()
				s.cache[t] = cacheEntry{
					data:      list,
					total:     snap.TotalCount,
					expiresAt: now.Add(s.cacheTTL),
					updatedAt: snap.CalculatedAt,
				}
				s.mu.Unlock()
				return paginateSlice(list, limit, offset), snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	// 2. Cache miss: Singleflight database fallback to prevent cache stampede
	res, err := s.sf.Do(string(t), func() (any, error) {
		snap, err := s.repo.GetSnapshot(ctx, t)
		if err == nil && snap.SnapshotData != "" {
			var list []CharacterRankingEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				now := s.nowFunc().UTC()
				s.mu.Lock()
				s.cache[t] = cacheEntry{
					data:      list,
					total:     snap.TotalCount,
					expiresAt: now.Add(s.cacheTTL),
					updatedAt: snap.CalculatedAt,
				}
				s.mu.Unlock()
				if s.valkeyCache != nil {
					_ = s.valkeyCache.Set(ctx, t, snap, s.cacheTTL)
				}
				return snap, nil
			}
		}
		return nil, errors.New("no snapshot available")
	})

	if err == nil {
		if snap, ok := res.(RankingSnapshot); ok {
			var list []CharacterRankingEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				return paginateSlice(list, limit, offset), snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	return nil, 0, time.Time{}, false
}

func (s *Service) getCachedPlayerWealthRanking(ctx context.Context, t RankingType, limit, offset int) ([]PlayerWealthRankingEntry, int, time.Time, bool) {
	s.mu.RLock()
	cached, exists := s.cache[t]
	s.mu.RUnlock()

	if exists && cached.expiresAt.After(s.nowFunc()) {
		if list, ok := cached.data.([]PlayerWealthRankingEntry); ok {
			return paginateSlice(list, limit, offset), cached.total, cached.updatedAt, true
		}
	}

	// 1. Check Valkey distributed cache
	if s.valkeyCache != nil {
		if snap, found, err := s.valkeyCache.Get(ctx, t); err == nil && found && snap.SnapshotData != "" {
			var list []PlayerWealthRankingEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				now := s.nowFunc().UTC()
				s.mu.Lock()
				s.cache[t] = cacheEntry{
					data:      list,
					total:     snap.TotalCount,
					expiresAt: now.Add(s.cacheTTL),
					updatedAt: snap.CalculatedAt,
				}
				s.mu.Unlock()
				return paginateSlice(list, limit, offset), snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	// 2. Cache miss: Singleflight database fallback to prevent cache stampede
	res, err := s.sf.Do(string(t), func() (any, error) {
		snap, err := s.repo.GetSnapshot(ctx, t)
		if err == nil && snap.SnapshotData != "" {
			var list []PlayerWealthRankingEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				now := s.nowFunc().UTC()
				s.mu.Lock()
				s.cache[t] = cacheEntry{
					data:      list,
					total:     snap.TotalCount,
					expiresAt: now.Add(s.cacheTTL),
					updatedAt: snap.CalculatedAt,
				}
				s.mu.Unlock()
				if s.valkeyCache != nil {
					_ = s.valkeyCache.Set(ctx, t, snap, s.cacheTTL)
				}
				return snap, nil
			}
		}
		return nil, errors.New("no snapshot available")
	})

	if err == nil {
		if snap, ok := res.(RankingSnapshot); ok {
			var list []PlayerWealthRankingEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				return paginateSlice(list, limit, offset), snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	return nil, 0, time.Time{}, false
}

func (s *Service) getCachedJobPopularityRanking(ctx context.Context) ([]JobPopularityEntry, int, time.Time, bool) {
	t := RankingTypeJobPopularity
	s.mu.RLock()
	cached, exists := s.cache[t]
	s.mu.RUnlock()

	if exists && cached.expiresAt.After(s.nowFunc()) {
		if list, ok := cached.data.([]JobPopularityEntry); ok {
			return list, cached.total, cached.updatedAt, true
		}
	}

	// 1. Check Valkey distributed cache
	if s.valkeyCache != nil {
		if snap, found, err := s.valkeyCache.Get(ctx, t); err == nil && found && snap.SnapshotData != "" {
			var list []JobPopularityEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				now := s.nowFunc().UTC()
				s.mu.Lock()
				s.cache[t] = cacheEntry{
					data:      list,
					total:     snap.TotalCount,
					expiresAt: now.Add(s.cacheTTL),
					updatedAt: snap.CalculatedAt,
				}
				s.mu.Unlock()
				return list, snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	// 2. Cache miss: Singleflight database fallback to prevent cache stampede
	res, err := s.sf.Do(string(t), func() (any, error) {
		snap, err := s.repo.GetSnapshot(ctx, t)
		if err == nil && snap.SnapshotData != "" {
			var list []JobPopularityEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				now := s.nowFunc().UTC()
				s.mu.Lock()
				s.cache[t] = cacheEntry{
					data:      list,
					total:     snap.TotalCount,
					expiresAt: now.Add(s.cacheTTL),
					updatedAt: snap.CalculatedAt,
				}
				s.mu.Unlock()
				if s.valkeyCache != nil {
					_ = s.valkeyCache.Set(ctx, t, snap, s.cacheTTL)
				}
				return snap, nil
			}
		}
		return nil, errors.New("no snapshot available")
	})

	if err == nil {
		if snap, ok := res.(RankingSnapshot); ok {
			var list []JobPopularityEntry
			if err := json.Unmarshal([]byte(snap.SnapshotData), &list); err == nil {
				return list, snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	return nil, 0, time.Time{}, false
}

func paginateSlice[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
