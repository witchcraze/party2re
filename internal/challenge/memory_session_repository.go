package challenge

import (
	"context"
	"sync"
	"time"
)

// MemorySessionRepository provides an in-memory, thread-safe implementation
// of ActiveSessionStore matching the exact semantics and error codes of the Valkey Lua script.
type MemorySessionRepository struct {
	mu          sync.RWMutex
	sessions    map[string]ChallengeSession
	expirations map[string]time.Time
	ttl         time.Duration
}

// NewMemorySessionRepository creates a new thread-safe in-memory store.
func NewMemorySessionRepository(opts ...ValkeyOption) *MemorySessionRepository {
	repo := &MemorySessionRepository{
		sessions:    make(map[string]ChallengeSession),
		expirations: make(map[string]time.Time),
		ttl:         DefaultSessionTTL,
	}
	for _, opt := range opts {
		cfg := &valkeyConfig{ttl: repo.ttl}
		opt(cfg)
		repo.ttl = cfg.ttl
	}
	return repo
}

func (m *MemorySessionRepository) GetActiveSession(ctx context.Context, characterID string) (*ChallengeSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[characterID]
	if !ok {
		return nil, nil
	}

	expTime, hasExp := m.expirations[characterID]
	if hasExp && time.Now().UTC().After(expTime) {
		delete(m.sessions, characterID)
		delete(m.expirations, characterID)
		return nil, nil
	}

	cp := s
	cp.AccumulatedItems = append([]string(nil), s.AccumulatedItems...)
	return &cp, nil
}

func (m *MemorySessionRepository) SaveActiveSession(ctx context.Context, session ChallengeSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := session
	cp.AccumulatedItems = append([]string(nil), session.AccumulatedItems...)
	m.sessions[session.CharacterID] = cp
	m.expirations[session.CharacterID] = time.Now().UTC().Add(m.ttl)
	return nil
}

func (m *MemorySessionRepository) DeleteActiveSession(ctx context.Context, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, characterID)
	delete(m.expirations, characterID)
	return nil
}

func (m *MemorySessionRepository) AdvanceRound(ctx context.Context, characterID string, params AdvanceRoundParams) (AdvanceRoundOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[characterID]
	if !ok {
		return AdvanceRoundOutcome{}, ErrSessionNotFound
	}

	expTime, hasExp := m.expirations[characterID]
	if hasExp && time.Now().UTC().After(expTime) {
		delete(m.sessions, characterID)
		delete(m.expirations, characterID)
		return AdvanceRoundOutcome{}, ErrSessionNotFound
	}

	if s.ID != params.ExpectedSessionID {
		return AdvanceRoundOutcome{}, ErrSessionIDMismatch
	}

	if s.Status != StatusActive {
		return AdvanceRoundOutcome{}, ErrSessionNotActive
	}

	s.CurrentRound++
	s.CharacterCurrentHP = params.SurvivingHP
	s.AccumulatedExp += params.ExpDelta
	s.AccumulatedGold += params.GoldDelta
	if params.RewardItemID != "" {
		s.AccumulatedItems = append(s.AccumulatedItems, params.RewardItemID)
	}
	s.UpdatedAt = params.Now

	m.sessions[characterID] = s
	m.expirations[characterID] = time.Now().UTC().Add(m.ttl)

	cp := s
	cp.AccumulatedItems = append([]string(nil), s.AccumulatedItems...)
	return AdvanceRoundOutcome{
		Session: cp,
		Status:  cp.Status,
	}, nil
}
