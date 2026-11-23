package party

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
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
	// DefaultLobbyTTL is the automatic expiration time for abandoned or idle party lobbies (30-minute auto-disband per quest.cgi).
	DefaultLobbyTTL = 30 * time.Minute
	// DefaultReadyTTL is the 60-second ready check countdown expiration duration.
	DefaultReadyTTL = 60 * time.Second
)

//go:embed lua/add_member.lua
var addMemberLua string

//go:embed lua/remove_member.lua
var removeMemberLua string

//go:embed lua/update_member_ready.lua
var updateMemberReadyLua string

//go:embed lua/update_party.lua
var updatePartyLua string

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
type ValkeyRepositoryOption func(*repositoryConfig)

// WithDurableLogRepository configures a delegate repository for durable adventure logs (e.g. MariaDB).
func WithDurableLogRepository(logRepo AdventureLogRepository) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		r.logRepo = logRepo
	}
}

// WithLobbyKeyPrefix overrides the default lobby key prefix.
func WithLobbyKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if prefix != "" {
			r.lobbyPrefix = prefix
		}
	}
}

// WithReadyKeyPrefix overrides the default ready countdown key prefix.
func WithReadyKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if prefix != "" {
			r.readyPrefix = prefix
		}
	}
}

// WithCharacterKeyPrefix overrides the default character-party index key prefix.
func WithCharacterKeyPrefix(prefix string) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if prefix != "" {
			r.charPrefix = prefix
		}
	}
}

// WithLobbiesIndexKey overrides the default lobbies index key.
func WithLobbiesIndexKey(key string) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if key != "" {
			r.lobbiesIndexKey = key
		}
	}
}

// WithLobbyTTL overrides the default lobby TTL.
func WithLobbyTTL(d time.Duration) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if d > 0 {
			r.lobbyTTL = d
		}
	}
}

// WithReadyTTL overrides the default ready check countdown TTL.
func WithReadyTTL(d time.Duration) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if d > 0 {
			r.readyTTL = d
		}
	}
}

type repositoryConfig struct {
	logRepo         AdventureLogRepository
	lobbyPrefix     string
	readyPrefix     string
	charPrefix      string
	lobbiesIndexKey string
	lobbyTTL        time.Duration
	readyTTL        time.Duration
}

func newRepositoryConfig(opts []ValkeyRepositoryOption) repositoryConfig {
	cfg := repositoryConfig{
		lobbyPrefix:     DefaultLobbyKeyPrefix,
		readyPrefix:     DefaultReadyKeyPrefix,
		charPrefix:      DefaultCharacterKeyPrefix,
		lobbiesIndexKey: DefaultLobbiesIndexKey,
		lobbyTTL:        DefaultLobbyTTL,
		readyTTL:        DefaultReadyTTL,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// ValkeyRepository stores transient party lobbies and ready checks in Valkey.
type ValkeyRepository struct {
	client valkey.Client
	repositoryConfig

	// Lua scripts for atomic lobby state mutations
	addMemberScript         *valkey.Lua
	removeMemberScript      *valkey.Lua
	updateMemberReadyScript *valkey.Lua
	updatePartyScript       *valkey.Lua
}

// NewValkeyRepository constructs a ValkeyRepository. It panics if client is nil.
func NewValkeyRepository(client valkey.Client, opts ...ValkeyRepositoryOption) *ValkeyRepository {
	if client == nil {
		panic("party: Valkey client is required")
	}
	return &ValkeyRepository{
		client:                  client,
		repositoryConfig:        newRepositoryConfig(opts),
		addMemberScript:         valkey.NewLuaScript(addMemberLua),
		removeMemberScript:      valkey.NewLuaScript(removeMemberLua),
		updateMemberReadyScript: valkey.NewLuaScript(updateMemberReadyLua),
		updatePartyScript:       valkey.NewLuaScript(updatePartyLua),
	}
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
	state := storedLobbyState{
		Party: storedParty{
			Party:        p,
			PasswordHash: p.PasswordHash,
		},
		Members: []storedMember{},
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
	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(id)).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			_ = r.client.Do(ctx, r.client.B().Zrem().Key(r.lobbiesIndexKey).Member(id).Build()).Error()
			return Party{}, ErrNotFound
		}
		return Party{}, err
	}

	var state storedLobbyState
	if err := json.Unmarshal(data, &state); err != nil {
		return Party{}, err
	}

	p := state.Party.toParty()
	if p.Status == StatusDisbanded {
		return Party{}, ErrNotFound
	}

	return p, nil
}

// GetPartyForUpdate retrieves party lobby state for update.
func (r *ValkeyRepository) GetPartyForUpdate(ctx context.Context, id string) (Party, error) {
	return r.GetParty(ctx, id)
}

