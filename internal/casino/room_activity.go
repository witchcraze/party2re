package casino

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

var ErrInvalidRoomMapping = errors.New("casino character room mapping is invalid")

// HasCharacterRoom reveals only actual admission, including spectators, without renewal.
func (s *Service) HasCharacterRoom(ctx context.Context, characterID string) (bool, error) {
	if s.roomRepo == nil {
		return false, errors.New("casino activity reader is required")
	}
	if err := s.checkPendingDeparture(ctx, characterID); err != nil {
		return false, err
	}
	roomID, err := s.roomRepo.GetCharacterRoom(ctx, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(roomID) == "" {
		return false, ErrInvalidRoomMapping
	}
	room, err := s.roomRepo.GetRoom(ctx, roomID)
	if errors.Is(err, ErrRoomNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if room.Status == RoomStatusDisbanded || room.UpdatedAt.Before(time.Now().UTC().Add(-IdleRoomTimeout)) {
		return false, nil
	}
	members, err := s.roomRepo.ListMembers(ctx, roomID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.CharacterID == characterID {
			return true, nil
		}
	}
	return false, nil
}

// GetCharacterRoomView discovers actual admission through the owner index and
// reuses the authorized, masked projection. Missing/expired rooms are absent.
func (s *Service) GetCharacterRoomView(ctx context.Context, playerID, characterID string) (*RoomView, error) {
	if playerID == "" || characterID == "" {
		return nil, ErrRoomViewForbidden
	}
	if s.charRepo == nil || s.roomRepo == nil {
		return nil, errors.New("casino activity readers are required")
	}
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if char.ID != characterID || char.PlayerID != playerID {
		return nil, ErrRoomViewForbidden
	}
	if err := s.checkPendingDeparture(ctx, characterID); err != nil {
		return nil, err
	}
	roomID, err := s.roomRepo.GetCharacterRoom(ctx, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, ErrInvalidRoomMapping
	}
	view, err := s.GetRoomView(ctx, roomID, playerID, characterID)
	if errors.Is(err, ErrRoomNotFound) || errors.Is(err, ErrRoomViewForbidden) {
		return nil, nil
	}
	return view, err
}

// RenewRoom restarts an admitted member's room idle lifetime for a room log
// write, since legacy measures idleness by the log's mtime (casino.cgi:124-129).
func (s *Service) RenewRoom(ctx context.Context, roomID, characterID string) error {
	if s.roomRepo == nil {
		return errors.New("room repository is required")
	}
	return s.withRoomMutationLock(ctx, roomID, characterID, func(lockedCtx context.Context) error {
		room, err := s.roomRepo.GetRoomForUpdate(lockedCtx, roomID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if room.Status == RoomStatusDisbanded || room.UpdatedAt.Before(now.Add(-IdleRoomTimeout)) {
			return ErrRoomNotFound
		}
		members, err := s.roomRepo.ListMembersForUpdate(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(members, func(m RoomMember) bool { return m.CharacterID == characterID }) {
			return ErrMemberNotFound
		}
		room.UpdatedAt = now
		return s.roomRepo.UpdateRoomDetail(lockedCtx, RoomDetail{Room: *room, Members: members})
	})
}
