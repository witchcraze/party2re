package player

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

const (
	// DefaultSessionKeyPrefix is the default Valkey key prefix for player sessions.
	DefaultSessionKeyPrefix = "party2:session:"

	// DefaultPlayerSessionsKeyPrefix is the default Valkey key prefix for tracking player session IDs in a set.
	DefaultPlayerSessionsKeyPrefix = "party2:player:sessions:"
)

// ValkeySessionOption defines configuration options for ValkeySessionRepository.
type ValkeySessionOption func(*ValkeySessionRepository)

// WithSessionKeyPrefix overrides the default session key prefix.
func WithSessionKeyPrefix(prefix string) ValkeySessionOption {
	return func(r *ValkeySessionRepository) {
		if prefix != "" {
			r.sessionPrefix = prefix
		}
	}
}

// WithPlayerSessionsKeyPrefix overrides the default player session set key prefix.
func WithPlayerSessionsKeyPrefix(prefix string) ValkeySessionOption {
	return func(r *ValkeySessionRepository) {
		if prefix != "" {
			r.playerPrefix = prefix
		}
	}
}

// ValkeySessionRepository implements SessionRepository by mastering ephemeral player sessions
// in Valkey Master with native TTL, eliminating MariaDB connection contention on authenticated requests.
// When client is nil (or in environments without Valkey), it operates via a thread-safe in-memory session store.
type ValkeySessionRepository struct {
	client        valkey.Client
	sessionPrefix string
	playerPrefix  string

	mu             sync.RWMutex
	memorySessions map[string]coreplayer.Session
}

// NewValkeySessionRepository constructs a new ValkeySessionRepository.
func NewValkeySessionRepository(client valkey.Client, opts ...ValkeySessionOption) *ValkeySessionRepository {
	r := &ValkeySessionRepository{
		client:         client,
		sessionPrefix:  DefaultSessionKeyPrefix,
		playerPrefix:   DefaultPlayerSessionsKeyPrefix,
		memorySessions: make(map[string]coreplayer.Session),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	return r
}

// Save persists a player session with native TTL in Valkey and tracks the session ID in the player's session set.
func (r *ValkeySessionRepository) Save(ctx context.Context, session coreplayer.Session) error {
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.PlayerID) == "" {
		return coreplayer.ErrInvalidSession
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		ttl = SessionDuration
	}
	ttlSeconds := int64(ttl.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 1
	}

	// 1. In-memory store for fallback/local execution
	r.mu.Lock()
	r.memorySessions[session.ID] = session
	r.mu.Unlock()

	// 2. Persist to Valkey if client is available
	if r.client != nil {
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("marshal session: %w", err)
		}

		sessionKey := r.sessionKey(session.ID)
		playerKey := r.playerSessionsKey(session.PlayerID)

		setCmd := r.client.B().Set().Key(sessionKey).Value(string(data)).ExSeconds(ttlSeconds).Build()
		if err := r.client.Do(ctx, setCmd).Error(); err != nil {
			return fmt.Errorf("save session to valkey: %w", err)
		}

		saddCmd := r.client.B().Sadd().Key(playerKey).Member(session.ID).Build()
		_ = r.client.Do(ctx, saddCmd).Error()
		expireCmd := r.client.B().Expire().Key(playerKey).Seconds(ttlSeconds).Build()
		_ = r.client.Do(ctx, expireCmd).Error()
	}

	return nil
}

