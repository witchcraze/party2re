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
	members     map[string]string
}

// NewMemorySessionRepository creates a new thread-safe in-memory store.
func NewMemorySessionRepository(opts ...ValkeyOption) *MemorySessionRepository {
	repo := &MemorySessionRepository{
		sessions:    make(map[string]ChallengeSession),
		expirations: make(map[string]time.Time),
		members:     make(map[string]string),
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

	if expires, ok := m.expirations[characterID]; ok && time.Now().After(expires) {
		delete(m.sessions, characterID)
		delete(m.expirations, characterID)
	}
	owner := characterID
	if leader, ok := m.members[characterID]; ok && m.sessions[characterID].ID == "" {
		owner = leader
	}
	s, ok := m.sessions[owner]
	if ok && !s.ContainsCharacter(characterID) {
		return nil, nil
	}
	if !ok {
		return nil, nil
	}

	expTime, hasExp := m.expirations[owner]
	if hasExp && time.Now().UTC().After(expTime) {
		delete(m.sessions, owner)
		delete(m.expirations, owner)
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
	for _, member := range session.Members {
		if member.CharacterID != session.CharacterID {
			m.members[member.CharacterID] = session.CharacterID
		}
	}
	m.expirations[session.CharacterID] = time.Now().UTC().Add(m.ttl)
	return nil
}

func (m *MemorySessionRepository) DeleteActiveSession(ctx context.Context, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if leader, ok := m.members[characterID]; ok && leader != characterID && m.sessions[characterID].ID == "" {
		if run, exists := m.sessions[leader]; exists && run.ContainsCharacter(characterID) && time.Now().Before(m.expirations[leader]) {
			return ErrForbidden
		}
	}

	run, ok := m.sessions[characterID]
	delete(m.sessions, characterID)
	delete(m.expirations, characterID)

	if ok {
		for _, member := range run.Members {
			if m.members[member.CharacterID] == characterID {
				delete(m.members, member.CharacterID)
			}
		}
	}
	return nil
}

// DiscardActiveSession atomically deletes the active challenge session if and only if
// the ID matches session.ID, preserving any replacement session and its member mappings.
func (m *MemorySessionRepository) DiscardActiveSession(ctx context.Context, session ChallengeSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, ok := m.sessions[session.CharacterID]
	if ok && current.ID == session.ID {
		delete(m.sessions, session.CharacterID)
		delete(m.expirations, session.CharacterID)
	}

	for _, member := range session.Members {
		if member.CharacterID == session.CharacterID {
			continue
		}
		if leader, ok := m.members[member.CharacterID]; ok && leader == session.CharacterID {
			if replacement, exists := m.sessions[session.CharacterID]; exists && replacement.ID != session.ID && replacement.ContainsCharacter(member.CharacterID) {
				continue
			}
			delete(m.members, member.CharacterID)
		}
	}
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
	cp.Members = append([]ChallengeMember(nil), s.Members...)
	return AdvanceRoundOutcome{
		Session: cp,
		Status:  cp.Status,
	}, nil
}
