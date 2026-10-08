package casino

import (
	"context"
	"errors"
)

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
	roomID, err := s.roomRepo.GetCharacterRoom(ctx, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if roomID == "" {
		return nil, nil
	}
	view, err := s.GetRoomView(ctx, roomID, playerID, characterID)
	if errors.Is(err, ErrRoomNotFound) {
		return nil, nil
	}
	return view, err
}
