package costume

import (
	"context"
	"sync"
	"time"
)

// ClaimChecker verifies whether an exclusive claim token is currently held for a category and target.
type ClaimChecker func(ctx context.Context, category, targetID, token string) bool

// MemoryCostumeRepository provides thread-safe in-memory costume storage.
type MemoryCostumeRepository struct {
	mu           sync.RWMutex
	costumes     map[string]ActiveCostume
	claimChecker ClaimChecker
}

// NewMemoryCostumeRepository creates a new in-memory costume repository.
func NewMemoryCostumeRepository() *MemoryCostumeRepository {
	return &MemoryCostumeRepository{
		costumes: make(map[string]ActiveCostume),
	}
}

// SetClaimChecker registers a claim verification callback for atomic claim-guarded reset.
func (r *MemoryCostumeRepository) SetClaimChecker(checker ClaimChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimChecker = checker
}

// GetActiveCostume retrieves the active costume from memory if not expired.
func (r *MemoryCostumeRepository) GetActiveCostume(_ context.Context, characterID string) (*ActiveCostume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.costumes[characterID]
	if !ok {
		return nil, nil
	}
	if !c.IsActive(time.Now().UTC()) {
		return nil, nil
	}
	res := c
	return &res, nil
}

// SaveActiveCostume stores active costume state in memory.
func (r *MemoryCostumeRepository) SaveActiveCostume(_ context.Context, costume ActiveCostume, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.costumes[costume.CharacterID] = costume
	return nil
}

// ClearActiveCostume deletes active costume rental state from memory.
func (r *MemoryCostumeRepository) ClearActiveCostume(_ context.Context, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.costumes, characterID)
	return nil
}

// ClearActiveCostumeWithClaim atomically deletes active costume rental state only if claimCategory has claimToken
// and the costume has not been modified by a successor cycle.
func (r *MemoryCostumeRepository) ClearActiveCostumeWithClaim(ctx context.Context, characterID, claimCategory, claimToken string) (bool, error) {
	if r.claimChecker == nil {
		// When claimChecker is not configured, ownership cannot be verified atomically.
		// Refuse unsafe deletion to preserve successor state.
		return false, nil
	}

	r.mu.RLock()
	current, exists := r.costumes[characterID]
	r.mu.RUnlock()
	if !exists {
		// Nothing to clear, but verify claim token validity
		if !r.claimChecker(ctx, claimCategory, characterID, claimToken) {
			return false, nil
		}
		return true, nil
	}

	// Verify claim ownership
	if !r.claimChecker(ctx, claimCategory, characterID, claimToken) {
		return false, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure the costume was not replaced by a successor cycle after claim verification
	active, stillExists := r.costumes[characterID]
	if !stillExists {
		return true, nil
	}
	if !active.RentedAt.Equal(current.RentedAt) || active.ItemNo != current.ItemNo {
		// Replaced by a successor cycle; do not delete the successor's new costume!
		return false, nil
	}

	delete(r.costumes, characterID)
	return true, nil
}
