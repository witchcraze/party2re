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

type pairCacheData struct {
	leader         []CharacterRankingEntry
	leaderTotal    int
	leaderCalc     time.Time
	dependent      []CharacterRankingEntry
	dependentTotal int
	dependentCalc  time.Time
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
	refreshMu   sync.Mutex
	mu          sync.RWMutex
	cache       map[string]cacheEntry
	refreshSeq  map[string]uint64
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
		repo:       repo,
		nowFunc:    func() time.Time { return time.Now().UTC() },
		cacheTTL:   DefaultCacheTTL,
		cache:      make(map[string]cacheEntry),
		refreshSeq: make(map[string]uint64),
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

func pairCacheKey(pair boardPairSpec) string {
	return "pair:" + string(pair.leaderType) + ":" + string(pair.dependentType)
}

func (s *Service) readFreshCache(key string) (cacheEntry, uint64, bool) {
	now := s.nowFunc().UTC()
	s.mu.RLock()
	cached, exists := s.cache[key]
	seq := s.refreshSeq[key]
	s.mu.RUnlock()
	if exists && cached.expiresAt.After(now) {
		return cached, seq, true
	}
	return cacheEntry{}, seq, false
}

func (s *Service) storeRefreshedCache(key string, data any, total int, updatedAt time.Time) {
	now := s.nowFunc().UTC()
	s.mu.Lock()
	s.refreshSeq[key]++
	if existing, ok := s.cache[key]; ok && existing.updatedAt.After(updatedAt) {
		s.mu.Unlock()
		return
	}
	s.cache[key] = cacheEntry{
		data:      data,
		total:     total,
		expiresAt: now.Add(s.cacheTTL),
		updatedAt: updatedAt,
	}
	s.mu.Unlock()
}

func (s *Service) storeBackfillCache(key string, seqBefore uint64, data any, total int, updatedAt time.Time) cacheEntry {
	now := s.nowFunc().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.cache[key]; ok {
		if s.refreshSeq[key] != seqBefore {
			if existing.expiresAt.After(now) {
				return existing
			}
			// A refresh occurred while fallback was in flight; do not overwrite cache with stale data.
			return cacheEntry{data: data, total: total, updatedAt: updatedAt}
		}
		if existing.updatedAt.After(updatedAt) {
			return existing
		}
	} else if s.refreshSeq[key] != seqBefore {
		return cacheEntry{data: data, total: total, updatedAt: updatedAt}
	}

	entry := cacheEntry{
		data:      data,
		total:     total,
		expiresAt: now.Add(s.cacheTTL),
		updatedAt: updatedAt,
	}
	s.cache[key] = entry
	return entry
}

func (s *Service) publishSingleSnapshot(ctx context.Context, snap RankingSnapshot, data any) {
	if s.valkeyCache != nil {
		_, _ = s.valkeyCache.Set(ctx, snap.RankingType, snap, s.cacheTTL, CacheWriteRefresh)
	}
	s.storeRefreshedCache(string(snap.RankingType), data, snap.TotalCount, snap.CalculatedAt)
}

func (s *Service) publishPairSnapshots(ctx context.Context, pair boardPairSpec, leader, dependent preparedSnapshot) {
	if s.valkeyCache != nil {
		_, _ = s.valkeyCache.SetPair(ctx, SnapshotPair{
			Leader:    leader.snapshot,
			Dependent: dependent.snapshot,
		}, s.cacheTTL, CacheWriteRefresh)
	}
	leaderList, _ := leader.data.([]CharacterRankingEntry)
	depList, _ := dependent.data.([]CharacterRankingEntry)
	pairData := pairCacheData{
		leader:         leaderList,
		leaderTotal:    leader.snapshot.TotalCount,
		leaderCalc:     leader.snapshot.CalculatedAt,
		dependent:      depList,
		dependentTotal: dependent.snapshot.TotalCount,
		dependentCalc:  dependent.snapshot.CalculatedAt,
	}
	s.storeRefreshedCache(pairCacheKey(pair), pairData, 0, laterTime(leader.snapshot.CalculatedAt, dependent.snapshot.CalculatedAt))
}

type loadedSnapshot struct {
	data         any
	total        int
	calculatedAt time.Time
}

func decodePairSnapshot(pair boardPairSpec, sp SnapshotPair) (pairCacheData, time.Time, error) {
	if sp.Leader.SnapshotData == "" {
		return pairCacheData{}, time.Time{}, fmt.Errorf("decode ranking snapshot %s: empty snapshot_data", pair.leaderType)
	}
	if sp.Dependent.SnapshotData == "" {
		return pairCacheData{}, time.Time{}, fmt.Errorf("decode ranking snapshot %s: empty snapshot_data", pair.dependentType)
	}
	leaderRaw, err := unmarshalSnapshotData(pair.leaderType, sp.Leader.SnapshotData)
	if err != nil {
		return pairCacheData{}, time.Time{}, fmt.Errorf("decode ranking snapshot %s: %w", pair.leaderType, err)
	}
	depRaw, err := unmarshalSnapshotData(pair.dependentType, sp.Dependent.SnapshotData)
	if err != nil {
		return pairCacheData{}, time.Time{}, fmt.Errorf("decode ranking snapshot %s: %w", pair.dependentType, err)
	}
	leaderList, ok := leaderRaw.([]CharacterRankingEntry)
	if !ok {
		return pairCacheData{}, time.Time{}, fmt.Errorf("unexpected snapshot data type %T for %s", leaderRaw, pair.leaderType)
	}
	depList, ok := depRaw.([]CharacterRankingEntry)
	if !ok {
		return pairCacheData{}, time.Time{}, fmt.Errorf("unexpected snapshot data type %T for %s", depRaw, pair.dependentType)
	}
	return pairCacheData{
		leader:         leaderList,
		leaderTotal:    sp.Leader.TotalCount,
		leaderCalc:     sp.Leader.CalculatedAt,
		dependent:      depList,
		dependentTotal: sp.Dependent.TotalCount,
		dependentCalc:  sp.Dependent.CalculatedAt,
	}, laterTime(sp.Leader.CalculatedAt, sp.Dependent.CalculatedAt), nil
}

func extractPairBoard(data pairCacheData, pair boardPairSpec, t RankingType) (any, int, time.Time) {
	if t == pair.leaderType {
		return data.leader, data.leaderTotal, data.leaderCalc
	}
	return data.dependent, data.dependentTotal, data.dependentCalc
}

func (s *Service) getOrLoadSnapshot(ctx context.Context, t RankingType) (any, int, time.Time, error) {
	if pair, ok := exclusionPairFor(t); ok {
		return s.getOrLoadPairSnapshot(ctx, pair, t)
	}
	return s.getOrLoadSingleSnapshot(ctx, t)
}

func (s *Service) getOrLoadPairSnapshot(ctx context.Context, pair boardPairSpec, t RankingType) (any, int, time.Time, error) {
	key := pairCacheKey(pair)
	if cached, _, ok := s.readFreshCache(key); ok {
		if pairData, ok := cached.data.(pairCacheData); ok {
			data, total, calc := extractPairBoard(pairData, pair, t)
			return data, total, calc, nil
		}
	}

	res, err := s.sf.Do(key, func() (any, error) {
		if cached, _, ok := s.readFreshCache(key); ok {
			if pairData, ok := cached.data.(pairCacheData); ok {
				return pairData, nil
			}
		}

		// Capture refresh sequence before checking external stores so a concurrent refresh wins.
		_, seqBefore, _ := s.readFreshCache(key)

		if s.valkeyCache != nil {
			if sp, found, err := s.valkeyCache.GetPair(ctx, pair.leaderType, pair.dependentType); err == nil && found {
				if pairData, updatedAt, err := decodePairSnapshot(pair, sp); err == nil {
					stored := s.storeBackfillCache(key, seqBefore, pairData, 0, updatedAt)
					if pd, ok := stored.data.(pairCacheData); ok {
						return pd, nil
					}
					return pairData, nil
				}
			}
		}

		all, err := s.repo.GetAllSnapshots(ctx)
		if err != nil {
			return nil, err
		}
		leaderSnap, okL := all[pair.leaderType]
		depSnap, okD := all[pair.dependentType]
		if !okL || !okD {
			return nil, ErrSnapshotNotFound
		}

		sp := SnapshotPair{Leader: leaderSnap, Dependent: depSnap}
		pairData, updatedAt, err := decodePairSnapshot(pair, sp)
		if err != nil {
			return nil, err
		}

		if s.valkeyCache != nil {
			written, setErr := s.valkeyCache.SetPair(ctx, sp, s.cacheTTL, CacheWriteBackfill)
			if setErr == nil && !written {
				if reloaded, found, getErr := s.valkeyCache.GetPair(ctx, pair.leaderType, pair.dependentType); getErr == nil && found {
					if newerData, newerUpdatedAt, decErr := decodePairSnapshot(pair, reloaded); decErr == nil {
						pairData = newerData
						updatedAt = newerUpdatedAt
					}
				}
			}
		}

		stored := s.storeBackfillCache(key, seqBefore, pairData, 0, updatedAt)
		if pd, ok := stored.data.(pairCacheData); ok {
			return pd, nil
		}
		return pairData, nil
	})
	if err != nil {
		return nil, 0, time.Time{}, err
	}

	pairData, ok := res.(pairCacheData)
	if !ok {
		return nil, 0, time.Time{}, fmt.Errorf("unexpected loaded pair snapshot result %T for %s", res, t)
	}
	data, total, calc := extractPairBoard(pairData, pair, t)
	return data, total, calc, nil
}

func (s *Service) getOrLoadSingleSnapshot(ctx context.Context, t RankingType) (any, int, time.Time, error) {
	key := string(t)
	if cached, _, ok := s.readFreshCache(key); ok {
		return cached.data, cached.total, cached.updatedAt, nil
	}

	res, err := s.sf.Do(key, func() (any, error) {
		if cached, _, ok := s.readFreshCache(key); ok {
			return loadedSnapshot{
				data:         cached.data,
				total:        cached.total,
				calculatedAt: cached.updatedAt,
			}, nil
		}

		_, seqBefore, _ := s.readFreshCache(key)

		if s.valkeyCache != nil {
			if snap, found, err := s.valkeyCache.Get(ctx, t); err == nil && found && snap.SnapshotData != "" {
				if data, err := unmarshalSnapshotData(t, snap.SnapshotData); err == nil {
					stored := s.storeBackfillCache(key, seqBefore, data, snap.TotalCount, snap.CalculatedAt)
					return loadedSnapshot{
						data:         stored.data,
						total:        stored.total,
						calculatedAt: stored.updatedAt,
					}, nil
				}
			}
		}

		snap, err := s.repo.GetSnapshot(ctx, t)
		if err != nil {
			return nil, err
		}
		data, err := unmarshalSnapshotData(t, snap.SnapshotData)
		if err != nil {
			return nil, fmt.Errorf("decode ranking snapshot %s: %w", t, err)
		}
		total := snap.TotalCount
		calcAt := snap.CalculatedAt

		if s.valkeyCache != nil {
			written, setErr := s.valkeyCache.Set(ctx, t, snap, s.cacheTTL, CacheWriteBackfill)
			if setErr == nil && !written {
				if reloaded, found, getErr := s.valkeyCache.Get(ctx, t); getErr == nil && found && reloaded.SnapshotData != "" {
					if newerData, decErr := unmarshalSnapshotData(t, reloaded.SnapshotData); decErr == nil {
						data = newerData
						total = reloaded.TotalCount
						calcAt = reloaded.CalculatedAt
					}
				}
			}
		}

		stored := s.storeBackfillCache(key, seqBefore, data, total, calcAt)
		return loadedSnapshot{
			data:         stored.data,
			total:        stored.total,
			calculatedAt: stored.updatedAt,
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
