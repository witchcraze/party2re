package gvg

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/valkey-io/valkey-go"
)

const (
	DefaultRoomKeyPrefix      = "party2:gvg:room:"
	DefaultCharacterKeyPrefix = "party2:gvg:character:"
	DefaultRoomsIndexKey      = "party2:gvg:rooms"
)

// MatchSettlement encapsulates updates to durable guild standings upon match completion.
type MatchSettlement struct {
	WinnerGuildID string         // Empty if match ended in a draw
	WinnerPrizeGP int            // Prize pool GP to award to winning guild
	GuildIDs      []string       // All participating guilds in the match
	IsDraw        bool           // True if match ended in a draw
	ParticipantGP map[string]int // Additional GP awarded per guild (4 GP per participant)
}

// StandingRepository defines durable persistence for guild battle standings and trophy progression.
type StandingRepository interface {
	GetOrCreateStanding(ctx context.Context, guildID string) (GvGStanding, error)
	GetLeaderboard(ctx context.Context, limit int) ([]GvGStanding, error)
	AddRoundWinGP(ctx context.Context, guildID string, gp int) error
	RecordMatchSettlement(ctx context.Context, settlement MatchSettlement) error
}

// RoomRepository manages ephemeral guild battle room state.
type RoomRepository interface {
	SaveRoom(ctx context.Context, room GvGRoom, members []GvGMember) error
	GetRoom(ctx context.Context, id string) (RoomDetail, error)
	DeleteRoom(ctx context.Context, id string) error
	ListRooms(ctx context.Context) ([]RoomSummary, error)
	GetCharacterRoom(ctx context.Context, characterID string) (string, error)
	SetCharacterRoom(ctx context.Context, characterID string, roomID string) error
	DeleteCharacterRoom(ctx context.Context, characterID string) error
}

// MemoryRoomRepository is an in-memory thread-safe implementation of RoomRepository for tests.
type MemoryRoomRepository struct {
	mu        sync.RWMutex
	rooms     map[string]RoomDetail
	charRooms map[string]string
}

// NewMemoryRoomRepository creates a new in-memory room repository.
func NewMemoryRoomRepository() *MemoryRoomRepository {
	return &MemoryRoomRepository{
		rooms:     make(map[string]RoomDetail),
		charRooms: make(map[string]string),
	}
}

func (m *MemoryRoomRepository) SaveRoom(_ context.Context, room GvGRoom, members []GvGMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	membersCopy := make([]GvGMember, len(members))
	copy(membersCopy, members)

	m.rooms[room.ID] = RoomDetail{
		Room:    room,
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
	membersCopy := make([]GvGMember, len(detail.Members))
	copy(membersCopy, detail.Members)
	return RoomDetail{Room: detail.Room, Members: membersCopy}, nil
}

func (m *MemoryRoomRepository) DeleteRoom(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if detail, ok := m.rooms[id]; ok {
		for _, mem := range detail.Members {
			delete(m.charRooms, mem.CharacterID)
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
		summaries = append(summaries, RoomSummary{
			ID:                d.Room.ID,
			Name:              d.Room.Name,
			LeaderCharacterID: d.Room.LeaderCharacterID,
			LeaderName:        d.Room.LeaderName,
			Speed:             d.Room.Speed,
			Stage:             d.Room.Stage,
			CurrentMembers:    len(d.Members),
			MaxMembers:        d.Room.MaxMembers,
			PrizePool:         d.Room.PrizePool,
			TargetWins:        d.Room.TargetWins,
			HasPassword:       d.Room.PasswordHash != "",
			Status:            d.Room.Status,
			Round:             d.Room.Round,
			CreatedAt:         d.Room.CreatedAt,
		})
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
	if !ok {
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

func (m *MemoryRoomRepository) DeleteCharacterRoom(_ context.Context, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.charRooms, characterID)
	return nil
}

// ValkeyRoomRepository persists GvG rooms in Valkey with automatic 30-minute expiration.
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

func (v *ValkeyRoomRepository) SaveRoom(ctx context.Context, room GvGRoom, members []GvGMember) error {
	detail := RoomDetail{Room: room, Members: members}
	data, err := json.Marshal(detail)
	if err != nil {
		return err
	}

	key := DefaultRoomKeyPrefix + room.ID
	cmd := v.client.B().Set().Key(key).Value(string(data)).Ex(DefaultLobbyTTL).Build()
	if err := v.client.Do(ctx, cmd).Error(); err != nil {
		return err
	}

	indexCmd := v.client.B().Zadd().Key(DefaultRoomsIndexKey).ScoreMember().
		ScoreMember(float64(room.CreatedAt.Unix()), room.ID).Build()
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

	var detail RoomDetail
	if err := json.Unmarshal([]byte(str), &detail); err != nil {
		return RoomDetail{}, err
	}
	return detail, nil
}

func (v *ValkeyRoomRepository) DeleteRoom(ctx context.Context, id string) error {
	key := DefaultRoomKeyPrefix + id
	delCmd := v.client.B().Del().Key(key).Build()
	_ = v.client.Do(ctx, delCmd)

	zremCmd := v.client.B().Zrem().Key(DefaultRoomsIndexKey).Member(id).Build()
	return v.client.Do(ctx, zremCmd).Error()
}

func (v *ValkeyRoomRepository) ListRooms(ctx context.Context) ([]RoomSummary, error) {
	cmd := v.client.B().Zrevrange().Key(DefaultRoomsIndexKey).Start(0).Stop(50).Build()
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
			_ = v.client.Do(ctx, v.client.B().Zrem().Key(DefaultRoomsIndexKey).Member(id).Build())
			continue
		}
		if detail.Room.Status != StatusRecruiting {
			continue
		}
		summaries = append(summaries, RoomSummary{
			ID:                detail.Room.ID,
			Name:              detail.Room.Name,
			LeaderCharacterID: detail.Room.LeaderCharacterID,
			LeaderName:        detail.Room.LeaderName,
			Speed:             detail.Room.Speed,
			Stage:             detail.Room.Stage,
			CurrentMembers:    len(detail.Members),
			MaxMembers:        detail.Room.MaxMembers,
			PrizePool:         detail.Room.PrizePool,
			TargetWins:        detail.Room.TargetWins,
			HasPassword:       detail.Room.PasswordHash != "",
			Status:            detail.Room.Status,
			Round:             detail.Room.Round,
			CreatedAt:         detail.Room.CreatedAt,
		})
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
	cmd := v.client.B().Set().Key(key).Value(roomID).Ex(DefaultLobbyTTL).Build()
	return v.client.Do(ctx, cmd).Error()
}

func (v *ValkeyRoomRepository) DeleteCharacterRoom(ctx context.Context, characterID string) error {
	key := DefaultCharacterKeyPrefix + characterID
	cmd := v.client.B().Del().Key(key).Build()
	return v.client.Do(ctx, cmd).Error()
}
