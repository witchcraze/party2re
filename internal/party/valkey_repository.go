package party

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/pagination"
)

const (
	// DefaultLobbyKeyPrefix is the Valkey key prefix for transient party lobby state.
	DefaultLobbyKeyPrefix = "party2:party:lobby:"

	// DefaultReadyKeyPrefix is the Valkey key prefix for 60-second ready check countdowns.
	DefaultReadyKeyPrefix = "party2:party:ready:"

	// DefaultCharacterKeyPrefix is the Valkey key prefix for O(1) character-to-party mapping.
	DefaultCharacterKeyPrefix = "party2:party:character:"

	// DefaultLobbiesIndexKey is the Valkey Sorted Set key tracking active recruiting lobbies.
	DefaultLobbiesIndexKey = "party2:party:lobbies"

	// DefaultLobbyTTL is the automatic expiration time for abandoned or idle party lobbies.
	DefaultLobbyTTL = 15 * time.Minute

	// DefaultReadyTTL is the 60-second ready check countdown expiration duration.
	DefaultReadyTTL = 60 * time.Second
)

const addMemberLua = `
local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
if state.party and state.party.status == 'disbanded' then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local memberObj = cjson.decode(ARGV[1])
local charID = ARGV[2]
local maxMembers = 4
if state.party and state.party.max_members and tonumber(state.party.max_members) > 0 then
    maxMembers = tonumber(state.party.max_members)
end

if type(state.members) ~= 'table' then
    state.members = {}
end

local found = false
for i, m in ipairs(state.members) do
    if m.character_id == charID then
        state.members[i] = memberObj
        found = true
        break
    end
end

if not found then
    if #state.members >= maxMembers then
        return redis.error_reply('ERR_PARTY_FULL')
    end
    table.insert(state.members, memberObj)
end

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[4])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)
redis.call('SET', KEYS[2], ARGV[3], 'EX', lobbyTTL)

if ARGV[6] == '1' then
    local readyTTL = tonumber(ARGV[5])
    redis.call('SET', KEYS[3], '1', 'EX', readyTTL)
else
    redis.call('DEL', KEYS[3])
end

return 'OK'
`

const removeMemberLua = `
local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
local charID = ARGV[1]

local remaining = {}
if type(state.members) == 'table' then
    for _, m in ipairs(state.members) do
        if m.character_id ~= charID then
            table.insert(remaining, m)
        end
    end
end
state.members = remaining

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[2])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)

redis.call('DEL', KEYS[2])
redis.call('DEL', KEYS[3])

return 'OK'
`

const updateMemberReadyLua = `
local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
local charID = ARGV[1]
local ready = (ARGV[2] == '1')

local found = false
if type(state.members) == 'table' then
    for i, m in ipairs(state.members) do
        if m.character_id == charID then
            state.members[i].ready_state = ready
            found = true
            break
        end
    end
end

if not found then
    return redis.error_reply('ERR_CHAR_NOT_IN_PARTY')
end

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[3])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)

if ready then
    local readyTTL = tonumber(ARGV[4])
    redis.call('SET', KEYS[2], '1', 'EX', readyTTL)
else
    redis.call('DEL', KEYS[2])
end

return 'OK'
`

const updatePartyLua = `
local lobbyData = redis.call('GET', KEYS[1])
if not lobbyData then
    return redis.error_reply('ERR_PARTY_NOT_FOUND')
end

local state = cjson.decode(lobbyData)
local partyObj = cjson.decode(ARGV[1])
state.party = partyObj

local updatedData = cjson.encode(state)
local lobbyTTL = tonumber(ARGV[2])
redis.call('SET', KEYS[1], updatedData, 'EX', lobbyTTL)

local status = ARGV[3]
if status == 'disbanded' or status == 'completed' then
    redis.call('ZREM', KEYS[2], ARGV[4])
end

return 'OK'
`

// LobbyState represents the ephemeral composite state stored in Valkey Master for a party.
type LobbyState struct {
	Party   Party    `json:"party"`
	Members []Member `json:"members"`
}

