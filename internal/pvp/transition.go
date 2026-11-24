package pvp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
)

var ErrTransitionConflict = errors.New("colosseum transition changed; retry the operation")

// Transition is the durable financial claim and the exact publication to retry.
// PasswordHash and PendingPvPKills are persisted separately because ColosseumRoom omits them from public JSON.
type Transition struct {
	Version         int
	Kind            string
	ActorID         string
	Detail          RoomDetail
	PasswordHash    string
	PendingPvPKills map[string]int
	AddedID         string
	RemovedIDs      []string
	Result          *RoundResolution
	ForfeitedPool   int
	LastActivityAt  time.Time
	PenaltyUntil    time.Time
	Pending         bool
}

// TransitionRepository commits a unique room version with its asset mutation.
// Removed membership claims remain reserved until publication is acknowledged.
type TransitionRepository interface {
	FindTransition(context.Context, string) (Transition, error)
	FindCharacterClaim(context.Context, string) (string, error)
	CommitTransition(context.Context, Transition, func(context.Context) error) error
	AcknowledgeTransition(context.Context, string, int) error
}

// IsActivityTransitionKind reports whether kind is a room-activity writer subject to inactivity guards.
func IsActivityTransitionKind(kind string) bool {
	return kind == "select" || kind == "start" || kind == "advance" || kind == "kick" || kind == "host"
}

func WithTransitionRepository(repo TransitionRepository) Option {
	return func(s *Service) { s.transitions = repo }
}

// GetCharacterClaim returns the room whose SQL participation claim still holds the character,
// including claims whose Valkey publication is missing. It does not publish or repair.
func (s *Service) GetCharacterClaim(ctx context.Context, characterID string) (string, error) {
	return s.transitions.FindCharacterClaim(ctx, characterID)
}

type transitionVersionKey struct{}

func (s *Service) commitTransition(ctx context.Context, t Transition, mutate func(context.Context) error) error {
	t, err := s.commitSQL(ctx, t, mutate)
	if err != nil {
		return err
	}
	published, err := s.publishTransition(ctx, t)
	if err != nil {
		return err
	}
	if t.Kind != "timeout" && published.Kind == "timeout" {
		if t.Kind == "leave" && slices.Contains(published.RemovedIDs, t.ActorID) {
			return nil
		}
		return ErrRoomInactive
	}
	return nil
}

// commitSQL commits t as the room's next pending version; publication follows
// only after the caller's transaction commits.
func (s *Service) commitSQL(ctx context.Context, t Transition, mutate func(context.Context) error) (Transition, error) {
	version, _ := ctx.Value(transitionVersionKey{}).(int)
	if t.Detail.Room.FinancialVersion != version {
		return Transition{}, ErrTransitionConflict
	}
	t.Version = version + 1
	t.Detail.Room.FinancialVersion = t.Version
	t.Pending = true
	t.PasswordHash = t.Detail.Room.PasswordHash
	t.PendingPvPKills = cloneIntMap(t.Detail.Room.PendingPvPKills)
	if t.LastActivityAt.IsZero() {
		t.LastActivityAt = t.Detail.Room.LastActivityAt
	}
	if t.LastActivityAt.IsZero() {
		t.LastActivityAt = t.Detail.Room.CreatedAt
	}
	t.Detail.Room.LastActivityAt = t.LastActivityAt
	if mutate == nil {
		mutate = func(context.Context) error { return nil }
	}
	if err := s.transitions.CommitTransition(ctx, t, func(txCtx context.Context) error {
		return s.runInTx(txCtx, mutate)
	}); err != nil {
		return Transition{}, err
	}
	return t, nil
}

func (s *Service) publishTransition(ctx context.Context, t Transition) (Transition, error) {
	t.Detail.Room.PasswordHash = t.PasswordHash
	t.Detail.Room.PendingPvPKills = cloneIntMap(t.PendingPvPKills)
	t.Detail.Room.FinancialVersion = t.Version
	t.Detail.Room.LastActivityAt = t.LastActivityAt
	if t.Detail.Room.Status == StatusDisbanded {
		if err := s.repo.DeleteRoom(ctx, t.Detail.Room.ID); err != nil {
			return Transition{}, err
		}
		for _, characterID := range t.RemovedIDs {
			if err := s.repo.DeleteCharacterRoom(ctx, characterID, t.Detail.Room.ID); err != nil {
				return Transition{}, err
			}
		}
		if t.Kind == "timeout" {
			if s.timers == nil {
				return Transition{}, ErrInvalidDependencies
			}
			for _, characterID := range t.RemovedIDs {
				if err := s.timers.ExtendLockUntil(ctx, timer.CategorySleep, characterID, t.PenaltyUntil); err != nil {
					return Transition{}, err
				}
			}
		}
		if err := s.transitions.AcknowledgeTransition(ctx, t.Detail.Room.ID, t.Version); err != nil {
			return Transition{}, err
		}
		return t, nil
	}

	now := s.now()
	if s.isInactive(t.LastActivityAt, now) {
		if _, roomErr := s.repo.GetRoom(ctx, t.Detail.Room.ID); roomErr != nil && !errors.Is(roomErr, ErrRoomNotFound) {
			return Transition{}, roomErr
		}
		if s.timers == nil {
			return Transition{}, ErrInvalidDependencies
		}
		for _, characterID := range t.RemovedIDs {
			if err := s.repo.DeleteCharacterRoom(ctx, characterID, t.Detail.Room.ID); err != nil {
				return Transition{}, err
			}
		}
		if err := s.transitions.AcknowledgeTransition(ctx, t.Detail.Room.ID, t.Version); err != nil {
			return Transition{}, err
		}
		timeoutCtx := context.WithValue(ctx, transitionVersionKey{}, t.Version)
		return s.cleanupInactiveRoom(timeoutCtx, t, now)
	}

	ttl := DefaultLobbyTTL
	if !t.LastActivityAt.IsZero() {
		ttl = t.LastActivityAt.Add(InactivityTimeout).Sub(now)
	}
	if ttl < time.Second {
		ttl = time.Second
	}

	ttlCtx := withRemainingTTL(ctx, ttl)
	if err := s.repo.SaveRoom(ttlCtx, t.Detail.Room, t.Detail.Members); err != nil {
		return Transition{}, err
	}
	for _, m := range t.Detail.Members {
		if err := s.repo.SetCharacterRoom(ttlCtx, m.CharacterID, t.Detail.Room.ID); err != nil {
			return Transition{}, err
		}
	}
	for _, characterID := range t.RemovedIDs {
		if err := s.repo.DeleteCharacterRoom(ctx, characterID, t.Detail.Room.ID); err != nil {
			return Transition{}, err
		}
	}
	if t.Kind == "kick" && !t.PenaltyUntil.IsZero() && s.actionWait != nil {
		for _, characterID := range t.RemovedIDs {
			if err := s.actionWait.ImposeActionWaitUntil(ctx, characterID, t.PenaltyUntil); err != nil {
				return Transition{}, err
			}
		}
	}
	if err := s.transitions.AcknowledgeTransition(ctx, t.Detail.Room.ID, t.Version); err != nil {
		return Transition{}, err
	}
	return t, nil
}

