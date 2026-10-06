package casino

import "context"

// RoomSnapshotForTest exposes the trusted command snapshot to existing game fixtures.
// Production callers can only observe rooms through the authenticated GetRoomView.
func RoomSnapshotForTest(s *Service, ctx context.Context, roomID, characterID string) (*RoomDetail, error) {
	return s.getRoomDetail(ctx, roomID, characterID)
}
