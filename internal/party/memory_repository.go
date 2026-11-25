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
	memReadyExp     map[string]map[string]time.Time
	memLobbyExp     map[string]time.Time
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
		memReadyExp:      make(map[string]map[string]time.Time),
		memLobbyExp:      make(map[string]time.Time),
		memPartyLocks:    make(map[string]string),
	}
}

func (r *MemoryRepository) SaveParty(_ context.Context, p Party) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveLocked(p)
	return nil
}

func (r *MemoryRepository) saveLocked(p Party) {
	now := time.Now().UTC()
	r.memLobbies[p.ID] = p
	r.memLobbyExp[p.ID] = now.Add(r.lobbyTTL)
	if p.LeaderCharacterID != "" {
		r.memCharParty[p.LeaderCharacterID] = p.ID
	}
	if _, exists := r.memMembers[p.ID]; !exists {
		r.memMembers[p.ID] = make(map[string]Member)
	}
	if _, exists := r.memReadyExp[p.ID]; !exists {
		r.memReadyExp[p.ID] = make(map[string]time.Time)
	}
}

func (r *MemoryRepository) GetParty(_ context.Context, id string) (Party, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	exp, ok := r.memLobbyExp[id]
	if !ok || time.Now().UTC().After(exp) {
		return Party{}, ErrNotFound
	}
	p, ok := r.memLobbies[id]
	if !ok || p.Status == StatusDisbanded {
		return Party{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) ListParties(_ context.Context, status string, limit, offset int) ([]PartySummary, int, error) {
	limit, offset = pagination.NormalizeWithDefaults(limit, offset, 50, 100)

	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().UTC()
	var activeList []PartySummary
	for id, p := range r.memLobbies {
		exp, ok := r.memLobbyExp[id]
		if !ok || now.After(exp) {
			continue
		}
		if status != "" && p.Status != status {
			continue
		}
		var leaderName string
		memCount := len(r.memMembers[id])
		for _, m := range r.memMembers[id] {
			if m.IsLeader {
				leaderName = m.CharacterName
				break
			}
		}
		activeList = append(activeList, newSummary(p, leaderName, memCount))
	}

	// Sort newest first
	sort.Slice(activeList, func(i, j int) bool {
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
	if !ok || time.Now().UTC().After(exp) {
		return ErrNotFound
	}
	if _, ok := r.memLobbies[p.ID]; !ok {
		return ErrNotFound
	}
	r.memLobbies[p.ID] = p
	r.memLobbyExp[p.ID] = time.Now().UTC().Add(r.lobbyTTL)
	return nil
}

func (r *MemoryRepository) DeleteParty(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.memLobbies, id)
	delete(r.memLobbyExp, id)
	if members, ok := r.memMembers[id]; ok {
		for charID := range members {
			delete(r.memCharParty, charID)
		}
		delete(r.memMembers, id)
	}
	delete(r.memReadyExp, id)
	return nil
}

func (r *MemoryRepository) AddMember(_ context.Context, m Member) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.memLobbyExp[m.PartyID]
	if !ok || time.Now().UTC().After(exp) {
		return ErrNotFound
	}
	p, ok := r.memLobbies[m.PartyID]
	if !ok || p.Status == StatusDisbanded {
		return ErrNotFound
	}
	if _, ok := r.memMembers[m.PartyID]; !ok {
		r.memMembers[m.PartyID] = make(map[string]Member)
	}
	if _, ok := r.memReadyExp[m.PartyID]; !ok {
		r.memReadyExp[m.PartyID] = make(map[string]time.Time)
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
	if m.ReadyState {
		r.memReadyExp[m.PartyID][m.CharacterID] = time.Now().UTC().Add(r.readyTTL)
	} else {
		delete(r.memReadyExp[m.PartyID], m.CharacterID)
	}
	r.memLobbyExp[m.PartyID] = time.Now().UTC().Add(r.lobbyTTL)
	return nil
}

func (r *MemoryRepository) GetMembers(_ context.Context, partyID string) ([]Member, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now().UTC()
	membersMap, ok := r.memMembers[partyID]
	if !ok {
		return nil, nil
	}
	var list []Member
	for _, m := range membersMap {
		m.BattleActor = cloneBattleActor(m.BattleActor)
		if readyExp, ok := r.memReadyExp[partyID][m.CharacterID]; ok && now.Before(readyExp) {
			m.ReadyState = true
		} else {
			m.ReadyState = false
		}
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
	if !ok || time.Now().UTC().After(exp) {
		return Party{}, Member{}, ErrNotFound
	}
	p, ok := r.memLobbies[partyID]
	if !ok || p.Status == StatusDisbanded {
		return Party{}, Member{}, ErrNotFound
	}
	m, ok := r.memMembers[partyID][characterID]
	if !ok {
		return Party{}, Member{}, ErrNotFound
	}
	m.BattleActor = cloneBattleActor(m.BattleActor)
	if readyExp, ok := r.memReadyExp[partyID][characterID]; ok && time.Now().UTC().Before(readyExp) {
		m.ReadyState = true
	} else {
		m.ReadyState = false
	}
	return p, m, nil
}

func (r *MemoryRepository) RemoveMember(_ context.Context, partyID, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.memLobbyExp[partyID]
	if !ok || time.Now().UTC().After(exp) {
		return ErrNotFound
	}
	if _, ok := r.memLobbies[partyID]; !ok {
		return ErrNotFound
	}
	if members, ok := r.memMembers[partyID]; ok {
		delete(members, characterID)
	}
	delete(r.memCharParty, characterID)
	if readyMap, ok := r.memReadyExp[partyID]; ok {
		delete(readyMap, characterID)
	}
	r.memLobbyExp[partyID] = time.Now().UTC().Add(r.lobbyTTL)
	return nil
}

func (r *MemoryRepository) UpdateMemberReady(_ context.Context, partyID, characterID string, ready bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.memLobbyExp[partyID]
	if !ok || time.Now().UTC().After(exp) {
		return ErrNotFound
	}
	members, ok := r.memMembers[partyID]
	if !ok {
		return ErrNotFound
	}
	m, ok := members[characterID]
	if !ok {
		return ErrCharacterNotInParty
	}
	m.ReadyState = ready
	members[characterID] = m
	if _, ok := r.memReadyExp[partyID]; !ok {
		r.memReadyExp[partyID] = make(map[string]time.Time)
	}
	if ready {
		r.memReadyExp[partyID][characterID] = time.Now().UTC().Add(r.readyTTL)
	} else {
		delete(r.memReadyExp[partyID], characterID)
	}
	r.memLobbyExp[partyID] = time.Now().UTC().Add(r.lobbyTTL)
	return nil
}

func (r *MemoryRepository) GetPartyForUpdate(ctx context.Context, id string) (Party, error) {
	return r.GetParty(ctx, id)
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

func (r *MemoryRepository) CountMembers(ctx context.Context, partyID string) (int, error) {
	members, err := r.GetMembers(ctx, partyID)
	if err != nil {
		return 0, err
	}
	return len(members), nil
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
	delete(r.memLobbies, id)
	delete(r.memLobbyExp, id)
	delete(r.memMembers, id)
	delete(r.memReadyExp, id)
	return nil
}

var _ Repository = (*MemoryRepository)(nil)