// RecoverCharacterDeparture completes a pending kick publication and wait deadline
// when the character has an unacknowledged kick transition.
func (s *Service) RecoverCharacterDeparture(ctx context.Context, characterID string) error {
	roomID, err := s.transitions.FindCharacterClaim(ctx, characterID)
	if err != nil || roomID == "" {
		return err
	}
	t, err := s.transitions.FindTransition(ctx, roomID)
	if err != nil || !t.Pending || t.Kind != "kick" || !slices.Contains(t.RemovedIDs, characterID) {
		return err
	}
	return s.repo.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		latest, err := s.transitions.FindTransition(lockedCtx, roomID)
		if err != nil || !latest.Pending {
			return err
		}
		_, err = s.publishTransition(lockedCtx, latest)
		return err
	})
}

// HasPendingKickWait reports whether characterID has an unacknowledged kick wait.
func (s *Service) HasPendingKickWait(ctx context.Context, characterID string) (bool, error) {
	roomID, err := s.transitions.FindCharacterClaim(ctx, characterID)
	if err != nil || roomID == "" {
		return false, err
	}
	t, err := s.transitions.FindTransition(ctx, roomID)
	if err != nil {
		return false, err
	}
	return t.Pending && t.Kind == "kick" && slices.Contains(t.RemovedIDs, characterID), nil
}

// recoverCharacter also finds creation that failed before publishing a reverse mapping,
// and triggers lazy inactivity cleanup when the claimed room's idle timeout has elapsed.
func (s *Service) recoverCharacter(ctx context.Context, characterID string) (Transition, error) {
	roomID, err := s.transitions.FindCharacterClaim(ctx, characterID)
	if err != nil || roomID == "" {
		return Transition{}, err
	}
	var recovered Transition
	err = s.repo.WithRoomLock(ctx, roomID, func(lockedCtx context.Context) error {
		t, err := s.transitions.FindTransition(lockedCtx, roomID)
		if err != nil {
			return err
		}
		if t.Pending {
			published, err := s.publishTransition(lockedCtx, t)
			if err != nil {
				return err
			}
			t = published
			t.Pending = false
			recovered = t
		}
		if t.Version > 0 && t.Detail.Room.Status != StatusDisbanded && hasRoomMember(t.Detail.Members, characterID) && s.isInactive(t.LastActivityAt, s.now()) {
			if _, roomErr := s.repo.GetRoom(lockedCtx, roomID); roomErr != nil && !errors.Is(roomErr, ErrRoomNotFound) {
				return roomErr
			}
			lockedCtx = context.WithValue(lockedCtx, transitionVersionKey{}, t.Version)
			timeoutT, err := s.cleanupInactiveRoom(lockedCtx, t, s.now())
			if err != nil {
				return err
			}
			recovered = timeoutT
		}
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

// CanTransitionFrom checks whether a transition kind is allowed from the prior room status.
func CanTransitionFrom(prevStatus, kind string) bool {
	if prevStatus == StatusDisbanded {
		return false
	}
	if prevStatus == StatusCompleted {
		return kind == "leave" || kind == "disband" || kind == "abandon" || kind == "timeout"
	}
	return true
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
	if t.LastActivityAt.IsZero() {
		t.LastActivityAt = t.Detail.Room.CreatedAt
	}
	t.Detail.Room.FinancialVersion = t.Version
	t.Detail.Room.PasswordHash = t.PasswordHash
	t.Detail.Room.PendingPvPKills = cloneIntMap(t.PendingPvPKills)
	t.Detail.Room.LastActivityAt = t.LastActivityAt
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
	if previous.Version+1 != t.Version || previous.Pending || !CanTransitionFrom(previous.Detail.Room.Status, t.Kind) {
		return ErrTransitionConflict
	}
	last := previous.LastActivityAt
	if last.IsZero() {
		last = previous.Detail.Room.CreatedAt
	}
	if IsActivityTransitionKind(t.Kind) && !last.IsZero() && t.LastActivityAt.After(last.Add(InactivityTimeout)) {
		return ErrRoomInactive
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
	t.Pending = false
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	r.states[roomID] = data
	return nil
}