// UnmarshalJSON implements json.Unmarshaler to handle cases where empty Lua cjson tables
// are serialized as "{}" instead of "[]" for Members.
func (s *LobbyState) UnmarshalJSON(data []byte) error {
	type Alias LobbyState
	aux := struct {
		Members json.RawMessage `json:"members"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Members) == 0 || string(aux.Members) == "null" {
		s.Members = []Member{}
		return nil
	}
	trimmed := bytes.TrimSpace(aux.Members)
	if bytes.Equal(trimmed, []byte("{}")) {
		s.Members = []Member{}
		return nil
	}
	return json.Unmarshal(aux.Members, &s.Members)
}

func mapLuaError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "ERR_PARTY_FULL") {
		return ErrPartyFull
	}
	if strings.Contains(errStr, "ERR_PARTY_NOT_FOUND") {
		return ErrNotFound
	}
	if strings.Contains(errStr, "ERR_CHAR_NOT_IN_PARTY") {
		return ErrCharacterNotInParty
	}
	return err
}

// ValkeyRepositoryOption configures ValkeyRepository.
type ValkeyRepositoryOption func(*ValkeyRepository)

// WithDurableLogRepository configures a delegate repository for durable adventure logs (e.g. MariaDB).
func WithDurableLogRepository(logRepo AdventureLogRepository) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		r.logRepo = logRepo
	}
}

// WithLobbyKeyPrefix overrides the default lobby key prefix.
func WithLobbyKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		if prefix != "" {
			r.lobbyPrefix = prefix
		}
	}
}

// WithReadyKeyPrefix overrides the default ready countdown key prefix.
func WithReadyKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		if prefix != "" {
			r.readyPrefix = prefix
		}
	}
}

// WithCharacterKeyPrefix overrides the default character-party index key prefix.
func WithCharacterKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		if prefix != "" {
			r.charPrefix = prefix
		}
	}
}

// WithLobbiesIndexKey overrides the default lobbies index key.
func WithLobbiesIndexKey(key string) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		if key != "" {
			r.lobbiesIndexKey = key
		}
	}
}

// WithLobbyTTL overrides the default lobby TTL.
func WithLobbyTTL(d time.Duration) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		if d > 0 {
			r.lobbyTTL = d
		}
	}
}

// WithReadyTTL overrides the default ready check countdown TTL.
func WithReadyTTL(d time.Duration) ValkeyRepositoryOption {
	return func(r *ValkeyRepository) {
		if d > 0 {
			r.readyTTL = d
		}
	}
}

// ValkeyRepository implements party.Repository by storing ephemeral party lobbies,
// member wait states, and 60-second ready checks in Valkey Master.
// When client is nil, it seamlessly falls back to a thread-safe in-memory store.
type ValkeyRepository struct {
	client  valkey.Client
	logRepo AdventureLogRepository

	lobbyPrefix     string
	readyPrefix     string
	charPrefix      string
	lobbiesIndexKey string

	lobbyTTL time.Duration
	readyTTL time.Duration

	// Lua scripts for atomic lobby state mutations
	addMemberScript         *valkey.Lua
	removeMemberScript      *valkey.Lua
	updateMemberReadyScript *valkey.Lua
	updatePartyScript       *valkey.Lua

	// In-memory fallback
	mu           sync.RWMutex
	memLobbies   map[string]Party
	memMembers   map[string]map[string]Member
	memCharParty map[string]string
	memReadyExp  map[string]map[string]time.Time
	memLobbyExp  map[string]time.Time
	memLogs      []PartyAdventureLog
}

// NewValkeyRepository constructs a new ValkeyRepository.
func NewValkeyRepository(client valkey.Client, opts ...ValkeyRepositoryOption) *ValkeyRepository {
	r := &ValkeyRepository{
		client:                  client,
		lobbyPrefix:             DefaultLobbyKeyPrefix,
		readyPrefix:             DefaultReadyKeyPrefix,
		charPrefix:              DefaultCharacterKeyPrefix,
		lobbiesIndexKey:         DefaultLobbiesIndexKey,
		lobbyTTL:                DefaultLobbyTTL,
		readyTTL:                DefaultReadyTTL,
		addMemberScript:         valkey.NewLuaScript(addMemberLua),
		removeMemberScript:      valkey.NewLuaScript(removeMemberLua),
		updateMemberReadyScript: valkey.NewLuaScript(updateMemberReadyLua),
		updatePartyScript:       valkey.NewLuaScript(updatePartyLua),
		memLobbies:              make(map[string]Party),
		memMembers:              make(map[string]map[string]Member),
		memCharParty:            make(map[string]string),
		memReadyExp:             make(map[string]map[string]time.Time),
		memLobbyExp:             make(map[string]time.Time),
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *ValkeyRepository) lobbyKey(id string) string {
	return r.lobbyPrefix + id
}

func (r *ValkeyRepository) readyKey(partyID, charID string) string {
	return r.readyPrefix + partyID + ":" + charID
}

func (r *ValkeyRepository) characterKey(charID string) string {
	return r.charPrefix + charID
}

// SaveParty stores a newly created party lobby in Valkey Master.
func (r *ValkeyRepository) SaveParty(ctx context.Context, p Party) error {
	if r.client == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
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
		return nil
	}

	state := LobbyState{
		Party:   p,
		Members: []Member{},
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	// 1. Store lobby state with TTL
	ttlSeconds := int64(r.lobbyTTL.Seconds())
	err = r.client.Do(ctx, r.client.B().Set().Key(r.lobbyKey(p.ID)).Value(string(data)).ExSeconds(ttlSeconds).Build()).Error()
	if err != nil {
		return err
	}

	// 2. Add to active lobbies sorted set (score = CreatedAt unix)
	score := float64(p.CreatedAt.Unix())
	if score <= 0 {
		score = float64(time.Now().UTC().Unix())
	}
	_ = r.client.Do(ctx, r.client.B().Zadd().Key(r.lobbiesIndexKey).ScoreMember().ScoreMember(score, p.ID).Build()).Error()

	// 3. Set leader character index
	if p.LeaderCharacterID != "" {
		_ = r.client.Do(ctx, r.client.B().Set().Key(r.characterKey(p.LeaderCharacterID)).Value(p.ID).ExSeconds(ttlSeconds).Build()).Error()
	}

	return nil
}

// GetParty retrieves party metadata from Valkey Master.
func (r *ValkeyRepository) GetParty(ctx context.Context, id string) (Party, error) {
	if r.client == nil {
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

	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(id)).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			_ = r.client.Do(ctx, r.client.B().Zrem().Key(r.lobbiesIndexKey).Member(id).Build()).Error()
			return Party{}, ErrNotFound
		}
		return Party{}, err
	}

	var state LobbyState
	if err := json.Unmarshal(data, &state); err != nil {
		return Party{}, err
	}

	if state.Party.Status == StatusDisbanded {
		return Party{}, ErrNotFound
	}

	return state.Party, nil
}

// GetPartyForUpdate retrieves party lobby state for update.
func (r *ValkeyRepository) GetPartyForUpdate(ctx context.Context, id string) (Party, error) {
	return r.GetParty(ctx, id)
}

// ListParties lists active party lobbies from Valkey Master using Sorted Set index.
func (r *ValkeyRepository) ListParties(ctx context.Context, status string, limit, offset int) ([]PartySummary, int, error) {
	limit, offset = pagination.NormalizeWithDefaults(limit, offset, 50, 100)

	if r.client == nil {
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
			activeList = append(activeList, PartySummary{
				ID:                p.ID,
				Name:              p.Name,
				LeaderCharacterID: p.LeaderCharacterID,
				LeaderName:        leaderName,
				StageID:           p.StageID,
				Speed:             p.Speed,
				CurrentMembers:    memCount,
				MaxMembers:        p.MaxMembers,
				HasPassword:       p.PasswordHash != "",
				MinLevel:          p.MinLevel,
				MaxLevel:          p.MaxLevel,
				MinHP:             p.MinHP,
				Status:            p.Status,
				CreatedAt:         p.CreatedAt,
			})
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

	// In Valkey: Fetch IDs from sorted set
	ids, err := r.client.Do(ctx, r.client.B().Zrevrange().Key(r.lobbiesIndexKey).Start(0).Stop(-1).Build()).AsStrSlice()
	if err != nil {
		return nil, 0, err
	}

	var summaries []PartySummary
	for _, id := range ids {
		pData, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(id)).Build()).AsBytes()
		if err != nil {
			if valkey.IsValkeyNil(err) {
				// Clean stale index
				_ = r.client.Do(ctx, r.client.B().Zrem().Key(r.lobbiesIndexKey).Member(id).Build()).Error()
			}
			continue
		}

		var state LobbyState
		if err := json.Unmarshal(pData, &state); err != nil {
			continue
		}

		if status != "" && state.Party.Status != status {
			continue
		}

		var leaderName string
		for _, m := range state.Members {
			if m.IsLeader {
				leaderName = m.CharacterName
				break
			}
		}

		summaries = append(summaries, PartySummary{
			ID:                state.Party.ID,
			Name:              state.Party.Name,
			LeaderCharacterID: state.Party.LeaderCharacterID,
			LeaderName:        leaderName,
			StageID:           state.Party.StageID,
			Speed:             state.Party.Speed,
			CurrentMembers:    len(state.Members),
			MaxMembers:        state.Party.MaxMembers,
			HasPassword:       state.Party.PasswordHash != "",
			MinLevel:          state.Party.MinLevel,
			MaxLevel:          state.Party.MaxLevel,
			MinHP:             state.Party.MinHP,
			Status:            state.Party.Status,
			CreatedAt:         state.Party.CreatedAt,
		})
	}

	total := len(summaries)
	if offset >= total {
		return []PartySummary{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return summaries[offset:end], total, nil
}

// UpdateParty updates party metadata in Valkey Master.
func (r *ValkeyRepository) UpdateParty(ctx context.Context, p Party) error {
	if r.client == nil {
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

	partyJSON, err := json.Marshal(p)
	if err != nil {
		return err
	}

	res := r.updatePartyScript.Exec(ctx, r.client, []string{
		r.lobbyKey(p.ID),
		r.lobbiesIndexKey,
	}, []string{
		string(partyJSON),
		strconv.FormatInt(int64(r.lobbyTTL.Seconds()), 10),
		p.Status,
		p.ID,
	})
	return mapLuaError(res.Error())
}

// DeleteParty disbands/deletes a party lobby and purges its members from Valkey Master.
func (r *ValkeyRepository) DeleteParty(ctx context.Context, id string) error {
	if r.client == nil {
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

	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(id)).Build()).AsBytes()
	if err == nil {
		var state LobbyState
		if json.Unmarshal(data, &state) == nil {
			for _, m := range state.Members {
				_ = r.client.Do(ctx, r.client.B().Del().Key(r.characterKey(m.CharacterID)).Build()).Error()
				_ = r.client.Do(ctx, r.client.B().Del().Key(r.readyKey(id, m.CharacterID)).Build()).Error()
			}
			if state.Party.LeaderCharacterID != "" {
				_ = r.client.Do(ctx, r.client.B().Del().Key(r.characterKey(state.Party.LeaderCharacterID)).Build()).Error()
			}
		}
	}

	_ = r.client.Do(ctx, r.client.B().Del().Key(r.lobbyKey(id)).Build()).Error()
	_ = r.client.Do(ctx, r.client.B().Zrem().Key(r.lobbiesIndexKey).Member(id).Build()).Error()
	return nil
}

// AddMember adds a member to a party lobby in Valkey Master.
func (r *ValkeyRepository) AddMember(ctx context.Context, m Member) error {
	if r.client == nil {
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

	memberJSON, err := json.Marshal(m)
	if err != nil {
		return err
	}
	readyArg := "0"
	if m.ReadyState {
		readyArg = "1"
	}

	res := r.addMemberScript.Exec(ctx, r.client, []string{
		r.lobbyKey(m.PartyID),
		r.characterKey(m.CharacterID),
		r.readyKey(m.PartyID, m.CharacterID),
	}, []string{
		string(memberJSON),
		m.CharacterID,
		m.PartyID,
		strconv.FormatInt(int64(r.lobbyTTL.Seconds()), 10),
		strconv.FormatInt(int64(r.readyTTL.Seconds()), 10),
		readyArg,
	})
	return mapLuaError(res.Error())
}

// GetMembers retrieves all members of a party lobby, resolving ready states against 60-second countdowns.
func (r *ValkeyRepository) GetMembers(ctx context.Context, partyID string) ([]Member, error) {
	if r.client == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		now := time.Now().UTC()
		membersMap, ok := r.memMembers[partyID]
		if !ok {
			return nil, nil
		}
		var list []Member
		for _, m := range membersMap {
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

	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(partyID)).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	var state LobbyState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	for i := range state.Members {
		// Check ready countdown key in Valkey
		exists, err := r.client.Do(ctx, r.client.B().Exists().Key(r.readyKey(partyID, state.Members[i].CharacterID)).Build()).AsInt64()
		if err == nil && exists > 0 {
			state.Members[i].ReadyState = true
		} else {
			state.Members[i].ReadyState = false
		}
	}

	sort.Slice(state.Members, func(i, j int) bool {
		if state.Members[i].IsLeader != state.Members[j].IsLeader {
			return state.Members[i].IsLeader
		}
		return state.Members[i].JoinedAt.Before(state.Members[j].JoinedAt)
	})

	return state.Members, nil
}

// GetMember retrieves a single member and evaluates their 60-second ready check countdown.
func (r *ValkeyRepository) GetMember(ctx context.Context, partyID, characterID string) (Member, error) {
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

// GetActivePartyByCharacter returns the active party and member record for a character.
func (r *ValkeyRepository) GetActivePartyByCharacter(ctx context.Context, characterID string) (Party, Member, error) {
	if r.client == nil {
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
		if readyExp, ok := r.memReadyExp[partyID][characterID]; ok && time.Now().UTC().Before(readyExp) {
			m.ReadyState = true
		} else {
			m.ReadyState = false
		}
		return p, m, nil
	}

	partyID, err := r.client.Do(ctx, r.client.B().Get().Key(r.characterKey(characterID)).Build()).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return Party{}, Member{}, ErrNotFound
		}
		return Party{}, Member{}, err
	}

	p, err := r.GetParty(ctx, partyID)
	if err != nil {
		// Stale character index, clean up
		_ = r.client.Do(ctx, r.client.B().Del().Key(r.characterKey(characterID)).Build()).Error()
		return Party{}, Member{}, ErrNotFound
	}

	m, err := r.GetMember(ctx, partyID, characterID)
	if err != nil {
		_ = r.client.Do(ctx, r.client.B().Del().Key(r.characterKey(characterID)).Build()).Error()
		return Party{}, Member{}, ErrNotFound
	}

	return p, m, nil
}

// RemoveMember removes a member from a party lobby in Valkey Master.
func (r *ValkeyRepository) RemoveMember(ctx context.Context, partyID, characterID string) error {
	if r.client == nil {
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

	res := r.removeMemberScript.Exec(ctx, r.client, []string{
		r.lobbyKey(partyID),
		r.characterKey(characterID),
		r.readyKey(partyID, characterID),
	}, []string{
		characterID,
		strconv.FormatInt(int64(r.lobbyTTL.Seconds()), 10),
	})
	return mapLuaError(res.Error())
}

// UpdateMemberReady sets or unsets member ready status with a 60-second countdown in Valkey Master.
func (r *ValkeyRepository) UpdateMemberReady(ctx context.Context, partyID, characterID string, ready bool) error {
	if r.client == nil {
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

	readyArg := "0"
	if ready {
		readyArg = "1"
	}

	res := r.updateMemberReadyScript.Exec(ctx, r.client, []string{
		r.lobbyKey(partyID),
		r.readyKey(partyID, characterID),
	}, []string{
		characterID,
		readyArg,
		strconv.FormatInt(int64(r.lobbyTTL.Seconds()), 10),
		strconv.FormatInt(int64(r.readyTTL.Seconds()), 10),
	})
	return mapLuaError(res.Error())
}

// CountMembers returns the active member count for a party lobby.
func (r *ValkeyRepository) CountMembers(ctx context.Context, partyID string) (int, error) {
	members, err := r.GetMembers(ctx, partyID)
	if err != nil {
		return 0, err
	}
	return len(members), nil
}

// SaveAdventureLog persists durable adventure logs to MariaDB (or fallback).
func (r *ValkeyRepository) SaveAdventureLog(ctx context.Context, log PartyAdventureLog) error {
	if r.logRepo != nil {
		return r.logRepo.SaveAdventureLog(ctx, log)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.memLogs = append(r.memLogs, log)
	return nil
}

// GetLogs returns in-memory logged adventure runs (useful for tests).
func (r *ValkeyRepository) GetLogs() []PartyAdventureLog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copied := make([]PartyAdventureLog, len(r.memLogs))
	copy(copied, r.memLogs)
	return copied
}

// Ensure interface compliance
var _ Repository = (*ValkeyRepository)(nil)
