package party

import (
	"context"
	"sort"
	"time"
)

func (r *MemoryRepository) DueLobbies(_ context.Context, now time.Time, limit int) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var ids []string
	for id, score := range r.memLobbyDue {
		if score < now.UnixMilli() {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if r.memLobbyDue[ids[i]] == r.memLobbyDue[ids[j]] {
			return ids[i] < ids[j]
		}
		return r.memLobbyDue[ids[i]] < r.memLobbyDue[ids[j]]
	})
	return ids[:min(limit, len(ids))], nil
}

func (r *MemoryRepository) BeginLobbyTimeout(_ context.Context, id string, now time.Time) ([]Member, time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.memLobbies[id]
	if !ok || !r.clock().Before(r.memLobbyExp[id]) {
		r.deleteLobbyLocked(id)
		return nil, time.Time{}, nil
	}
	if p.Status != StatusRecruiting {
		return nil, time.Time{}, ErrPartyNotRecruiting
	}
	if p.idleAt >= now.UnixMilli() {
		return nil, time.Time{}, nil
	}
	if p.sleepUntil == 0 {
		p.sleepUntil = now.Add(lobbyIdleSleep).UnixMilli()
		r.memLobbies[id] = p
		r.memLobbyExp[id] = r.clock().Add(r.lobbyTTL)
	}
	members := make([]Member, 0, len(r.memMembers[id]))
	for _, member := range r.memMembers[id] {
		members = append(members, member)
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].JoinedAt.Before(members[j].JoinedAt)
	})
	return members, time.UnixMilli(p.sleepUntil).UTC(), nil
}

func (r *MemoryRepository) RetryLobbyTimeout(_ context.Context, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if score, ok := r.memLobbyDue[id]; ok && score < now.UnixMilli() {
		r.memLobbyDue[id] = now.UnixMilli()
	}
	return nil
}
