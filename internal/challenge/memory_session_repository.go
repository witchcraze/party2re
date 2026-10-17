package challenge

import (
	"context"
	"sync"
	"time"
)

// MemorySessionRepository provides an in-memory, thread-safe implementation
// of ActiveSessionStore matching the exact semantics and error codes of the Valkey Lua script.
type MemorySessionRepository struct {
	mu           sync.RWMutex
	sessions     map[string]ChallengeSession
	expirations  map[string]time.Time
	memberLeader map[string]string
	ttl          time.Duration
}

// NewMemorySessionRepository creates a new thread-safe in-memory store.
func NewMemorySessionRepository(opts ...ValkeyOption) *MemorySessionRepository {
	repo := &MemorySessionRepository{
		sessions:     make(map[string]ChallengeSession),
		expirations:  make(map[string]time.Time),
		memberLeader: make(map[string]string),
		ttl:          DefaultSessionTTL,
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
	if ok {
		expTime, hasExp := m.expirations[characterID]
		if hasExp && time.Now().UTC().After(expTime) {
			m.deleteSessionLocked(characterID)
			return nil, nil
		}

		cp := s
		cp.AccumulatedItems = append([]string(nil), s.AccumulatedItems...)
		cp.Members = append([]ChallengeMember(nil), s.Members...)
		return &cp, nil
	}

	leaderID, hasLeader := m.memberLeader[characterID]
	if !hasLeader {
		return nil, nil
	}

	s, ok = m.sessions[leaderID]
	if !ok {
		delete(m.memberLeader, characterID)
		return nil, nil
	}

	expTime, hasExp := m.expirations[leaderID]
	if hasExp && time.Now().UTC().After(expTime) {
		m.deleteSessionLocked(leaderID)
		return nil, nil
	}

	// Privacy check: verify characterID is an authorized member of this session
	found := false
	for _, mem := range s.Members {
		if mem.CharacterID == characterID {
			found = true
			break
		}
	}
	if !found {
		delete(m.memberLeader, characterID)
		return nil, nil
	}

	cp := s
	cp.AccumulatedItems = append([]string(nil), s.AccumulatedItems...)
	cp.Members = append([]ChallengeMember(nil), s.Members...)
	return &cp, nil
}

func (m *MemorySessionRepository) SaveActiveSession(ctx context.Context, session ChallengeSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := session
	cp.AccumulatedItems = append([]string(nil), session.AccumulatedItems...)
	cp.Members = append([]ChallengeMember(nil), session.Members...)
	m.sessions[session.CharacterID] = cp
	m.expirations[session.CharacterID] = time.Now().UTC().Add(m.ttl)

	for _, mem := range session.Members {
		if mem.CharacterID != session.CharacterID {
			m.memberLeader[mem.CharacterID] = session.CharacterID
		}
	}
	return nil
}

func (m *MemorySessionRepository) DeleteActiveSession(ctx context.Context, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[characterID]; ok {
		m.deleteSessionLocked(characterID)
		return nil
	}
	if leaderID, ok := m.memberLeader[characterID]; ok {
		m.deleteSessionLocked(leaderID)
		return nil
	}
	return nil
}

func (m *MemorySessionRepository) deleteSessionLocked(leaderID string) {
	if s, ok := m.sessions[leaderID]; ok {
		for _, mem := range s.Members {
			delete(m.memberLeader, mem.CharacterID)
		}
	}
	delete(m.sessions, leaderID)
	delete(m.expirations, leaderID)
}

func (m *MemorySessionRepository) AdvanceRound(ctx context.Context, characterID string, params AdvanceRoundParams) (AdvanceRoundOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	targetID := characterID
	if _, ok := m.sessions[targetID]; !ok {
		if lid, hasLeader := m.memberLeader[targetID]; hasLeader {
			targetID = lid
		}
	}

	s, ok := m.sessions[targetID]
	if !ok {
		return AdvanceRoundOutcome{}, ErrSessionNotFound
	}

	expTime, hasExp := m.expirations[targetID]
	if hasExp && time.Now().UTC().After(expTime) {
		m.deleteSessionLocked(targetID)
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

	m.sessions[targetID] = s
	m.expirations[targetID] = time.Now().UTC().Add(m.ttl)

	cp := s
	cp.AccumulatedItems = append([]string(nil), s.AccumulatedItems...)
	cp.Members = append([]ChallengeMember(nil), s.Members...)
	return AdvanceRoundOutcome{
		Session: cp,
		Status:  cp.Status,
	}, nil
}
