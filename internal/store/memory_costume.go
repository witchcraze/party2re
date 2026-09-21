package store

import (
	"context"
	"sync"
	"time"
)

// MemoryCostumeRepository provides thread-safe in-memory costume storage.
type MemoryCostumeRepository struct {
	mu       sync.RWMutex
	costumes map[string]ActiveCostume
}

func NewMemoryCostumeRepository() *MemoryCostumeRepository {
	return &MemoryCostumeRepository{
		costumes: make(map[string]ActiveCostume),
	}
}

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

func (r *MemoryCostumeRepository) SaveActiveCostume(_ context.Context, costume ActiveCostume, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.costumes[costume.CharacterID] = costume
	return nil
}

func (r *MemoryCostumeRepository) ClearActiveCostume(_ context.Context, characterID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.costumes, characterID)
	return nil
}
