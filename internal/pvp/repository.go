package pvp

import (
	"context"
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	DefaultRoomKeyPrefix       = "party2:pvp:room:"
	DefaultCharacterKeyPrefix  = "party2:pvp:character:"
	DefaultRoomsActiveIndexKey = "party2:pvp:rooms:active"
	DefaultRoomsIndexKey       = DefaultRoomsActiveIndexKey // Deprecated: alias for backward compatibility
)

type roomTTLContextKey struct{}

func withRemainingTTL(ctx context.Context, ttl time.Duration) context.Context {
	return context.WithValue(ctx, roomTTLContextKey{}, ttl)
}

func ttlFromContext(ctx context.Context) time.Duration {
	if ttl, ok := ctx.Value(roomTTLContextKey{}).(time.Duration); ok && ttl > 0 {
		if ttl < time.Millisecond {
			return time.Millisecond
		}
		return ttl
	}
	return DefaultLobbyTTL
}

// RoomRepository manages ephemeral colosseum room state.
type RoomRepository interface {
	SaveRoom(ctx context.Context, room ColosseumRoom, members []RoomMember) error
	GetRoom(ctx context.Context, id string) (RoomDetail, error)
	DeleteRoom(ctx context.Context, id string) error
	ListRooms(ctx context.Context) ([]RoomSummary, error)
	GetCharacterRoom(ctx context.Context, characterID string) (string, error)
	SetCharacterRoom(ctx context.Context, characterID string, roomID string) error
	DeleteCharacterRoom(ctx context.Context, characterID string, roomID string) error
	RoomLockRepository
}

// MemoryRoomRepository is an in-memory thread-safe implementation of RoomRepository for tests.
type MemoryRoomRepository struct {
	mu        sync.RWMutex
	rooms     map[string]RoomDetail
	charRooms map[string]string
	roomLocks map[string]*sync.Mutex
	lockMu    sync.Mutex
}

// NewMemoryRoomRepository creates a new in-memory room repository.
func NewMemoryRoomRepository() *MemoryRoomRepository {
	return &MemoryRoomRepository{
		rooms:     make(map[string]RoomDetail),
		charRooms: make(map[string]string),
		roomLocks: make(map[string]*sync.Mutex),
	}
}

func cloneIntMap(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (m *MemoryRoomRepository) SaveRoom(_ context.Context, room ColosseumRoom, members []RoomMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	membersCopy := make([]RoomMember, len(members))
	copy(membersCopy, members)

	roomCopy := room
	roomCopy.TeamScores = cloneIntMap(room.TeamScores)
	roomCopy.PendingPvPKills = cloneIntMap(room.PendingPvPKills)

	m.rooms[room.ID] = RoomDetail{
		Room:    roomCopy,
		Members: membersCopy,
	}
	return nil
}

func (m *MemoryRoomRepository) GetRoom(_ context.Context, id string) (RoomDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	detail, ok := m.rooms[id]
	if !ok {
		return RoomDetail{}, ErrRoomNotFound
	}
	membersCopy := make([]RoomMember, len(detail.Members))
	copy(membersCopy, detail.Members)

	roomCopy := detail.Room
	roomCopy.TeamScores = cloneIntMap(detail.Room.TeamScores)
	roomCopy.PendingPvPKills = cloneIntMap(detail.Room.PendingPvPKills)

	return RoomDetail{Room: roomCopy, Members: membersCopy}, nil
}

func (m *MemoryRoomRepository) DeleteRoom(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if detail, ok := m.rooms[id]; ok {
		for _, mem := range detail.Members {
			if m.charRooms[mem.CharacterID] == id {
				delete(m.charRooms, mem.CharacterID)
			}
		}
	}
	delete(m.rooms, id)
	return nil
}

func (m *MemoryRoomRepository) ListRooms(_ context.Context) ([]RoomSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var summaries []RoomSummary
	for _, d := range m.rooms {
		if d.Room.Status != StatusRecruiting {
			continue
		}
		summaries = append(summaries, newRoomSummary(d))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].CreatedAt.After(summaries[j].CreatedAt)
	})
	return summaries, nil
}

func (m *MemoryRoomRepository) GetCharacterRoom(_ context.Context, characterID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	roomID, ok := m.charRooms[characterID]
	if !ok || roomID == "" {
		return "", ErrRoomNotFound
	}
	return roomID, nil
}

func (m *MemoryRoomRepository) SetCharacterRoom(_ context.Context, characterID string, roomID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.charRooms[characterID] = roomID
	return nil
}

func (m *MemoryRoomRepository) DeleteCharacterRoom(_ context.Context, characterID string, roomID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.charRooms[characterID] == roomID {
		delete(m.charRooms, characterID)
	}
	return nil
}

// WithRoomLock executes fn inside an exclusive lock for roomID.
// It supports reentrant calls within the same context.
func (m *MemoryRoomRepository) WithRoomLock(ctx context.Context, roomID string, fn func(ctx context.Context) error) error {
	if roomID == "" {
		return fn(ctx)
	}

	if isRoomLocked(ctx, roomID) {
		return fn(ctx)
	}

	m.lockMu.Lock()
	l, ok := m.roomLocks[roomID]
	if !ok {
		l = &sync.Mutex{}
		m.roomLocks[roomID] = l
	}
	m.lockMu.Unlock()

	l.Lock()
	defer l.Unlock()

	lockedCtx := withRoomLockedContext(ctx, roomID)
	return fn(lockedCtx)
}

// ValkeyRoomRepository persists colosseum rooms in Valkey with automatic 30-minute expiration.
type ValkeyRoomRepository struct {
	client valkey.Client
}

