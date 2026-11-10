package casino

import (
	"context"
	"errors"
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
