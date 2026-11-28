package party

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
)

// MemoryRepository stores party lobbies for tests that explicitly inject it.
type MemoryRepository struct {
	repositoryConfig
	mu              sync.RWMutex
	memLobbies      map[string]Party
	memMembers      map[string]map[string]Member
	memCharParty    map[string]string
	memLobbyExp     map[string]time.Time
	memLobbyDue     map[string]int64
	memPartyLocksMu sync.Mutex
	memPartyLocks   map[string]string
}

// NewMemoryRepository creates an in-memory lobby repository for tests.
func NewMemoryRepository(opts ...ValkeyRepositoryOption) *MemoryRepository {
	return &MemoryRepository{
		repositoryConfig: newRepositoryConfig(opts),
		memLobbies:       make(map[string]Party),
		memMembers:       make(map[string]map[string]Member),
		memCharParty:     make(map[string]string),
		memLobbyExp:      make(map[string]time.Time),
		memLobbyDue:      make(map[string]int64),
		memPartyLocks:    make(map[string]string),
	}
}

func (r *MemoryRepository) SaveParty(_ context.Context, p Party, members ...Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clock().Before(r.memLobbyExp[p.ID]) {
		return nil
	}
	for _, member := range members {
		if r.hasOtherLobbyLocked(p.ID, member.CharacterID) {
			return ErrAlreadyInParty
		}
	}
	r.memMembers[p.ID] = make(map[string]Member, len(members))
	for _, member := range members {
		member.BattleActor = cloneBattleActor(member.BattleActor)
		r.memMembers[p.ID][member.CharacterID] = member
		r.memCharParty[member.CharacterID] = p.ID
	}
	r.touchLocked(p)
	return nil
}

func (r *MemoryRepository) hasOtherLobbyLocked(id, actor string) bool {
	owner := r.memCharParty[actor]
	return owner != id && r.clock().Before(r.memLobbyExp[owner])
}

func (r *MemoryRepository) touchLocked(p Party) {
	now := r.clock()
	p.idleAt = now.Add(lobbyIdleTimeout).UnixMilli()
	r.memLobbies[p.ID] = p
	r.memLobbyExp[p.ID] = now.Add(r.lobbyTTL)
	if p.Status == StatusCompleted {
		delete(r.memLobbyDue, p.ID)
	} else {
		r.memLobbyDue[p.ID] = p.idleAt
	}
	for charID := range r.memMembers[p.ID] {
		if owner := r.memCharParty[charID]; owner == "" || owner == p.ID {
			r.memCharParty[charID] = p.ID
		}
	}
}

