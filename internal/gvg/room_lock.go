package gvg

import (
	"context"
)

// roomLockContextKey tracks acquired room locks in context for reentrancy.
type roomLockContextKey struct{}

// RoomLockRepository provides room-level distributed locking for atomic multiplayer serialization.
type RoomLockRepository interface {
	WithRoomLock(ctx context.Context, roomID string, fn func(ctx context.Context) error) error
}

// isRoomLocked checks whether the context already holds the exclusive lock for roomID.
func isRoomLocked(ctx context.Context, roomID string) bool {
	if val, ok := ctx.Value(roomLockContextKey{}).(string); ok && val == roomID {
		return true
	}
	return false
}

// withRoomLockedContext returns a new context marked with the acquired roomID lock.
func withRoomLockedContext(ctx context.Context, roomID string) context.Context {
	return context.WithValue(ctx, roomLockContextKey{}, roomID)
}

// withRoomLock executes fn inside an exclusive room lock if a room repository is configured.
func (s *Service) withRoomLock(ctx context.Context, roomID string, fn func(ctx context.Context) error) error {
	if lockRepo, ok := s.repo.(RoomLockRepository); ok && roomID != "" {
		return lockRepo.WithRoomLock(ctx, roomID, fn)
	}
	return fn(ctx)
}
