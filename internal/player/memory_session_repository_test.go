package player_test

import (
	"context"
	"strings"
	"sync"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type memorySessionRepository struct {
	mu             sync.Mutex
	memorySessions map[string]coreplayer.Session
}

func newMemorySessionRepository() *memorySessionRepository {
	return &memorySessionRepository{memorySessions: make(map[string]coreplayer.Session)}
}

func (r *memorySessionRepository) purgeExpiredMemory(now time.Time) {
	for id, sess := range r.memorySessions {
		if !sess.Active(now) {
			delete(r.memorySessions, id)
		}
	}
}

func (r *memorySessionRepository) Save(_ context.Context, session coreplayer.Session) error {
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.PlayerID) == "" {
		return coreplayer.ErrInvalidSession
	}

	now := time.Now().UTC()

	r.mu.Lock()
	r.purgeExpiredMemory(now)
	r.memorySessions[session.ID] = session
	r.mu.Unlock()

	return nil
}

func (r *memorySessionRepository) FindByID(_ context.Context, id string) (coreplayer.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return coreplayer.Session{}, coreplayer.ErrInvalidSession
	}

	now := time.Now().UTC()

	r.mu.Lock()
	r.purgeExpiredMemory(now)
	sess, ok := r.memorySessions[id]
	r.mu.Unlock()

	if !ok || !sess.Active(now) {
		return coreplayer.Session{}, coreplayer.ErrInvalidSession
	}
	return sess, nil
}

func (r *memorySessionRepository) Revoke(_ context.Context, id string, now time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return coreplayer.ErrInvalidSession
	}

	r.mu.Lock()
	sess, foundInMemory := r.memorySessions[id]
	if foundInMemory {
		delete(r.memorySessions, id)
	}
	r.purgeExpiredMemory(now)
	r.mu.Unlock()

	if !foundInMemory || !sess.Active(now) {
		return coreplayer.ErrInvalidSession
	}

	return nil
}

func (r *memorySessionRepository) DeleteByPlayerID(_ context.Context, playerID string) error {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil
	}

	r.mu.Lock()
	now := time.Now().UTC()
	for id, sess := range r.memorySessions {
		if sess.PlayerID == playerID || !sess.Active(now) {
			delete(r.memorySessions, id)
		}
	}
	r.mu.Unlock()

	return nil
}

func (r *memorySessionRepository) sessionCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.memorySessions)
}
