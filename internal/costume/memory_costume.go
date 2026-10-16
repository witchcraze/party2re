package costume

import (
	"context"
	"sync"
	"time"
)

// ClaimExecutor executes an action within the exclusive claim ownership boundary.
type ClaimExecutor func(ctx context.Context, category, targetID, token string, action func() error) (bool, error)

// MemoryCostumeRepository provides thread-safe in-memory costume storage.
type MemoryCostumeRepository struct {
	mu            sync.RWMutex
	costumes      map[string]ActiveCostume
	claimExecutor ClaimExecutor
}

// NewMemoryCostumeRepository creates a new in-memory costume repository.
func NewMemoryCostumeRepository() *MemoryCostumeRepository {
	return &MemoryCostumeRepository{
		costumes: make(map[string]ActiveCostume),
	}
}

// SetClaimExecutor registers an atomic claim execution boundary for claim-guarded reset.
func (r *MemoryCostumeRepository) SetClaimExecutor(executor ClaimExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimExecutor = executor
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

// ClearActiveCostumeWithClaim atomically deletes active costume rental state within the exclusive claim boundary.
func (r *MemoryCostumeRepository) ClearActiveCostumeWithClaim(ctx context.Context, characterID, claimCategory, claimToken string) (bool, error) {
	if r.claimExecutor == nil {
		// When claimExecutor is not configured, ownership cannot be verified atomically.
		// Refuse unsafe deletion to preserve successor state.
		return false, nil
	}

	return r.claimExecutor(ctx, claimCategory, characterID, claimToken, func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.costumes, characterID)
		return nil
	})
}
