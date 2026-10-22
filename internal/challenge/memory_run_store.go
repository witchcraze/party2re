package challenge

import (
	"context"
	"slices"
	"time"
)

func cloneRun(run ChallengeSession) ChallengeSession {
	run.Members = cloneMembers(run.Members)
	run.AccumulatedItems = slices.Clone(run.AccumulatedItems)
	run.TreasureAwards = slices.Clone(run.TreasureAwards)
	return run
}

func (m *MemorySessionRepository) ReadRun(ctx context.Context, id string) (*ChallengeSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.runs[id]
	if !ok || run.Status != StatusActive || !time.Now().Before(m.runExpiry[id]) {
		return nil, nil
	}
	cp := cloneRun(run)
	return &cp, nil
}
func (m *MemorySessionRepository) PublishRun(ctx context.Context, t Transition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.Publication == nil || t.Publication.ID != t.RunID || t.Publication.Revision != t.Revision {
		return ErrRevisionConflict
	}
	if t.LegacyOwner != "" {
		if old, ok := m.sessions[t.LegacyOwner]; ok && old.ID != t.RunID {
			return ErrSessionIDMismatch
		}
	}
	current := m.runs[t.RunID]
	if current.Revision == t.Revision && (EncodeJSON(current) != EncodeJSON(*t.Publication) || !m.runExpiry[t.RunID].Equal(t.Deadline)) {
		return ErrRevisionConflict
	}
	if current.Revision > t.Revision {
		return ErrRevisionConflict
	}
	if t.Publication.Status == StatusActive && time.Now().Before(t.Deadline) {
		m.runs[t.RunID] = cloneRun(*t.Publication)
		m.runExpiry[t.RunID] = t.Deadline
		for _, member := range t.Publication.Members {
			if old := m.actorRuns[member.CharacterID]; old != "" && old != t.RunID {
				return ErrActiveSessionExists
			}
			m.actorRuns[member.CharacterID] = t.RunID
		}
	} else {
		m.runs[t.RunID] = cloneRun(*t.Publication)
		m.runExpiry[t.RunID] = t.Deadline
	}
	for _, actor := range t.RemovedIDs {
		if m.actorRuns[actor] == t.RunID {
			delete(m.actorRuns, actor)
		}
	}
	if t.LegacyOwner != "" {
		if m.sessions[t.LegacyOwner].ID == t.RunID {
			delete(m.sessions, t.LegacyOwner)
			delete(m.expirations, t.LegacyOwner)
		}
		for actor, owner := range m.members {
			if owner == t.LegacyOwner {
				delete(m.members, actor)
			}
		}
	}
	return nil
}
func (m *MemorySessionRepository) ReadLegacyRun(ctx context.Context, owner string) (*ChallengeSession, time.Time, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[owner]
	deadline := m.expirations[owner]
	if !ok || !time.Now().Before(deadline) {
		return nil, time.Time{}, ErrRecoveryRequired
	}
	cp := cloneRun(s)
	return &cp, deadline, nil
}

func (m *MemorySessionRepository) SessionTTL() time.Duration { return m.ttl }
