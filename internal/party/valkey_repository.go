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
	// DefaultCharacterKeyPrefix is the Valkey key prefix for O(1) character-to-party mapping.
	DefaultCharacterKeyPrefix = "party2:party:character:"
	// DefaultLobbiesIndexKey is the Valkey Sorted Set key tracking active recruiting lobbies.
	DefaultLobbiesIndexKey = "party2:party:lobbies"
	// DefaultLobbyTTL is the safety expiry if the idle sweep cannot run.
	DefaultLobbyTTL = 2 * time.Hour
)

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
	if strings.Contains(errStr, "ERR_PARTY_NOT_RECRUITING") {
		return ErrPartyNotRecruiting
	}
	if strings.Contains(errStr, "ERR_ALREADY_IN_PARTY") {
		return ErrAlreadyInParty
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

// WithLobbyClock overrides the activity clock shared by lobby writes and tests.
func WithLobbyClock(clock func() time.Time) ValkeyRepositoryOption {
	return func(r *repositoryConfig) {
		if clock != nil {
			r.clock = clock
		}
	}
}

type repositoryConfig struct {
	clock           func() time.Time
	logRepo         AdventureLogRepository
	lobbyPrefix     string
	charPrefix      string
	lobbiesIndexKey string
	lobbyTTL        time.Duration
}

func newRepositoryConfig(opts []ValkeyRepositoryOption) repositoryConfig {
	cfg := repositoryConfig{
		clock:           time.Now,
		lobbyPrefix:     DefaultLobbyKeyPrefix,
		charPrefix:      DefaultCharacterKeyPrefix,
		lobbiesIndexKey: DefaultLobbiesIndexKey,
		lobbyTTL:        DefaultLobbyTTL,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// ValkeyRepository stores transient party lobbies in Valkey.
type ValkeyRepository struct {
	client valkey.Client
	repositoryConfig

	updatePartyScript *valkey.Lua
}

// NewValkeyRepository constructs a ValkeyRepository. It panics if client is nil.
func NewValkeyRepository(client valkey.Client, opts ...ValkeyRepositoryOption) *ValkeyRepository {
	if client == nil {
		panic("party: Valkey client is required")
	}
	return &ValkeyRepository{
		client:            client,
		repositoryConfig:  newRepositoryConfig(opts),
		updatePartyScript: valkey.NewLuaScript(updatePartyLua),
	}
}

func (r *ValkeyRepository) lobbyKey(id string) string {
	return r.lobbyPrefix + id
}

func (r *ValkeyRepository) characterKey(charID string) string {
	return r.charPrefix + charID
}

// SaveParty stores a newly created party lobby in Valkey Master.
func (r *ValkeyRepository) SaveParty(ctx context.Context, p Party, members ...Member) error {
	state := storedLobbyState{Party: newStoredParty(p), Members: make([]storedMember, 0, len(members))}
	for _, member := range members {
		stored, err := newStoredMember(member)
		if err != nil {
			return err
		}
		state.Members = append(state.Members, stored)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return mapLuaError(r.mutateLobby(ctx, "save", p.ID, string(data), r.clock()).Error())
}

// GetParty retrieves party metadata from Valkey Master.
func (r *ValkeyRepository) GetParty(ctx context.Context, id string) (Party, error) {
	state, err := r.readLobby(ctx, id)
	if err != nil {
		return Party{}, err
	}
	return state.toParty(), nil
}

func (r *ValkeyRepository) readLobby(ctx context.Context, id string) (storedLobbyState, error) {
	data, err := r.client.Do(ctx, r.client.B().Get().Key(r.lobbyKey(id)).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return storedLobbyState{}, ErrNotFound
		}
		return storedLobbyState{}, err
	}

	var state storedLobbyState
	err = json.Unmarshal(data, &state)
	return state, err
}

// GetPartyForUpdate retrieves party lobby state for update.
func (r *ValkeyRepository) GetPartyForUpdate(ctx context.Context, id string) (Party, error) {
	p, err := r.GetParty(ctx, id)
	if err == nil && p.sleepUntil != 0 {
		return Party{}, ErrPartyNotRecruiting
	}
	return p, err
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
			continue
		}

		var state storedLobbyState
		if err := json.Unmarshal(pData, &state); err != nil {
			continue
		}

		p := state.Party.toParty()
		if state.SleepUntil != 0 || (status != "" && p.Status != status) {
			continue
		}

		members := make([]Member, len(state.Members))
		for i, m := range state.Members {
			members[i] = m.Member
		}
		summaries = append(summaries, newSummary(p, members))
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].CreatedAt.Equal(summaries[j].CreatedAt) {
			return summaries[i].ID > summaries[j].ID
		}
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})
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
	data, err := json.Marshal(newStoredParty(p))
	if err != nil {
		return err
	}
	return mapLuaError(r.mutateLobby(ctx, "update", p.ID, string(data), r.clock()).Error())
}

// DeleteParty disbands/deletes a party lobby and purges its members from Valkey Master.
func (r *ValkeyRepository) DeleteParty(ctx context.Context, id string) error {
	return mapLuaError(r.mutateLobby(ctx, "delete", id, "", r.clock()).Error())
}

// AddMember adds a member to a party lobby in Valkey Master.
func (r *ValkeyRepository) AddMember(ctx context.Context, m Member) error {
	member, err := newStoredMember(m)
	if err != nil {
		return err
	}
	data, err := json.Marshal(member)
	if err != nil {
		return err
	}
	return mapLuaError(r.mutateLobby(ctx, "add", m.PartyID, string(data), r.clock()).Error())
}

func (r *ValkeyRepository) mutateLobby(ctx context.Context, op, id, value string, now time.Time) valkey.ValkeyResult {
	return r.updatePartyScript.Exec(ctx, r.client, []string{r.lobbyKey(id), r.lobbiesIndexKey}, []string{
		op, id, r.charPrefix, r.lobbyPrefix, strconv.FormatInt(max(1, r.lobbyTTL.Milliseconds()), 10),
		strconv.FormatInt(now.UnixMilli(), 10), strconv.FormatInt(now.Add(lobbyIdleTimeout).UnixMilli(), 10), value,
	})
}

// GetMembers retrieves all members of a party lobby, leader first.
func (r *ValkeyRepository) GetMembers(ctx context.Context, partyID string) ([]Member, error) {
	state, err := r.readLobby(ctx, partyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	members, err := decodeStoredMembers(state.Members)
	if err != nil {
		return nil, err
	}

	sort.Slice(members, func(i, j int) bool {
		if members[i].IsLeader != members[j].IsLeader {
			return members[i].IsLeader
		}
		return members[i].JoinedAt.Before(members[j].JoinedAt)
	})

	return members, nil
}

// GetMember retrieves a single lobby member.
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

	state, err := r.readLobby(ctx, partyID)
	if err != nil {
		return Party{}, Member{}, err
	}
	members, err := decodeStoredMembers(state.Members)
	if err != nil {
		return Party{}, Member{}, err
	}
	for _, member := range members {
		if member.CharacterID == characterID {
			return state.toParty(), member, nil
		}
	}
	return Party{}, Member{}, ErrCharacterNotInParty
}

// RemoveMember removes a member from a party lobby in Valkey Master; a
// removed leader is replaced by the earliest remaining member.
func (r *ValkeyRepository) RemoveMember(ctx context.Context, partyID, characterID string) error {
	return mapLuaError(r.mutateLobby(ctx, "remove", partyID, characterID, r.clock()).Error())
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
