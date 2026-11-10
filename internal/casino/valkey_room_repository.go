package casino

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
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
	return v.withAdmissionLock(ctx, room.ID, leaderMember.CharacterID, func(lockedCtx context.Context) error {
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

		expectedPrevRoomID, err := v.checkCharacterAdmissionAvailable(lockedCtx, room.ID, leaderMember.CharacterID)
		if err != nil {
			return err
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
		if err := v.client.Do(lockedCtx, setCmd).Error(); err != nil {
			return err
		}

		var syncErr error
		zaddCmd := v.client.B().Zadd().Key(DefaultRoomsActiveIndexKey).ScoreMember().
			ScoreMember(float64(room.UpdatedAt.Unix()), room.ID).Build()
		if err := v.client.Do(lockedCtx, zaddCmd).Error(); err != nil {
			syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
		}

		if err := v.claimAdmittedMemberMapping(lockedCtx, room.ID, leaderMember.CharacterID, expectedPrevRoomID); err != nil {
			if errors.Is(err, ErrAlreadyInRoom) {
				delErr := v.client.Do(lockedCtx, v.client.B().Del().Key(roomKey).Build()).Error()
				zremErr := v.client.Do(lockedCtx, v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(room.ID).Build()).Error()
				return errors.Join(ErrAlreadyInRoom, delErr, zremErr)
			}
			syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
		}
		return syncErr
	})
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

// saveRoomDetail persists the room JSON payload with sliding TTL and synchronizes the active rooms index.
//
// Writer-side partial failure boundary:
// The room payload is written via SET before updating the active rooms index (party2:casino:rooms:active).
// If the index update (ZADD/ZREM) or member key deletion fails, the error is propagated truthfully to the caller.
// Because these operations are not cross-key transactional in Valkey, the room payload may already have been updated.
// Callers must not assume payload rollback and clients must not perform blind command retries; partial state
// remains observable by room ID (GetRoom), while lobby discovery reflects the index state until a subsequent renewal succeeds.
func (v *ValkeyRoomRepository) saveRoomDetail(ctx context.Context, dto *valkeyRoomDetailDTO) error {
	return v.saveRoomDetailExcluding(ctx, dto, "")
}

func (v *ValkeyRoomRepository) saveRoomDetailExcluding(ctx context.Context, dto *valkeyRoomDetailDTO, skipCharacterID string) error {
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

	var syncErr error
	if dto.Room.Status == RoomStatusDisbanded {
		zremCmd := v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(dto.Room.ID).Build()
		if err := v.client.Do(ctx, zremCmd).Error(); err != nil {
			syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
		}
		for _, m := range dto.Members {
			if err := v.deleteMemberMapping(ctx, dto.Room.ID, m.CharacterID); err != nil {
				syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
			}
		}
		return syncErr
	}

	zaddCmd := v.client.B().Zadd().Key(DefaultRoomsActiveIndexKey).ScoreMember().
		ScoreMember(float64(dto.Room.UpdatedAt.Unix()), dto.Room.ID).Build()
	if err := v.client.Do(ctx, zaddCmd).Error(); err != nil {
		syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
	}
	if !dto.Room.UpdatedAt.Before(time.Now().UTC().Add(-DefaultLobbyTTL)) {
		for _, m := range dto.Members {
			if m.CharacterID == skipCharacterID {
				continue
			}
			if err := v.renewMemberMapping(ctx, dto.Room.ID, m.CharacterID); err != nil {
				syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
			}
		}
	}
	return syncErr
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
// Active index renewal failures are propagated. If the active index update fails,
// the room payload SET may already have applied in Valkey; callers must not assume rollback or retry blindly.
func (v *ValkeyRoomRepository) UpdateRoom(ctx context.Context, room Room) error {
	return v.WithRoomLock(ctx, room.ID, func(lockedCtx context.Context) error {
		dto, err := v.getRoomDetail(lockedCtx, room.ID)
		if err != nil {
			return err
		}
		dto.Room = room
		return v.saveRoomDetail(lockedCtx, dto)
	})
}

// DeleteRoom removes a room and associated member mappings from Valkey.
func (v *ValkeyRoomRepository) DeleteRoom(ctx context.Context, roomID string) error {
	return v.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		dto, err := v.getRoomDetail(lockedCtx, roomID)
		if err != nil && !errors.Is(err, ErrRoomNotFound) {
			return err
		}
		if dto != nil {
			for _, m := range dto.Members {
				if err := v.deleteMemberMapping(lockedCtx, roomID, m.CharacterID); err != nil {
					return err
				}
			}
		}
		if err := v.client.Do(lockedCtx, v.client.B().Del().Key(DefaultRoomKeyPrefix+roomID).Build()).Error(); err != nil {
			return err
		}
		return v.client.Do(lockedCtx, v.client.B().Zrem().Key(DefaultRoomsActiveIndexKey).Member(roomID).Build()).Error()
	})
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

// AddMember adds or updates a participant in the room.
func (v *ValkeyRoomRepository) AddMember(ctx context.Context, member RoomMember) error {
	return v.withAdmissionLock(ctx, member.RoomID, member.CharacterID, func(lockedCtx context.Context) error {
		dto, err := v.getRoomDetail(lockedCtx, member.RoomID)
		if err != nil {
			return err
		}
		expectedPrevRoomID, err := v.checkCharacterAdmissionAvailable(lockedCtx, member.RoomID, member.CharacterID)
		if err != nil {
			return err
		}
		prevMembers := slices.Clone(dto.Members)
		prevUpdatedAt := dto.Room.UpdatedAt
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
		syncErr := v.saveRoomDetailExcluding(lockedCtx, dto, member.CharacterID)
		if syncErr != nil && !errors.Is(syncErr, ErrSecondarySync) {
			return syncErr
		}

		if err := v.claimAdmittedMemberMapping(lockedCtx, member.RoomID, member.CharacterID, expectedPrevRoomID); err != nil {
			if errors.Is(err, ErrAlreadyInRoom) {
				dto.Members = prevMembers
				dto.Room.UpdatedAt = prevUpdatedAt
				rollbackErr := unwrapSecondarySync(v.saveRoomDetail(lockedCtx, dto))
				return errors.Join(ErrAlreadyInRoom, rollbackErr)
			}
			syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
		}
		return syncErr
	})
}

// GetMember retrieves a single participant from a room.
func (v *ValkeyRoomRepository) GetMember(ctx context.Context, roomID string, characterID string) (*RoomMember, error) {
	dto, err := v.getRoomDetail(ctx, roomID)
	if err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}
	for _, m := range dto.Members {
		if m.CharacterID == characterID {
			cpy := m
			if err := v.populateMemberName(ctx, &cpy); err != nil {
				return nil, err
			}
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
	if err := v.populateMemberNames(ctx, members); err != nil {
		return nil, err
	}
	return members, nil
}

// ListMembersForUpdate lists all participants in a room for update.
func (v *ValkeyRoomRepository) ListMembersForUpdate(ctx context.Context, roomID string) ([]RoomMember, error) {
	return v.ListMembers(ctx, roomID)
}

// UpdateMember updates participant state in the room.
func (v *ValkeyRoomRepository) UpdateMember(ctx context.Context, member RoomMember) error {
	return v.WithRoomLock(ctx, member.RoomID, func(lockedCtx context.Context) error {
		dto, err := v.getRoomDetail(lockedCtx, member.RoomID)
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
		return v.saveRoomDetail(lockedCtx, dto)
	})
}

// RemoveMember removes a participant from the room.
func (v *ValkeyRoomRepository) RemoveMember(ctx context.Context, roomID string, characterID string) error {
	return v.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		dto, err := v.getRoomDetail(lockedCtx, roomID)
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
		var syncErr error
		if idx >= 0 {
			dto.Members = append(dto.Members[:idx], dto.Members[idx+1:]...)
			dto.Room.UpdatedAt = time.Now().UTC()
			syncErr = v.saveRoomDetail(lockedCtx, dto)
			if syncErr != nil && !errors.Is(syncErr, ErrSecondarySync) {
				return syncErr
			}
		}
		if err := v.deleteMemberMapping(lockedCtx, roomID, characterID); err != nil {
			syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
		}
		return syncErr
	})
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
	roomID, err := res.ToString()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(roomID) == "" {
		return "", ErrInvalidRoomMapping
	}
	return roomID, nil
}