// ListParties lists active party lobbies from Valkey Master using Sorted Set index.
func (r *ValkeyRepository) ListParties(ctx context.Context, status string, limit, offset int) ([]PartySummary, int, error) {
	limit, offset = pagination.NormalizeWithDefaults(limit, offset, 50, 100)

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

		var state storedLobbyState
		if err := json.Unmarshal(pData, &state); err != nil {
			continue
		}

		p := state.Party.toParty()
		if status != "" && p.Status != status {
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
			ID:                p.ID,
			Name:              p.Name,
			LeaderCharacterID: p.LeaderCharacterID,
			LeaderName:        leaderName,
			StageID:           p.StageID,
			Speed:             p.Speed,
			CurrentMembers:    len(state.Members),
			MaxMembers:        p.MaxMembers,
			HasPassword:       p.PasswordHash != "",
			MinLevel:          p.MinLevel,
			MaxLevel:          p.MaxLevel,
			MinHP:             p.MinHP,
			NeedJoin:          p.NeedJoin,
			Status:            p.Status,
			CreatedAt:         p.CreatedAt,
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
	partyJSON, err := marshalStoredParty(p)
	if err != nil {
		return err
	}

	res := r.updatePartyScript.Exec(ctx, r.client,
		[]string{r.lobbyKey(p.ID), r.lobbiesIndexKey},
		[]string{string(partyJSON), strconv.FormatInt(int64(r.lobbyTTL.Seconds()), 10), p.Status, p.ID},
	)
	return mapLuaError(res.Error())
}

// DeleteParty disbands/deletes a party lobby and purges its members from Valkey Master.
func (r *ValkeyRepository) DeleteParty(ctx context.Context, id string) error {
	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(id)).Build()).AsBytes()
	if err != nil && !valkey.IsValkeyNil(err) {
		return err
	}
	var keys []string
	if err == nil {
		var state storedLobbyState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		for _, m := range state.Members {
			keys = append(keys, r.characterKey(m.CharacterID), r.readyKey(id, m.CharacterID))
		}
		if state.Party.LeaderCharacterID != "" {
			keys = append(keys, r.characterKey(state.Party.LeaderCharacterID))
		}
	}
	keys = append(keys, r.lobbyKey(id))
	for _, key := range keys {
		if err := r.client.Do(ctx, r.client.B().Del().Key(key).Build()).Error(); err != nil {
			return err
		}
	}
	return r.client.Do(ctx, r.client.B().Zrem().Key(r.lobbiesIndexKey).Member(id).Build()).Error()
}

// AddMember adds a member to a party lobby in Valkey Master.
func (r *ValkeyRepository) AddMember(ctx context.Context, m Member) error {
	memberJSON, err := marshalStoredMember(m)
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
	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(partyID)).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	var state storedLobbyState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	members, err := decodeStoredMembers(state.Members)
	if err != nil {
		return nil, err
	}
	for i := range members {
		// Check ready countdown key in Valkey
		exists, err := r.client.Do(ctx, r.client.B().Exists().Key(r.readyKey(partyID, members[i].CharacterID)).Build()).AsInt64()
		if err == nil && exists > 0 {
			members[i].ReadyState = true
		} else {
			members[i].ReadyState = false
		}
	}

	sort.Slice(members, func(i, j int) bool {
		if members[i].IsLeader != members[j].IsLeader {
			return members[i].IsLeader
		}
		return members[i].JoinedAt.Before(members[j].JoinedAt)
	})

	return members, nil
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
	partyID, err := r.client.Do(ctx, r.client.B().Get().Key(r.characterKey(characterID)).Build()).ToString()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return Party{}, Member{}, ErrNotFound
		}
		return Party{}, Member{}, err
	}

	p, err := r.GetParty(ctx, partyID)
	if err != nil {
		return Party{}, Member{}, err
	}

	m, err := r.GetMember(ctx, partyID, characterID)
	if err != nil {
		return Party{}, Member{}, err
	}

	return p, m, nil
}

// RemoveMember removes a member from a party lobby in Valkey Master.
func (r *ValkeyRepository) RemoveMember(ctx context.Context, partyID, characterID string) error {
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

// SaveAdventureLog persists durable adventure logs through the configured store.
func (r *repositoryConfig) SaveAdventureLog(ctx context.Context, log PartyAdventureLog) error {
	if r.logRepo == nil {
		return errors.New("adventure log repository is required")
	}
	return r.logRepo.SaveAdventureLog(ctx, log)
}

// Ensure interface compliance
var _ Repository = (*ValkeyRepository)(nil)
