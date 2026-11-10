package casino

import (
	"context"
	"errors"
	"slices"
	"time"
)

func (v *ValkeyRoomRepository) PublishDeparture(ctx context.Context, d Departure) error {
	return v.WithRoomLock(ctx, d.Detail.Room.ID, func(ctx context.Context) error {
		d.Detail.Room.PasswordHash = d.PasswordHash
		d.Detail.Room.DepartureVersion = d.Version
		ttl := time.Until(d.Detail.Room.UpdatedAt.Add(DefaultLobbyTTL))
		var publishErr error
		if ttl <= 0 {
			// Expired pending publication completes cleanup without reviving the room.
			publishErr = v.DeleteRoom(ctx, d.Detail.Room.ID)
		} else {
			publishErr = v.saveRoomDetailWithTTL(ctx, &valkeyRoomDetailDTO{Room: d.Detail.Room, Members: d.Detail.Members}, "", ttl)
			if publishErr != nil && !errors.Is(publishErr, ErrSecondarySync) {
				return publishErr
			}
		}
		for _, characterID := range d.ClaimedIDs {
			retained := ttl > 0 && d.Detail.Room.Status != RoomStatusDisbanded && slices.ContainsFunc(d.Detail.Members, func(m RoomMember) bool { return m.CharacterID == characterID })
			var err error
			if retained {
				// SQL claims permit restoring missing mappings, but never replacing another room.
				err = v.claimAdmittedMemberMapping(ctx, d.Detail.Room.ID, characterID, "")
			} else {
				err = v.deleteMemberMapping(ctx, d.Detail.Room.ID, characterID)
			}
			if err != nil {
				publishErr = errors.Join(publishErr, &SecondarySyncError{Err: err})
			}
		}
		return publishErr
	})
}

func (m *MemoryRoomRepository) PublishDeparture(ctx context.Context, d Departure) error {
	d.Detail.Room.PasswordHash = d.PasswordHash
	d.Detail.Room.DepartureVersion = d.Version
	expired := !d.Detail.Room.UpdatedAt.Add(DefaultLobbyTTL).After(time.Now())
	if expired {
		if err := m.DeleteRoom(ctx, d.Detail.Room.ID); err != nil {
			return err
		}
	} else {
		m.mu.Lock()
		// A pending snapshot can restore a lost room, just as the real adapter does.
		if _, exists := m.rooms[d.Detail.Room.ID]; !exists {
			m.rooms[d.Detail.Room.ID] = d.Detail.Room
		}
		m.mu.Unlock()
		if err := m.UpdateRoomDetail(ctx, d.Detail); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var syncErr error
	for _, characterID := range d.ClaimedIDs {
		retained := !expired && d.Detail.Room.Status != RoomStatusDisbanded && slices.ContainsFunc(d.Detail.Members, func(member RoomMember) bool { return member.CharacterID == characterID })
		current := m.charRooms[characterID]
		if retained {
			if current != "" && current != d.Detail.Room.ID {
				syncErr = errors.Join(syncErr, &SecondarySyncError{Err: ErrAlreadyInRoom})
			} else {
				m.charRooms[characterID] = d.Detail.Room.ID
			}
		} else if current == d.Detail.Room.ID {
			delete(m.charRooms, characterID)
		}
	}
	return syncErr
}
