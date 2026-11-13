package casino

import (
	"context"
	_ "embed"
	"errors"
	"sync/atomic"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	// DefaultRoomLockKeyPrefix is the Valkey key prefix for room-level distributed locks.
	DefaultRoomLockKeyPrefix = "party2:casino:lock:room:"
	// DefaultCharacterLockKeyPrefix serializes admission across independently locked rooms.
	DefaultCharacterLockKeyPrefix = "party2:casino:lock:character:"
	// DefaultRoomLockTTL is the distributed lock expiration safety TTL.
	DefaultRoomLockTTL = 10 * time.Second
	// DefaultRoomLockRetryWait is the backoff sleep between failed lock attempts.
	DefaultRoomLockRetryWait = 10 * time.Millisecond
	// DefaultRoomLockTimeout is the fallback wait to acquire a room lock when the caller context has no deadline.
	DefaultRoomLockTimeout = 5 * time.Second
)

//go:embed lua/release_room_lock.lua
var releaseRoomLockLua string

// releaseRoomLockScript releases the room lock atomically only if the caller owns the token.
var releaseRoomLockScript = valkey.NewLuaScript(releaseRoomLockLua)

// roomLockObserver is an optional test hook to measure wait and hold timings.
var roomLockObserver atomic.Pointer[func(waitDur, holdDur time.Duration, acquired bool, err error)]

// WithRoomLock executes fn inside an exclusive distributed room lock in Valkey Master.
// It supports reentrant calls within the same context.
func (v *ValkeyRoomRepository) WithRoomLock(ctx context.Context, roomID string, fn func(ctx context.Context) error) error {
	if roomID == "" {
		return fn(ctx)
	}

	if isRoomLocked(ctx, roomID) {
		return fn(ctx)
	}
	startWait := time.Now()
	var startHold time.Time
	err := v.withLockKey(ctx, DefaultRoomLockKeyPrefix+roomID, func(ctx context.Context) error {
		startHold = time.Now()
		return fn(withRoomLockedContext(ctx, roomID))
	})
	if observerPtr := roomLockObserver.Load(); observerPtr != nil {
		observer := *observerPtr
		if startHold.IsZero() {
			observer(time.Since(startWait), 0, false, err)
		} else {
			observer(startHold.Sub(startWait), time.Since(startHold), true, err)
		}
	}
	return err
}

// Room locks always precede the character admission lock; admission never locks another room.
func (v *ValkeyRoomRepository) withAdmissionLock(ctx context.Context, roomID, characterID string, fn func(context.Context) error) error {
	return v.WithRoomLock(ctx, roomID, func(ctx context.Context) error {
		return v.withLockKey(ctx, DefaultCharacterLockKeyPrefix+characterID, fn)
	})
}

func (v *ValkeyRoomRepository) withLockKey(ctx context.Context, lockKey string, fn func(context.Context) error) error {
	token := id.New()
	lockTTL := int64(DefaultRoomLockTTL.Seconds())
	if lockTTL < 1 {
		lockTTL = 1
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(DefaultRoomLockTimeout)
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
			return res.Error()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(DefaultRoomLockRetryWait):
		}
	}

	if !acquired {
		return errors.New("failed to acquire casino lock: timeout")
	}

	defer func() {
		relCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = releaseRoomLockScript.Exec(relCtx, v.client, []string{lockKey}, []string{token})
	}()

	return fn(ctx)
}
