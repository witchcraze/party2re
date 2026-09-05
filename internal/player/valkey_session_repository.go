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

// MemorySessionCount returns the number of active/stored sessions in the in-memory fallback store.
// Intended for diagnostics and automated leak tests.
func (r *ValkeySessionRepository) MemorySessionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.memorySessions)
}

// purgeExpiredMemory removes expired sessions from the in-memory fallback store.
// Caller MUST hold r.mu.Lock().
func (r *ValkeySessionRepository) purgeExpiredMemory(now time.Time) {
	for id, sess := range r.memorySessions {
		if !sess.Active(now) {
			delete(r.memorySessions, id)
		}
	}
}

// purgeExpiredZSet lazily purges expired session tokens from the player's Sorted Set index.
func (r *ValkeySessionRepository) purgeExpiredZSet(ctx context.Context, playerKey string, now time.Time) {
	if r.client == nil || strings.TrimSpace(playerKey) == "" {
		return
	}
	cmd := r.client.B().Zremrangebyscore().Key(playerKey).Min("-inf").Max(fmt.Sprintf("%d", now.Unix())).Build()
	_ = r.client.Do(ctx, cmd).Error()
}

// Save persists a player session with native TTL in Valkey and tracks the session ID in the player's session sorted set.
func (r *ValkeySessionRepository) Save(ctx context.Context, session coreplayer.Session) error {
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.PlayerID) == "" {
		return coreplayer.ErrInvalidSession
	}

	now := time.Now().UTC()
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		ttl = SessionDuration
	}
	ttlSeconds := int64(ttl.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 1
	}

	// 1. If live Valkey client is available, master exclusively in Valkey Master.
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

		score := float64(session.ExpiresAt.Unix())
		zaddCmd := r.client.B().Zadd().Key(playerKey).ScoreMember().ScoreMember(score, session.ID).Build()
		if err := r.client.Do(ctx, zaddCmd).Error(); err != nil && strings.Contains(err.Error(), "WRONGTYPE") {
			// Upgrade legacy Set key to Sorted Set
			_ = r.client.Do(ctx, r.client.B().Del().Key(playerKey).Build()).Error()
			_ = r.client.Do(ctx, r.client.B().Zadd().Key(playerKey).ScoreMember().ScoreMember(score, session.ID).Build()).Error()
		}

		// Lazily purge expired sessions from player's ZSET
		r.purgeExpiredZSet(ctx, playerKey, now)

		expireCmd := r.client.B().Expire().Key(playerKey).Seconds(ttlSeconds).Build()
		_ = r.client.Do(ctx, expireCmd).Error()

		return nil
	}

	// 2. In-memory store fallback only when client is nil
	r.mu.Lock()
	r.purgeExpiredMemory(now)
	r.memorySessions[session.ID] = session
	r.mu.Unlock()

	return nil
}

// FindByID retrieves a session by its token/ID from Valkey Master or in-memory fallback.
func (r *ValkeySessionRepository) FindByID(ctx context.Context, id string) (coreplayer.Session, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return coreplayer.Session{}, coreplayer.ErrInvalidSession
	}

	now := time.Now().UTC()

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
			if ok && sess.Active(now) {
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
			sess = coreplayer.Session{
				ID:        id,
				PlayerID:  trimmed,
				CreatedAt: now,
				ExpiresAt: now.Add(SessionDuration),
			}
		}

		if !sess.Active(now) {
			return coreplayer.Session{}, coreplayer.ErrInvalidSession
		}

		// Lazily purge expired sessions from player's ZSET
		if sess.PlayerID != "" {
			r.purgeExpiredZSet(ctx, r.playerSessionsKey(sess.PlayerID), now)
		}

		return sess, nil
	}

	// In-memory fallback
	r.mu.Lock()
	r.purgeExpiredMemory(now)
	sess, ok := r.memorySessions[id]
	r.mu.Unlock()

	if !ok || !sess.Active(now) {
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
			var playerID string
			if strings.HasPrefix(trimmed, "{") {
				if json.Unmarshal([]byte(trimmed), &sess) == nil && sess.PlayerID != "" {
					playerID = sess.PlayerID
				}
			} else if trimmed != "" {
				playerID = trimmed
			}

			if playerID != "" {
				playerKey := r.playerSessionsKey(playerID)
				_ = r.client.Do(ctx, r.client.B().Zrem().Key(playerKey).Member(id).Build()).Error()
				r.purgeExpiredZSet(ctx, playerKey, now)
			}
		}
	}

	r.mu.Lock()
	sess, foundInMemory := r.memorySessions[id]
	if foundInMemory {
		delete(r.memorySessions, id)
	}
	r.purgeExpiredMemory(now)
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
		members, err := r.client.Do(ctx, r.client.B().Zrange().Key(playerKey).Min("0").Max("-1").Build()).AsStrSlice()
		if err != nil && strings.Contains(err.Error(), "WRONGTYPE") {
			// Fallback in case old Set key exists
			members, err = r.client.Do(ctx, r.client.B().Smembers().Key(playerKey).Build()).AsStrSlice()
		}
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
	now := time.Now().UTC()
	for id, sess := range r.memorySessions {
		if sess.PlayerID == playerID || !sess.Active(now) {
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
