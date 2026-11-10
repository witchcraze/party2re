package casino

import (
	"context"
	"encoding/json"
	"sync"
)

// MemoryDepartureRepository is an explicitly injected test adapter.
type MemoryDepartureRepository struct {
	mu     sync.Mutex
	states map[string][]byte
	claims map[string]string
}

func NewMemoryDepartureRepository() *MemoryDepartureRepository {
	return &MemoryDepartureRepository{states: make(map[string][]byte), claims: make(map[string]string)}
}

func (r *MemoryDepartureRepository) FindDeparture(_ context.Context, roomID string) (Departure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.find(roomID)
}

func (r *MemoryDepartureRepository) find(roomID string) (Departure, error) {
	var d Departure
	if data := r.states[roomID]; len(data) > 0 {
		if err := json.Unmarshal(data, &d); err != nil {
			return Departure{}, err
		}
	}
	d.Detail.Room.PasswordHash = d.PasswordHash
	d.Detail.Room.DepartureVersion = d.Version
	return d, nil
}

func (r *MemoryDepartureRepository) FindPendingDeparture(_ context.Context, characterID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.claims[characterID], nil
}

func (r *MemoryDepartureRepository) CommitDeparture(ctx context.Context, d Departure, mutate func(context.Context) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous, err := r.find(d.Detail.Room.ID)
	if err != nil {
		return err
	}
	if previous.Pending || previous.Version+1 != d.Version {
		return ErrDepartureRecovery
	}
	for _, characterID := range d.ClaimedIDs {
		if r.claims[characterID] != "" {
			return ErrDepartureRecovery
		}
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := mutate(ctx); err != nil {
		return err
	}
	for _, characterID := range d.ClaimedIDs {
		r.claims[characterID] = d.Detail.Room.ID
	}
	r.states[d.Detail.Room.ID] = data
	return nil
}

func (r *MemoryDepartureRepository) AcknowledgeDeparture(_ context.Context, roomID string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, err := r.find(roomID)
	if err != nil {
		return err
	}
	if d.Version != version {
		return ErrDepartureRecovery
	}
	d.Pending = false
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	for _, characterID := range d.ClaimedIDs {
		if r.claims[characterID] == roomID {
			delete(r.claims, characterID)
		}
	}
	r.states[roomID] = data
	return nil
}