// NewValkeyRoomRepository creates a new Valkey-backed room repository.
func NewValkeyRoomRepository(client valkey.Client) (*ValkeyRoomRepository, error) {
	if client == nil {
		return nil, ErrInvalidDependencies
	}
	return &ValkeyRoomRepository{client: client}, nil
}

func (v *ValkeyRoomRepository) SaveRoom(ctx context.Context, room ColosseumRoom, members []RoomMember) error {
	now := time.Now().UTC()
	if room.CreatedAt.IsZero() {
		room.CreatedAt = now
	}
	if room.UpdatedAt.IsZero() {
		room.UpdatedAt = now
	}

	detail := RoomDetail{Room: room, Members: members}
	data, err := json.Marshal(struct {
		RoomDetail
		PasswordHash     string         `json:"password_hash,omitempty"`
		FinancialVersion int            `json:"financial_version,omitempty"`
		LastActivityAt   time.Time      `json:"last_activity_at,omitempty"`
		PendingPvPKills  map[string]int `json:"pending_pvp_kills,omitempty"`
	}{detail, room.PasswordHash, room.FinancialVersion, room.LastActivityAt, room.PendingPvPKills})
	if err != nil {
		return err
	}

	key := DefaultRoomKeyPrefix + room.ID
	cmd := v.client.B().Set().Key(key).Value(string(data)).Px(ttlFromContext(ctx)).Build()
	if err := v.client.Do(ctx, cmd).Error(); err != nil {
		return err
	}

	scoreTime := room.UpdatedAt
	if !room.LastActivityAt.IsZero() {
		scoreTime = room.LastActivityAt
	}
	indexCmd := v.client.B().Zadd().Key(DefaultRoomsActiveIndexKey).ScoreMember().
		ScoreMember(float64(scoreTime.Unix()), room.ID).Build()
	return v.client.Do(ctx, indexCmd).Error()
}

func (v *ValkeyRoomRepository) GetRoom(ctx context.Context, id string) (RoomDetail, error) {
	key := DefaultRoomKeyPrefix + id
	cmd := v.client.B().Get().Key(key).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return RoomDetail{}, ErrRoomNotFound
		}
		return RoomDetail{}, err
	}

	str, err := res.ToString()
	if err != nil {
		return RoomDetail{}, err
	}

	var detail struct {
		RoomDetail
		PasswordHash     string         `json:"password_hash,omitempty"`
		FinancialVersion int            `json:"financial_version,omitempty"`
		LastActivityAt   time.Time      `json:"last_activity_at,omitempty"`
		PendingPvPKills  map[string]int `json:"pending_pvp_kills,omitempty"`
	}
	if err := json.Unmarshal([]byte(str), &detail); err != nil {
		return RoomDetail{}, err
	}
	detail.Room.PasswordHash = detail.PasswordHash
	detail.Room.FinancialVersion = detail.FinancialVersion
	detail.Room.LastActivityAt = detail.LastActivityAt
	detail.Room.PendingPvPKills = detail.PendingPvPKills
	return detail.RoomDetail, nil
}

func (v *ValkeyRoomRepository) DeleteRoom(ctx context.Context, id string) error {
	key := DefaultRoomKeyPrefix + id
	delCmd := v.client.B().Del().Key(key).Build()
	if err := v.client.Do(ctx, delCmd).Error(); err != nil {
		return err
	}

	zremCmd := v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(id).Build()
	return v.client.Do(ctx, zremCmd).Error()
}

func (v *ValkeyRoomRepository) ListRooms(ctx context.Context) ([]RoomSummary, error) {
	now := time.Now().UTC()
	cutoff := float64(now.Add(-DefaultLobbyTTL).Unix())
	remCmd := v.client.B().Zremrangebyscore().Key(DefaultRoomsActiveIndexKey).Min("-inf").Max("(" + strconv.FormatFloat(cutoff, 'f', 0, 64)).Build()
	_ = v.client.Do(ctx, remCmd)

	cmd := v.client.B().Zrevrange().Key(DefaultRoomsActiveIndexKey).Start(0).Stop(-1).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		return nil, err
	}

	roomIDs, err := res.AsStrSlice()
	if err != nil {
		return nil, err
	}

	var summaries []RoomSummary
	for _, id := range roomIDs {
		detail, err := v.GetRoom(ctx, id)
		if err != nil {
			_ = v.client.Do(ctx, v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(id).Build())
			continue
		}
		if detail.Room.Status != StatusRecruiting {
			continue
		}
		if !detail.Room.LastActivityAt.IsZero() && now.After(detail.Room.LastActivityAt.Add(DefaultLobbyTTL)) {
			continue
		}
		summaries = append(summaries, newRoomSummary(detail))
	}
	return summaries, nil
}

func (v *ValkeyRoomRepository) GetCharacterRoom(ctx context.Context, characterID string) (string, error) {
	key := DefaultCharacterKeyPrefix + characterID
	cmd := v.client.B().Get().Key(key).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return "", ErrRoomNotFound
		}
		return "", err
	}
	return res.ToString()
}

func (v *ValkeyRoomRepository) SetCharacterRoom(ctx context.Context, characterID string, roomID string) error {
	key := DefaultCharacterKeyPrefix + characterID
	cmd := v.client.B().Set().Key(key).Value(roomID).Px(ttlFromContext(ctx)).Build()
	return v.client.Do(ctx, cmd).Error()
}

//go:embed lua/delete_character_room.lua
var deleteCharacterRoomLua string

var deleteCharacterRoomScript = valkey.NewLuaScript(deleteCharacterRoomLua)

func (v *ValkeyRoomRepository) DeleteCharacterRoom(ctx context.Context, characterID string, roomID string) error {
	key := DefaultCharacterKeyPrefix + characterID
	return deleteCharacterRoomScript.Exec(ctx, v.client, []string{key}, []string{roomID}).Error()
}
