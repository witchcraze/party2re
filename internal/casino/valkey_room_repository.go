package casino

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	DefaultRoomKeyPrefix       = "party2:casino:room:"
	DefaultRoomsActiveIndexKey = "party2:casino:rooms:active"
	DefaultCharacterKeyPrefix  = "party2:casino:character:"
	DefaultLobbyTTL            = 1800 * time.Second // 30 minutes (party2/lib/casino.cgi:38)
)

var (
	ErrNilValkeyClient = errors.New("valkey client is required")
)

var _ RoomRepository = (*ValkeyRoomRepository)(nil)

type valkeyRoomDetailDTO struct {
	Room         Room         `json:"room"`
	PasswordHash string       `json:"password_hash,omitempty"`
	Members      []RoomMember `json:"members"`
}

// ValkeyRoomRepository persists casino multiplayer room and turn state in Valkey Master (Candidate C).
type ValkeyRoomRepository struct {
	client   valkey.Client
	charRepo CharacterRepository
}

// ValkeyRoomRepositoryOption configures a ValkeyRoomRepository.
type ValkeyRoomRepositoryOption func(*ValkeyRoomRepository)

// WithValkeyCharacterRepository sets an optional character repository for name resolution.
func WithValkeyCharacterRepository(charRepo CharacterRepository) ValkeyRoomRepositoryOption {
	return func(v *ValkeyRoomRepository) {
		v.charRepo = charRepo
	}
}

// NewValkeyRoomRepository creates a new Valkey-backed casino room repository.
func NewValkeyRoomRepository(client valkey.Client, opts ...ValkeyRoomRepositoryOption) (*ValkeyRoomRepository, error) {
	if client == nil {
		return nil, ErrNilValkeyClient
	}
	repo := &ValkeyRoomRepository{client: client}
	for _, opt := range opts {
		opt(repo)
	}
	return repo, nil
}

// CreateRoom registers a new room and its leader in Valkey with 1800s sliding TTL.
func (v *ValkeyRoomRepository) CreateRoom(ctx context.Context, room Room, leaderMember RoomMember) error {
	now := time.Now().UTC()
	if room.CreatedAt.IsZero() {
		room.CreatedAt = now
	}
	if room.UpdatedAt.IsZero() {
		room.UpdatedAt = now
	}
	if leaderMember.JoinedAt.IsZero() {
		leaderMember.JoinedAt = now
	}
	if leaderMember.UpdatedAt.IsZero() {
		leaderMember.UpdatedAt = now
	}

	dto := &valkeyRoomDetailDTO{
		Room:         room,
		PasswordHash: room.PasswordHash,
		Members:      []RoomMember{leaderMember},
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}

	roomKey := DefaultRoomKeyPrefix + room.ID
	setCmd := v.client.B().Set().Key(roomKey).Value(string(data)).Ex(DefaultLobbyTTL).Build()
	if err := v.client.Do(ctx, setCmd).Error(); err != nil {
		return err
	}

	zaddCmd := v.client.B().Zadd().Key(DefaultRoomsActiveIndexKey).ScoreMember().
		ScoreMember(float64(room.UpdatedAt.Unix()), room.ID).Build()
	if err := v.client.Do(ctx, zaddCmd).Error(); err != nil {
		return err
	}

	charKey := DefaultCharacterKeyPrefix + leaderMember.CharacterID
	charCmd := v.client.B().Set().Key(charKey).Value(room.ID).Ex(DefaultLobbyTTL).Build()
	return v.client.Do(ctx, charCmd).Error()
}

func (v *ValkeyRoomRepository) getRoomDetail(ctx context.Context, roomID string) (*valkeyRoomDetailDTO, error) {
	roomKey := DefaultRoomKeyPrefix + roomID
	cmd := v.client.B().Get().Key(roomKey).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, ErrRoomNotFound
		}
		return nil, err
	}

	str, err := res.ToString()
	if err != nil {
		return nil, err
	}

	var dto valkeyRoomDetailDTO
	if err := json.Unmarshal([]byte(str), &dto); err != nil {
		return nil, err
	}
	if dto.PasswordHash != "" {
		dto.Room.PasswordHash = dto.PasswordHash
	}
	return &dto, nil
}

