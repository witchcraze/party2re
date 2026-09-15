package casino

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	// DefaultRoomLockKeyPrefix is the Valkey key prefix for room-level distributed locks.
	DefaultRoomLockKeyPrefix = "party2:casino:lock:room:"
	// DefaultRoomLockTTL is the distributed lock expiration safety TTL.
	DefaultRoomLockTTL = 10 * time.Second
	// DefaultRoomLockRetryWait is the backoff sleep between failed lock attempts.
	DefaultRoomLockRetryWait = 10 * time.Millisecond
	// DefaultRoomLockTimeout is the maximum time to wait to acquire a room lock.
	DefaultRoomLockTimeout = 5 * time.Second
)

//go:embed lua/release_room_lock.lua
var releaseRoomLockLua string

// releaseRoomLockScript releases the room lock atomically only if the caller owns the token.
var releaseRoomLockScript = valkey.NewLuaScript(releaseRoomLockLua)

// WithRoomLock executes fn inside an exclusive distributed room lock in Valkey Master.
// It supports reentrant calls within the same context.
func (v *ValkeyRoomRepository) WithRoomLock(ctx context.Context, roomID string, fn func(ctx context.Context) error) error {
	if roomID == "" {
		return fn(ctx)
	}

	if isRoomLocked(ctx, roomID) {
		return fn(ctx)
	}

	token := id.New()
	lockKey := DefaultRoomLockKeyPrefix + roomID
	lockTTL := int64(DefaultRoomLockTTL.Seconds())
	if lockTTL < 1 {
		lockTTL = 1
	}

	deadline := time.Now().Add(DefaultRoomLockTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	acquired := false
	for time.Now().Before(deadline) {
		cmd := v.client.B().Set().Key(lockKey).Value(token).Nx().ExSeconds(lockTTL).Build()
		res := v.client.Do(ctx, cmd)
		if res.Error() == nil {
			acquired = true
			break
		}
		if !valkey.IsValkeyNil(res.Error()) {
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(DefaultRoomLockRetryWait):
		}
	}

	if !acquired {
		return errors.New("failed to acquire casino room lock: timeout")
	}

	defer func() {
		relCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = releaseRoomLockScript.Exec(relCtx, v.client, []string{lockKey}, []string{token})
	}()

	lockedCtx := withRoomLockedContext(ctx, roomID)
	return fn(lockedCtx)
}
