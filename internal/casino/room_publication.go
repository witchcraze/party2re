package casino

import (
	"context"
	_ "embed"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

var (
	//go:embed lua/claim_character_room.lua
	claimCharacterRoomLua string
	//go:embed lua/renew_character_room.lua
	renewCharacterRoomLua string
	//go:embed lua/delete_character_room.lua
	deleteCharacterRoomLua string

	claimCharacterRoomScript  = valkey.NewLuaScript(claimCharacterRoomLua)
	renewCharacterRoomScript  = valkey.NewLuaScript(renewCharacterRoomLua)
	deleteCharacterRoomScript = valkey.NewLuaScript(deleteCharacterRoomLua)
)

func mapCharacterScriptError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "casino character room mapping is invalid"):
		return ErrInvalidRoomMapping
	case strings.Contains(msg, "casino character already in room"):
		return ErrAlreadyInRoom
	default:
		return err
	}
}

func (v *ValkeyRoomRepository) isCharacterAdmittedInLiveRoom(ctx context.Context, roomID, characterID string) (bool, error) {
	dto, err := v.getRoomDetail(ctx, roomID)
	if errors.Is(err, ErrRoomNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if dto.Room.Status == RoomStatusDisbanded || dto.Room.UpdatedAt.Before(time.Now().UTC().Add(-DefaultLobbyTTL)) {
		return false, nil
	}
	return slices.ContainsFunc(dto.Members, func(m RoomMember) bool {
		return m.CharacterID == characterID
	}), nil
}

func (v *ValkeyRoomRepository) checkCharacterAdmissionAvailable(ctx context.Context, roomID, characterID string) (string, error) {
	currentRoomID, err := v.GetCharacterRoom(ctx, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if currentRoomID == roomID {
		return roomID, nil
	}
	live, err := v.isCharacterAdmittedInLiveRoom(ctx, currentRoomID, characterID)
	if err != nil {
		return "", err
	}
	if live {
		return "", ErrAlreadyInRoom
	}
	return currentRoomID, nil
}

func (v *ValkeyRoomRepository) claimAdmittedMemberMapping(ctx context.Context, roomID, characterID, expectedPrevRoomID string) error {
	charKey := DefaultCharacterKeyPrefix + characterID
	ttlSeconds := strconv.FormatInt(int64(DefaultLobbyTTL/time.Second), 10)
	err := claimCharacterRoomScript.Exec(ctx, v.client, []string{charKey}, []string{roomID, expectedPrevRoomID, ttlSeconds}).Error()
	return mapCharacterScriptError(err)
}

func (v *ValkeyRoomRepository) renewMemberMapping(ctx context.Context, roomID, characterID string) error {
	charKey := DefaultCharacterKeyPrefix + characterID
	ttlSeconds := strconv.FormatInt(int64(DefaultLobbyTTL/time.Second), 10)
	err := renewCharacterRoomScript.Exec(ctx, v.client, []string{charKey}, []string{roomID, ttlSeconds}).Error()
	return mapCharacterScriptError(err)
}

func (v *ValkeyRoomRepository) deleteMemberMapping(ctx context.Context, roomID, characterID string) error {
	charKey := DefaultCharacterKeyPrefix + characterID
	err := deleteCharacterRoomScript.Exec(ctx, v.client, []string{charKey}, []string{roomID}).Error()
	return mapCharacterScriptError(err)
}

// UpdateRoomDetail replaces room/member state under the command's room lock.
// All required reads precede the single payload SET; mapping synchronization is secondary.
func (v *ValkeyRoomRepository) UpdateRoomDetail(ctx context.Context, detail RoomDetail) error {
	return v.WithRoomLock(ctx, detail.Room.ID, func(lockedCtx context.Context) error {
		previous, err := v.getRoomDetail(lockedCtx, detail.Room.ID)
		if err != nil {
			return err
		}
		dto := &valkeyRoomDetailDTO{Room: detail.Room, Members: detail.Members}
		syncErr := v.saveRoomDetail(lockedCtx, dto)
		if syncErr != nil && !errors.Is(syncErr, ErrSecondarySync) {
			return syncErr
		}
		for _, member := range previous.Members {
			if !slices.ContainsFunc(detail.Members, func(m RoomMember) bool { return m.CharacterID == member.CharacterID }) {
				if err := v.deleteMemberMapping(lockedCtx, detail.Room.ID, member.CharacterID); err != nil {
					syncErr = errors.Join(syncErr, &SecondarySyncError{Err: err})
				}
			}
		}
		return syncErr
	})
}

// UpdateRoomDetail applies the same complete-snapshot boundary as the Valkey adapter.
func (m *MemoryRoomRepository) UpdateRoomDetail(_ context.Context, detail RoomDetail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.rooms[detail.Room.ID]; !exists {
		return ErrRoomNotFound
	}
	for charID := range m.members[detail.Room.ID] {
		if detail.Room.Status == RoomStatusDisbanded || !slices.ContainsFunc(detail.Members, func(member RoomMember) bool { return member.CharacterID == charID }) {
			if m.charRooms[charID] == detail.Room.ID {
				delete(m.charRooms, charID)
			}
		}
	}
	m.rooms[detail.Room.ID] = detail.Room
	m.members[detail.Room.ID] = make(map[string]RoomMember, len(detail.Members))
	for _, member := range detail.Members {
		m.members[detail.Room.ID][member.CharacterID] = member
	}
	return nil
}