func (v *ValkeyRoomRepository) saveRoomDetail(ctx context.Context, dto *valkeyRoomDetailDTO) error {
	dto.PasswordHash = dto.Room.PasswordHash
	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}

	roomKey := DefaultRoomKeyPrefix + dto.Room.ID
	setCmd := v.client.B().Set().Key(roomKey).Value(string(data)).Ex(DefaultLobbyTTL).Build()
	if err := v.client.Do(ctx, setCmd).Error(); err != nil {
		return err
	}

	if dto.Room.Status == RoomStatusDisbanded {
		zremCmd := v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(dto.Room.ID).Build()
		_ = v.client.Do(ctx, zremCmd)
		for _, m := range dto.Members {
			charKey := DefaultCharacterKeyPrefix + m.CharacterID
			_ = v.client.Do(ctx, v.client.B().Del().Key(charKey).Build())
		}
	} else {
		zaddCmd := v.client.B().Zadd().Key(DefaultRoomsActiveIndexKey).ScoreMember().
			ScoreMember(float64(dto.Room.UpdatedAt.Unix()), dto.Room.ID).Build()
		_ = v.client.Do(ctx, zaddCmd)
	}
	return nil
}

// GetRoom retrieves a room by ID.
func (v *ValkeyRoomRepository) GetRoom(ctx context.Context, roomID string) (*Room, error) {
	dto, err := v.getRoomDetail(ctx, roomID)
	if err != nil {
		return nil, err
	}
	return &dto.Room, nil
}

// GetRoomForUpdate retrieves a room by ID.
func (v *ValkeyRoomRepository) GetRoomForUpdate(ctx context.Context, roomID string) (*Room, error) {
	return v.GetRoom(ctx, roomID)
}

// UpdateRoom updates room metadata in Valkey and refreshes sliding TTL.
func (v *ValkeyRoomRepository) UpdateRoom(ctx context.Context, room Room) error {
	dto, err := v.getRoomDetail(ctx, room.ID)
	if err != nil {
		return err
	}
	dto.Room = room
	return v.saveRoomDetail(ctx, dto)
}

// DeleteRoom removes a room and associated member mappings from Valkey.
func (v *ValkeyRoomRepository) DeleteRoom(ctx context.Context, roomID string) error {
	dto, err := v.getRoomDetail(ctx, roomID)
	if err == nil && dto != nil {
		for _, m := range dto.Members {
			charKey := DefaultCharacterKeyPrefix + m.CharacterID
			_ = v.client.Do(ctx, v.client.B().Del().Key(charKey).Build())
		}
	}
	roomKey := DefaultRoomKeyPrefix + roomID
	_ = v.client.Do(ctx, v.client.B().Del().Key(roomKey).Build())
	zremCmd := v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(roomID).Build()
	return v.client.Do(ctx, zremCmd).Error()
}

// ListActiveRooms lists non-disbanded rooms from the active sorted set with lazy pruning.
func (v *ValkeyRoomRepository) ListActiveRooms(ctx context.Context) ([]RoomDetail, error) {
	now := time.Now().UTC()
	cutoff := float64(now.Add(-DefaultLobbyTTL).Unix())
	remCmd := v.client.B().Zremrangebyscore().Key(DefaultRoomsActiveIndexKey).Min("-inf").Max(strconv.FormatFloat(cutoff, 'f', 0, 64)).Build()
	_ = v.client.Do(ctx, remCmd)

	cmd := v.client.B().Zrevrange().Key(DefaultRoomsActiveIndexKey).Start(0).Stop(100).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		return nil, err
	}

	roomIDs, err := res.AsStrSlice()
	if err != nil {
		return nil, err
	}

	var list []RoomDetail
	for _, id := range roomIDs {
		dto, err := v.getRoomDetail(ctx, id)
		if err != nil {
			if errors.Is(err, ErrRoomNotFound) {
				_ = v.client.Do(ctx, v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(id).Build())
			}
			continue
		}
		if dto.Room.Status != RoomStatusDisbanded {
			v.populateMemberNames(ctx, dto.Members)
			list = append(list, RoomDetail{
				Room:    dto.Room,
				Members: dto.Members,
			})
		}
	}
	return list, nil
}

// GetRoomByName searches for an active room with the given name.
func (v *ValkeyRoomRepository) GetRoomByName(ctx context.Context, name string) (*Room, error) {
	rooms, err := v.ListActiveRooms(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rooms {
		if r.Room.Name == name && r.Room.Status != RoomStatusDisbanded {
			cpy := r.Room
			return &cpy, nil
		}
	}
	return nil, ErrRoomNotFound
}

// PurgeIdleRooms explicitly deletes rooms whose last activity is before cutoff.
func (v *ValkeyRoomRepository) PurgeIdleRooms(ctx context.Context, cutoff time.Time) (int, error) {
	cmd := v.client.B().Zrangebyscore().Key(DefaultRoomsActiveIndexKey).
		Min("-inf").Max(strconv.FormatFloat(float64(cutoff.Unix()), 'f', 0, 64)).Build()
	res := v.client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		return 0, err
	}
	expiredIDs, err := res.AsStrSlice()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range expiredIDs {
		if err := v.DeleteRoom(ctx, id); err == nil {
			count++
		}
	}
	return count, nil
}

