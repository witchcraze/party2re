package party

import (
	"context"
	_ "embed"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	// DefaultAdventureLockKeyPrefix is the Valkey key prefix for party adventure distributed locks (Rank 0).
	DefaultAdventureLockKeyPrefix = "party2:party:lock:adventure:"
	// DefaultAdventureLockTTL is the distributed lock expiration safety TTL.
	DefaultAdventureLockTTL = 10 * time.Second
)

//go:embed lua/release_party_lock.lua
var releasePartyLockLua string

// releasePartyLockScript releases the adventure lock atomically only if the caller owns the token.
var releasePartyLockScript = valkey.NewLuaScript(releasePartyLockLua)

// WithPartyAdventureLock executes fn inside an exclusive distributed party adventure lock in Valkey Master.
// If another adventure invocation is currently running for this party, it immediately fails with ErrPartyNotRecruiting.
// It supports reentrant calls within the same context.
func (r *ValkeyRepository) WithPartyAdventureLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error {
	if partyID == "" {
		return fn(ctx)
	}

	if isPartyAdventureLocked(ctx, partyID) {
		return fn(ctx)
	}

	if r.client == nil {
		r.memPartyLocksMu.Lock()
		if _, locked := r.memPartyLocks[partyID]; locked {
			r.memPartyLocksMu.Unlock()
			return ErrPartyNotRecruiting
		}
		token := id.New()
		r.memPartyLocks[partyID] = token
		r.memPartyLocksMu.Unlock()

		defer func() {
			r.memPartyLocksMu.Lock()
			if r.memPartyLocks[partyID] == token {
				delete(r.memPartyLocks, partyID)
			}
			r.memPartyLocksMu.Unlock()
		}()

		lockedCtx := withPartyAdventureLockedContext(ctx, partyID)
		return fn(lockedCtx)
	}

	token := id.New()
	lockKey := DefaultAdventureLockKeyPrefix + partyID
	lockTTL := int64(DefaultAdventureLockTTL.Seconds())
	if lockTTL < 1 {
		lockTTL = 1
	}

	cmd := r.client.B().Set().Key(lockKey).Value(token).Nx().ExSeconds(lockTTL).Build()
	res := r.client.Do(ctx, cmd)
	if res.Error() != nil {
		if valkey.IsValkeyNil(res.Error()) {
			return ErrPartyNotRecruiting
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return res.Error()
	}

	defer func() {
		relCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = releasePartyLockScript.Exec(relCtx, r.client, []string{lockKey}, []string{token})
	}()

	lockedCtx := withPartyAdventureLockedContext(ctx, partyID)
	return fn(lockedCtx)
}
