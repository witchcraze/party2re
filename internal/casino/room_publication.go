package casino

import (
	"context"
	"errors"
	"slices"
)

// UpdateRoomDetail replaces room/member state under the command's room lock.
// All required reads precede the single payload SET; mapping cleanup is secondary.
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
				key := DefaultCharacterKeyPrefix + member.CharacterID
				if err := v.client.Do(lockedCtx, v.client.B().Del().Key(key).Build()).Error(); err != nil {
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
			delete(m.charRooms, charID)
		}
	}
	m.rooms[detail.Room.ID] = detail.Room
	m.members[detail.Room.ID] = make(map[string]RoomMember, len(detail.Members))
	for _, member := range detail.Members {
		m.members[detail.Room.ID][member.CharacterID] = member
	}
	return nil
}
