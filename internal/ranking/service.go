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

// TransactionProvider defines ambient transaction runner contract.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
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
	txProvider  TransactionProvider
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

// WithTransactionProvider configures a transaction provider for atomic ranking operations.
func WithTransactionProvider(txProvider TransactionProvider) ServiceOption {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	if tp, ok := s.repo.(TransactionProvider); ok {
		return tp.RunInTx(ctx, fn)
	}
	return fn(ctx)
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

// GetRankingByType retrieves the published ranking for a dynamic type string.
func (s *Service) GetRankingByType(ctx context.Context, rankingType RankingType, limit, offset int) (any, error) {
	switch NormalizeRankingType(rankingType) {
	case RankingTypePvPVictory:
		return s.GetPvPVictoryRanking(ctx, limit, offset)
	case RankingTypeJobPopularity:
		return s.GetJobPopularityRanking(ctx)
	case RankingTypeHelper:
		return s.GetHelperRanking(ctx, limit, offset)
	case RankingTypeCasinoWins:
		return s.GetCasinoWinsRanking(ctx, limit, offset)
	case RankingTypeAlchemy:
		return s.GetAlchemyRanking(ctx, limit, offset)
	case RankingTypeWeeklyJobChange:
		return s.GetWeeklyJobChangeRanking(ctx, limit, offset)
	case RankingTypeMonsterKills:
		return s.GetMonsterKillsRanking(ctx, limit, offset)
	case RankingTypeMaoCount:
		return s.GetMaoCountRanking(ctx, limit, offset)
	case RankingTypeHeroCount:
		return s.GetHeroCountRanking(ctx, limit, offset)
	default:
		return nil, ErrInvalidRankingType
	}
}

func unmarshalSnapshotData(t RankingType, raw string) (any, error) {
	switch t {
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

type loadedSnapshot struct {
	data         any
	total        int
	calculatedAt time.Time
}

func (s *Service) getOrLoadSnapshot(ctx context.Context, t RankingType) (any, int, time.Time, error) {
	s.mu.RLock()
	cached, exists := s.cache[t]
	s.mu.RUnlock()

	if exists && cached.expiresAt.After(s.nowFunc()) {
		return cached.data, cached.total, cached.updatedAt, nil
	}

	// 1. Check Valkey distributed cache
	if s.valkeyCache != nil {
		if snap, found, err := s.valkeyCache.Get(ctx, t); err == nil && found && snap.SnapshotData != "" {
			if data, err := unmarshalSnapshotData(t, snap.SnapshotData); err == nil {
				s.storeCache(t, data, snap.TotalCount, snap.CalculatedAt)
				return data, snap.TotalCount, snap.CalculatedAt, nil
			}
		}
	}

	// 2. Cache miss: Singleflight database fallback to prevent cache stampede
	res, err := s.sf.Do(string(t), func() (any, error) {
		snap, err := s.repo.GetSnapshot(ctx, t)
		if err != nil {
			return nil, err
		}
		data, err := unmarshalSnapshotData(t, snap.SnapshotData)
		if err != nil {
			return nil, fmt.Errorf("decode ranking snapshot %s: %w", t, err)
		}
		s.storeCache(t, data, snap.TotalCount, snap.CalculatedAt)
		if s.valkeyCache != nil {
			_ = s.valkeyCache.Set(ctx, t, snap, s.cacheTTL)
		}
		return loadedSnapshot{
			data:         data,
			total:        snap.TotalCount,
			calculatedAt: snap.CalculatedAt,
		}, nil
	})
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	loaded, ok := res.(loadedSnapshot)
	if !ok {
		return nil, 0, time.Time{}, fmt.Errorf("unexpected loaded snapshot result %T for %s", res, t)
	}
	return loaded.data, loaded.total, loaded.calculatedAt, nil
}

func (s *Service) getCharacterRankingPage(ctx context.Context, t RankingType, limit, offset int) (RankingPage[CharacterRankingEntry], error) {
	limit, offset = NormalizePagination(limit, offset)
	page := RankingPage[CharacterRankingEntry]{
		RankingType: t,
		Entries:     []CharacterRankingEntry{},
		Limit:       limit,
		Offset:      offset,
	}
	data, total, calcTime, err := s.getOrLoadSnapshot(ctx, t)
	if err != nil {
		if errors.Is(err, ErrSnapshotNotFound) {
			return page, nil
		}
		return RankingPage[CharacterRankingEntry]{}, err
	}
	list, ok := data.([]CharacterRankingEntry)
	if !ok {
		return RankingPage[CharacterRankingEntry]{}, fmt.Errorf("unexpected snapshot data type %T for %s", data, t)
	}
	page.Entries, page.Total, page.CalculatedAt, page.IsSnapshot = paginateSlice(list, limit, offset), total, calcTime, true
	return page, nil
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