func (r *MemoryRepository) GetParty(_ context.Context, id string) (Party, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	exp, ok := r.memLobbyExp[id]
	if !ok || !r.clock().Before(exp) {
		return Party{}, ErrNotFound
	}
	p, ok := r.memLobbies[id]
	if !ok {
		return Party{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) ListParties(_ context.Context, status string, limit, offset int) ([]PartySummary, int, error) {
	limit, offset = pagination.NormalizeWithDefaults(limit, offset, 50, 100)

	r.mu.RLock()
	defer r.mu.RUnlock()

	now := r.clock()
	var activeList []PartySummary
	for id, p := range r.memLobbies {
		exp, ok := r.memLobbyExp[id]
		if !ok || !now.Before(exp) {
			continue
		}
		if p.sleepUntil != 0 || (status != "" && p.Status != status) {
			continue
		}
		members := make([]Member, 0, len(r.memMembers[id]))
		for _, m := range r.memMembers[id] {
			members = append(members, m)
		}
		sort.Slice(members, func(i, j int) bool { return members[i].JoinedAt.Before(members[j].JoinedAt) })
		activeList = append(activeList, newSummary(p, members))
	}

	// Sort newest first
	sort.Slice(activeList, func(i, j int) bool {
		if activeList[i].CreatedAt.Equal(activeList[j].CreatedAt) {
			return activeList[i].ID > activeList[j].ID
		}
		return activeList[i].CreatedAt.After(activeList[j].CreatedAt)
	})

	total := len(activeList)
	if offset >= total {
		return []PartySummary{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return activeList[offset:end], total, nil
}

func (r *MemoryRepository) UpdateParty(_ context.Context, p Party) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.memLobbyExp[p.ID]
	if !ok || !r.clock().Before(exp) {
		return ErrNotFound
	}
	if r.memLobbies[p.ID].sleepUntil != 0 {
		return ErrPartyNotRecruiting
	}
	r.touchLocked(p)
	return nil
}

func (r *MemoryRepository) DeleteParty(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for charID := range r.memMembers[id] {
		if r.memCharParty[charID] == id {
			delete(r.memCharParty, charID)
		}
	}
	r.deleteLobbyLocked(id)
	return nil
}

func (r *MemoryRepository) deleteLobbyLocked(id string) {
	delete(r.memLobbies, id)
	delete(r.memLobbyExp, id)
	delete(r.memLobbyDue, id)
	delete(r.memMembers, id)
}

func (r *MemoryRepository) AddMember(_ context.Context, m Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.memLobbyExp[m.PartyID]
	if !ok || !r.clock().Before(exp) {
		return ErrNotFound
	}
	p, ok := r.memLobbies[m.PartyID]
	if !ok {
		return ErrNotFound
	}
	if p.sleepUntil != 0 {
		return ErrPartyNotRecruiting
	}
	if r.hasOtherLobbyLocked(m.PartyID, m.CharacterID) {
		return ErrAlreadyInParty
	}
	if _, ok := r.memMembers[m.PartyID]; !ok {
		r.memMembers[m.PartyID] = make(map[string]Member)
	}

	maxMembers := p.MaxMembers
	if maxMembers <= 0 {
		maxMembers = MaxPartyMembers
	}
	_, alreadyMember := r.memMembers[m.PartyID][m.CharacterID]
	if !alreadyMember && len(r.memMembers[m.PartyID]) >= maxMembers {
		return ErrPartyFull
	}

	m.BattleActor = cloneBattleActor(m.BattleActor)
	r.memMembers[m.PartyID][m.CharacterID] = m
	r.memCharParty[m.CharacterID] = m.PartyID
	r.touchLocked(p)
	return nil
}

func (r *MemoryRepository) GetMembers(_ context.Context, partyID string) ([]Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	membersMap, ok := r.memMembers[partyID]
	if !ok || !r.clock().Before(r.memLobbyExp[partyID]) {
		return nil, nil
	}
	var list []Member
	for _, m := range membersMap {
		m.BattleActor = cloneBattleActor(m.BattleActor)
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsLeader != list[j].IsLeader {
			return list[i].IsLeader
		}
		return list[i].JoinedAt.Before(list[j].JoinedAt)
	})
	return list, nil
}

func (r *MemoryRepository) GetActivePartyByCharacter(_ context.Context, characterID string) (Party, Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	partyID, ok := r.memCharParty[characterID]
	if !ok {
		return Party{}, Member{}, ErrNotFound
	}
	exp, ok := r.memLobbyExp[partyID]
	if !ok || !r.clock().Before(exp) {
		return Party{}, Member{}, ErrNotFound
	}
	p, ok := r.memLobbies[partyID]
	if !ok {
		return Party{}, Member{}, ErrNotFound
	}
	m, ok := r.memMembers[partyID][characterID]
	if !ok {
		return Party{}, Member{}, ErrNotFound
	}
	m.BattleActor = cloneBattleActor(m.BattleActor)
	return p, m, nil
}

func (r *MemoryRepository) RemoveMember(_ context.Context, partyID, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.memLobbyExp[partyID]
	if !ok || !r.clock().Before(exp) {
		return ErrNotFound
	}
	p, ok := r.memLobbies[partyID]
	if !ok {
		return ErrNotFound
	}
	if p.sleepUntil != 0 {
		return ErrPartyNotRecruiting
	}
	members := r.memMembers[partyID]
	delete(members, characterID)
	if r.memCharParty[characterID] == partyID {
		delete(r.memCharParty, characterID)
	}
	if p.LeaderCharacterID == characterID {
		// Earliest joiner leads, as the Valkey script's join order does.
		var next Member
		for _, m := range members {
			if next.CharacterID == "" || m.JoinedAt.Before(next.JoinedAt) {
				next = m
			}
		}
		if next.CharacterID != "" {
			next.IsLeader = true
			members[next.CharacterID] = next
			p.LeaderCharacterID = next.CharacterID
		}
	}
	r.touchLocked(p)
	return nil
}

func (r *MemoryRepository) GetPartyForUpdate(ctx context.Context, id string) (Party, error) {
	p, err := r.GetParty(ctx, id)
	if err == nil && p.sleepUntil != 0 {
		return Party{}, ErrPartyNotRecruiting
	}
	return p, err
}

func (r *MemoryRepository) GetMember(ctx context.Context, partyID, characterID string) (Member, error) {
	members, err := r.GetMembers(ctx, partyID)
	if err != nil {
		return Member{}, err
	}
	for _, m := range members {
		if m.CharacterID == characterID {
			return m, nil
		}
	}
	return Member{}, ErrCharacterNotInParty
}

func (r *MemoryRepository) WithPartyAdventureLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error {
	if partyID == "" {
		return fn(ctx)
	}

	if isPartyAdventureLocked(ctx, partyID) {
		return fn(ctx)
	}

	r.memPartyLocksMu.Lock()
	if _, locked := r.memPartyLocks[partyID]; locked {
		r.memPartyLocksMu.Unlock()
		return ErrPartyNotRecruiting
	}
	token := id.New()
	r.memPartyLocks[partyID] = token
	r.memPartyLocksMu.Unlock()

	defer func() {
		r.memPartyLocksMu.Lock()
		if r.memPartyLocks[partyID] == token {
			delete(r.memPartyLocks, partyID)
		}
		r.memPartyLocksMu.Unlock()
	}()

	lockedCtx := withPartyAdventureLockedContext(ctx, partyID)
	return fn(lockedCtx)
}

func (r *MemoryRepository) CleanupParty(_ context.Context, state LobbyState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := state.Party.ID
	for _, charID := range lobbyCharacterIDs(state) {
		if r.memCharParty[charID] == id {
			delete(r.memCharParty, charID)
		}
	}
	r.deleteLobbyLocked(id)
	return nil
}

var _ Repository = (*MemoryRepository)(nil)
