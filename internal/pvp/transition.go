package pvp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

var ErrTransitionConflict = errors.New("colosseum transition changed; retry the operation")

// Transition is the durable financial claim and the exact publication to retry.
// PasswordHash is persisted separately because the public room DTO omits it.
type Transition struct {
	Version      int
	Kind         string
	ActorID      string
	Detail       RoomDetail
	PasswordHash string
	AddedID      string
	RemovedIDs   []string
	Result       *RoundResolution
	Pending      bool
}

// TransitionRepository commits a unique room version with its asset mutation.
// Removed membership claims remain reserved until publication is acknowledged.
type TransitionRepository interface {
	FindTransition(context.Context, string) (Transition, error)
	FindCharacterClaim(context.Context, string) (string, error)
	CommitTransition(context.Context, Transition, func(context.Context) error) error
	AcknowledgeTransition(context.Context, string, int) error
}

func WithTransitionRepository(repo TransitionRepository) Option {
	return func(s *Service) { s.transitions = repo }
}

type transitionVersionKey struct{}

func (s *Service) commitTransition(ctx context.Context, t Transition, mutate func(context.Context) error) error {
	version, _ := ctx.Value(transitionVersionKey{}).(int)
	if t.Detail.Room.FinancialVersion != version {
		return ErrTransitionConflict
	}
	t.Version = version + 1
	t.Detail.Room.FinancialVersion = t.Version
	t.Pending = true
	t.PasswordHash = t.Detail.Room.PasswordHash
	if err := s.transitions.CommitTransition(ctx, t, func(txCtx context.Context) error {
		return s.runInTx(txCtx, mutate)
	}); err != nil {
		return err
	}
	return s.publishTransition(ctx, t)
}

func (s *Service) publishTransition(ctx context.Context, t Transition) error {
	t.Detail.Room.PasswordHash = t.PasswordHash
	t.Detail.Room.FinancialVersion = t.Version
	if t.Detail.Room.Status == StatusDisbanded {
		if err := s.repo.DeleteRoom(ctx, t.Detail.Room.ID); err != nil {
			return err
		}
	} else {
		if err := s.repo.SaveRoom(ctx, t.Detail.Room, t.Detail.Members); err != nil {
			return err
		}
		for _, m := range t.Detail.Members {
			if err := s.repo.SetCharacterRoom(ctx, m.CharacterID, t.Detail.Room.ID); err != nil {
				return err
			}
		}
	}
	for _, characterID := range t.RemovedIDs {
		if err := s.repo.DeleteCharacterRoom(ctx, characterID, t.Detail.Room.ID); err != nil {
			return err
		}
	}
	return s.transitions.AcknowledgeTransition(ctx, t.Detail.Room.ID, t.Version)
}

// recoverCharacter also finds creation that failed before publishing a reverse mapping.
func (s *Service) recoverCharacter(ctx context.Context, characterID string) (Transition, error) {
	roomID, err := s.transitions.FindCharacterClaim(ctx, characterID)
	if err != nil || roomID == "" {
		return Transition{}, err
	}
	var recovered Transition
	err = s.repo.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		t, err := s.transitions.FindTransition(lockedCtx, roomID)
		if err != nil || !t.Pending {
			return err
		}
		if err := s.publishTransition(lockedCtx, t); err != nil {
			return err
		}
		recovered = t
		return nil
	})
	return recovered, err
}

// MemoryTransitionRepository is an explicitly injected test adapter.
type MemoryTransitionRepository struct {
	mu     sync.Mutex
	states map[string][]byte
	claims map[string]string
}

func NewMemoryTransitionRepository() *MemoryTransitionRepository {
	return &MemoryTransitionRepository{states: make(map[string][]byte), claims: make(map[string]string)}
}

func (r *MemoryTransitionRepository) FindTransition(_ context.Context, roomID string) (Transition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var t Transition
	if data := r.states[roomID]; len(data) > 0 {
		if err := json.Unmarshal(data, &t); err != nil {
			return Transition{}, err
		}
	}
	t.Detail.Room.FinancialVersion = t.Version
	t.Detail.Room.PasswordHash = t.PasswordHash
	return t, nil
}

func (r *MemoryTransitionRepository) FindCharacterClaim(_ context.Context, characterID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.claims[characterID], nil
}

func (r *MemoryTransitionRepository) CommitTransition(ctx context.Context, t Transition, mutate func(context.Context) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var previous Transition
	if data := r.states[t.Detail.Room.ID]; len(data) > 0 {
		if err := json.Unmarshal(data, &previous); err != nil {
			return err
		}
	}
	if previous.Version+1 != t.Version || previous.Pending || previous.Detail.Room.Status == StatusCompleted || previous.Detail.Room.Status == StatusDisbanded {
		return ErrTransitionConflict
	}
	if t.AddedID != "" && r.claims[t.AddedID] != "" {
		return ErrAlreadyInRoom
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := mutate(ctx); err != nil {
		return err
	}
	if t.AddedID != "" {
		r.claims[t.AddedID] = t.Detail.Room.ID
	}
	r.states[t.Detail.Room.ID] = data
	return nil
}

func (r *MemoryTransitionRepository) AcknowledgeTransition(_ context.Context, roomID string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var t Transition
	if err := json.Unmarshal(r.states[roomID], &t); err != nil {
		return err
	}
	if t.Version != version {
		return ErrTransitionConflict
	}
	for _, characterID := range t.RemovedIDs {
		if r.claims[characterID] == roomID {
			delete(r.claims, characterID)
		}
	}
	if t.Detail.Room.Status == StatusCompleted {
		for _, m := range t.Detail.Members {
			if r.claims[m.CharacterID] == roomID {
				delete(r.claims, m.CharacterID)
			}
		}
	}
	t.Pending = false
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	r.states[roomID] = data
	return nil
}