// AddMember adds or updates a participant in the room.
func (v *ValkeyRoomRepository) AddMember(ctx context.Context, member RoomMember) error {
	dto, err := v.getRoomDetail(ctx, member.RoomID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if member.JoinedAt.IsZero() {
		member.JoinedAt = now
	}
	if member.UpdatedAt.IsZero() {
		member.UpdatedAt = now
	}

	found := false
	for i, m := range dto.Members {
		if m.CharacterID == member.CharacterID {
			dto.Members[i] = member
			found = true
			break
		}
	}
	if !found {
		dto.Members = append(dto.Members, member)
	}

	dto.Room.UpdatedAt = now
	if err := v.saveRoomDetail(ctx, dto); err != nil {
		return err
	}

	charKey := DefaultCharacterKeyPrefix + member.CharacterID
	charCmd := v.client.B().Set().Key(charKey).Value(member.RoomID).Ex(DefaultLobbyTTL).Build()
	return v.client.Do(ctx, charCmd).Error()
}

// GetMember retrieves a single participant from a room.
func (v *ValkeyRoomRepository) GetMember(ctx context.Context, roomID string, characterID string) (*RoomMember, error) {
	dto, err := v.getRoomDetail(ctx, roomID)
	if err != nil {
		return nil, ErrMemberNotFound
	}
	for _, m := range dto.Members {
		if m.CharacterID == characterID {
			cpy := m
			v.populateMemberName(ctx, &cpy)
			return &cpy, nil
		}
	}
	return nil, ErrMemberNotFound
}

// GetMemberForUpdate retrieves a participant for update.
func (v *ValkeyRoomRepository) GetMemberForUpdate(ctx context.Context, roomID string, characterID string) (*RoomMember, error) {
	return v.GetMember(ctx, roomID, characterID)
}

// ListMembers lists all participants in a room.
func (v *ValkeyRoomRepository) ListMembers(ctx context.Context, roomID string) ([]RoomMember, error) {
	dto, err := v.getRoomDetail(ctx, roomID)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return nil, nil
		}
		return nil, err
	}
	members := make([]RoomMember, len(dto.Members))
	copy(members, dto.Members)
	v.populateMemberNames(ctx, members)
	return members, nil
}

// ListMembersForUpdate lists all participants in a room for update.
func (v *ValkeyRoomRepository) ListMembersForUpdate(ctx context.Context, roomID string) ([]RoomMember, error) {
	return v.ListMembers(ctx, roomID)
}

// UpdateMember updates participant state in the room.
func (v *ValkeyRoomRepository) UpdateMember(ctx context.Context, member RoomMember) error {
	dto, err := v.getRoomDetail(ctx, member.RoomID)
	if err != nil {
		return err
	}
	found := false
	for i, m := range dto.Members {
		if m.CharacterID == member.CharacterID {
			dto.Members[i] = member
			found = true
			break
		}
	}
	if !found {
		return ErrMemberNotFound
	}
	dto.Room.UpdatedAt = time.Now().UTC()
	return v.saveRoomDetail(ctx, dto)
}

// RemoveMember removes a participant from the room.
func (v *ValkeyRoomRepository) RemoveMember(ctx context.Context, roomID string, characterID string) error {
	dto, err := v.getRoomDetail(ctx, roomID)
	if err != nil {
		return err
	}
	idx := -1
	for i, m := range dto.Members {
		if m.CharacterID == characterID {
			idx = i
			break
		}
	}
	if idx >= 0 {
		dto.Members = append(dto.Members[:idx], dto.Members[idx+1:]...)
		dto.Room.UpdatedAt = time.Now().UTC()
		if err := v.saveRoomDetail(ctx, dto); err != nil {
			return err
		}
	}
	charKey := DefaultCharacterKeyPrefix + characterID
	_ = v.client.Do(ctx, v.client.B().Del().Key(charKey).Build())
	return nil
}

// GetCharacterRoom retrieves the room ID a character is currently associated with.
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

func (v *ValkeyRoomRepository) populateMemberName(ctx context.Context, m *RoomMember) {
	if m.CharacterName != "" || v.charRepo == nil {
		return
	}
	ch, err := v.charRepo.FindByID(ctx, m.CharacterID)
	if err == nil {
		m.CharacterName = ch.Name
	}
}

func (v *ValkeyRoomRepository) populateMemberNames(ctx context.Context, members []RoomMember) {
	for i := range members {
		v.populateMemberName(ctx, &members[i])
	}
}
