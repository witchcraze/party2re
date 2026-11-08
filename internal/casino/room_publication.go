package casino

import (
	"context"
	"errors"
	"slices"
	"time"
)

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

func (v *ValkeyRoomRepository) setAdmittedMemberMapping(ctx context.Context, roomID, characterID string) error {
	currentRoomID, err := v.GetCharacterRoom(ctx, characterID)
	if err != nil && !errors.Is(err, ErrRoomNotFound) {
		return err
	}
	if err == nil && currentRoomID != roomID {
		live, err := v.isCharacterAdmittedInLiveRoom(ctx, currentRoomID, characterID)
		if err != nil {
			return err
		}
		if live {
			return nil
		}
	}
	charKey := DefaultCharacterKeyPrefix + characterID
	charCmd := v.client.B().Set().Key(charKey).Value(roomID).Ex(DefaultLobbyTTL).Build()
	return v.client.Do(ctx, charCmd).Error()
}

func (v *ValkeyRoomRepository) renewMemberMapping(ctx context.Context, roomID, characterID string) error {
	currentRoomID, err := v.GetCharacterRoom(ctx, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if currentRoomID != roomID {
		return nil
	}
	charKey := DefaultCharacterKeyPrefix + characterID
	charCmd := v.client.B().Set().Key(charKey).Value(roomID).Ex(DefaultLobbyTTL).Build()
	return v.client.Do(ctx, charCmd).Error()
}

func (v *ValkeyRoomRepository) deleteMemberMapping(ctx context.Context, roomID, characterID string) error {
	currentRoomID, err := v.GetCharacterRoom(ctx, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if currentRoomID != roomID {
		return nil
	}
	charKey := DefaultCharacterKeyPrefix + characterID
	return v.client.Do(ctx, v.client.B().Del().Key(charKey).Build()).Error()
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
