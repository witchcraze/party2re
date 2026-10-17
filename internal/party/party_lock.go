package party

import "context"

// partyAdventureLockContextKey tracks acquired party adventure locks in context for reentrancy.
type partyAdventureLockContextKey struct{}

// PartyAdventureLockRepository provides party-level distributed locking for atomic adventure serialization (Rank 0).
type PartyAdventureLockRepository interface {
	WithPartyAdventureLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error
}

// isPartyAdventureLocked checks whether ctx already holds the exclusive adventure lock for partyID.
func isPartyAdventureLocked(ctx context.Context, partyID string) bool {
	if locks, ok := ctx.Value(partyAdventureLockContextKey{}).(map[string]bool); ok {
		return locks[partyID]
	}
	if val, ok := ctx.Value(partyAdventureLockContextKey{}).(string); ok && val == partyID {
		return true
	}
	return false
}

// withPartyAdventureLockedContext returns a new context marked with the acquired partyID adventure lock.
func withPartyAdventureLockedContext(ctx context.Context, partyID string) context.Context {
	existing, _ := ctx.Value(partyAdventureLockContextKey{}).(map[string]bool)
	newLocks := make(map[string]bool, len(existing)+1)
	for k, v := range existing {
		newLocks[k] = v
	}
	if single, ok := ctx.Value(partyAdventureLockContextKey{}).(string); ok {
		newLocks[single] = true
	}
	newLocks[partyID] = true
	return context.WithValue(ctx, partyAdventureLockContextKey{}, newLocks)
}

// PartyLocker provides party-level distributed locking for atomic run and adventure serialization (Rank 0).
type PartyLocker interface {
	WithPartyLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error
}
