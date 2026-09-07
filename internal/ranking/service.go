package ranking

import (
	"context"
	"encoding/json"
	"errors"
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
	case RankingTypeSmallMedals:
		return s.GetSmallMedalRanking(ctx, limit, offset, useSnapshot)
	default:
		return nil, ErrInvalidRankingType
	}
}

func unmarshalSnapshotData(t RankingType, raw string) (any, error) {
	switch t {
	case RankingTypePlayerWealth:
		var list []PlayerWealthRankingEntry
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, err
		}
		return list, nil
	case RankingTypeJobPopularity:
		var list []JobPopularityEntry
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, err
		}
		return list, nil
	default:
		var list []CharacterRankingEntry
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, err
		}
		return list, nil
	}
}

func (s *Service) storeCache(t RankingType, data any, total int, calculatedAt time.Time) {
	now := s.nowFunc().UTC()
	s.mu.Lock()
	s.cache[t] = cacheEntry{
		data:      data,
		total:     total,
		expiresAt: now.Add(s.cacheTTL),
		updatedAt: calculatedAt,
	}
	s.mu.Unlock()
}

func (s *Service) getOrLoadSnapshot(ctx context.Context, t RankingType) (any, int, time.Time, bool) {
	s.mu.RLock()
	cached, exists := s.cache[t]
	s.mu.RUnlock()

	if exists && cached.expiresAt.After(s.nowFunc()) {
		return cached.data, cached.total, cached.updatedAt, true
	}

	// 1. Check Valkey distributed cache
	if s.valkeyCache != nil {
		if snap, found, err := s.valkeyCache.Get(ctx, t); err == nil && found && snap.SnapshotData != "" {
			if data, err := unmarshalSnapshotData(t, snap.SnapshotData); err == nil {
				s.storeCache(t, data, snap.TotalCount, snap.CalculatedAt)
				return data, snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	// 2. Cache miss: Singleflight database fallback to prevent cache stampede
	res, err := s.sf.Do(string(t), func() (any, error) {
		snap, err := s.repo.GetSnapshot(ctx, t)
		if err == nil && snap.SnapshotData != "" {
			if data, err := unmarshalSnapshotData(t, snap.SnapshotData); err == nil {
				s.storeCache(t, data, snap.TotalCount, snap.CalculatedAt)
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
			if data, err := unmarshalSnapshotData(t, snap.SnapshotData); err == nil {
				return data, snap.TotalCount, snap.CalculatedAt, true
			}
		}
	}

	return nil, 0, time.Time{}, false
}

func (s *Service) getCachedCharacterRanking(ctx context.Context, t RankingType, limit, offset int) ([]CharacterRankingEntry, int, time.Time, bool) {
	data, total, calcTime, ok := s.getOrLoadSnapshot(ctx, t)
	if !ok {
		return nil, 0, time.Time{}, false
	}
	if list, ok := data.([]CharacterRankingEntry); ok {
		return paginateSlice(list, limit, offset), total, calcTime, true
	}
	return nil, 0, time.Time{}, false
}

func (s *Service) getCachedPlayerWealthRanking(ctx context.Context, t RankingType, limit, offset int) ([]PlayerWealthRankingEntry, int, time.Time, bool) {
	data, total, calcTime, ok := s.getOrLoadSnapshot(ctx, t)
	if !ok {
		return nil, 0, time.Time{}, false
	}
	if list, ok := data.([]PlayerWealthRankingEntry); ok {
		return paginateSlice(list, limit, offset), total, calcTime, true
	}
	return nil, 0, time.Time{}, false
}

func (s *Service) getCachedJobPopularityRanking(ctx context.Context) ([]JobPopularityEntry, int, time.Time, bool) {
	data, total, calcTime, ok := s.getOrLoadSnapshot(ctx, RankingTypeJobPopularity)
	if !ok {
		return nil, 0, time.Time{}, false
	}
	if list, ok := data.([]JobPopularityEntry); ok {
		return list, total, calcTime, true
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
