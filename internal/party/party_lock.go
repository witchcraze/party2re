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
	if val, ok := ctx.Value(partyAdventureLockContextKey{}).(string); ok && val == partyID {
		return true
	}
	return false
}

// withPartyAdventureLockedContext returns a new context marked with the acquired partyID adventure lock.
func withPartyAdventureLockedContext(ctx context.Context, partyID string) context.Context {
	return context.WithValue(ctx, partyAdventureLockContextKey{}, partyID)
}
