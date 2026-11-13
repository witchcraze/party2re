package casino

import (
	"context"
	"time"
)

// RoomSnapshotForTest exposes the trusted command snapshot to existing game fixtures.
// Production callers can only observe rooms through the authenticated GetRoomView.
func RoomSnapshotForTest(s *Service, ctx context.Context, roomID, characterID string) (*RoomDetail, error) {
	return s.getRoomDetail(ctx, roomID, characterID)
}

// SetRoomLockObserverForTest sets a temporary hook to observe room lock wait and hold timings.
func SetRoomLockObserverForTest(observer func(waitDur, holdDur time.Duration, acquired bool, err error)) func() {
	var next *func(waitDur, holdDur time.Duration, acquired bool, err error)
	if observer != nil {
		next = &observer
	}
	prev := roomLockObserver.Swap(next)
	return func() {
		roomLockObserver.Store(prev)
	}
}