// FindByID retrieves a session by its token/ID from Valkey Master or in-memory fallback.
func (r *ValkeySessionRepository) FindByID(ctx context.Context, id string) (coreplayer.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return coreplayer.Session{}, coreplayer.ErrInvalidSession
	}

	if r.client != nil {
		sessionKey := r.sessionKey(id)
		cmd := r.client.B().Get().Key(sessionKey).Build()
		val, err := r.client.Do(ctx, cmd).ToString()
		if err != nil {
			if valkey.IsValkeyNil(err) {
				return coreplayer.Session{}, coreplayer.ErrInvalidSession
			}
			// Transient Valkey error: check memory store fallback if populated
			r.mu.RLock()
			sess, ok := r.memorySessions[id]
			r.mu.RUnlock()
			if ok && sess.Active(time.Now().UTC()) {
				return sess, nil
			}
			return coreplayer.Session{}, coreplayer.ErrInvalidSession
		}

		var sess coreplayer.Session
		trimmed := strings.TrimSpace(val)
		if strings.HasPrefix(trimmed, "{") {
			if err := json.Unmarshal([]byte(trimmed), &sess); err != nil {
				return coreplayer.Session{}, coreplayer.ErrInvalidSession
			}
		} else {
			// Direct string token -> player_id mapping
			now := time.Now().UTC()
			sess = coreplayer.Session{
				ID:        id,
				PlayerID:  trimmed,
				CreatedAt: now,
				ExpiresAt: now.Add(SessionDuration),
			}
		}

		if !sess.Active(time.Now().UTC()) {
			return coreplayer.Session{}, coreplayer.ErrInvalidSession
		}
		return sess, nil
	}

	// In-memory fallback
	r.mu.RLock()
	sess, ok := r.memorySessions[id]
	r.mu.RUnlock()
	if !ok || !sess.Active(time.Now().UTC()) {
		return coreplayer.Session{}, coreplayer.ErrInvalidSession
	}
	return sess, nil
}

// Revoke revokes and removes a session from Valkey Master and in-memory fallback.
func (r *ValkeySessionRepository) Revoke(ctx context.Context, id string, now time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return coreplayer.ErrInvalidSession
	}

	var foundInValkey bool
	if r.client != nil {
		sessionKey := r.sessionKey(id)
		cmd := r.client.B().Getdel().Key(sessionKey).Build()
		val, err := r.client.Do(ctx, cmd).ToString()
		if err == nil {
			foundInValkey = true
			trimmed := strings.TrimSpace(val)
			var sess coreplayer.Session
			if strings.HasPrefix(trimmed, "{") {
				if json.Unmarshal([]byte(trimmed), &sess) == nil && sess.PlayerID != "" {
					playerKey := r.playerSessionsKey(sess.PlayerID)
					_ = r.client.Do(ctx, r.client.B().Srem().Key(playerKey).Member(id).Build()).Error()
				}
			} else if trimmed != "" {
				playerKey := r.playerSessionsKey(trimmed)
				_ = r.client.Do(ctx, r.client.B().Srem().Key(playerKey).Member(id).Build()).Error()
			}
		}
	}

	r.mu.Lock()
	sess, foundInMemory := r.memorySessions[id]
	if foundInMemory {
		delete(r.memorySessions, id)
	}
	r.mu.Unlock()

	if !foundInValkey && (!foundInMemory || !sess.Active(now)) {
		return coreplayer.ErrInvalidSession
	}

	return nil
}

// DeleteByPlayerID deletes all active sessions associated with the specified player from Valkey Master and in-memory fallback.
func (r *ValkeySessionRepository) DeleteByPlayerID(ctx context.Context, playerID string) error {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil
	}

	if r.client != nil {
		playerKey := r.playerSessionsKey(playerID)
		members, err := r.client.Do(ctx, r.client.B().Smembers().Key(playerKey).Build()).AsStrSlice()
		if err == nil && len(members) > 0 {
			keys := make([]string, 0, len(members)+1)
			for _, m := range members {
				keys = append(keys, r.sessionKey(m))
			}
			keys = append(keys, playerKey)
			_ = r.client.Do(ctx, r.client.B().Del().Key(keys...).Build()).Error()
		} else {
			_ = r.client.Do(ctx, r.client.B().Del().Key(playerKey).Build()).Error()
		}
	}

	r.mu.Lock()
	for id, sess := range r.memorySessions {
		if sess.PlayerID == playerID {
			delete(r.memorySessions, id)
		}
	}
	r.mu.Unlock()

	return nil
}

func (r *ValkeySessionRepository) sessionKey(id string) string {
	return r.sessionPrefix + id
}

func (r *ValkeySessionRepository) playerSessionsKey(playerID string) string {
	return r.playerPrefix + playerID
}
