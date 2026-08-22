package player

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const passwordIterations = 100000

var (
	ErrInvalidPlayer   = errors.New("player is invalid")
	ErrInvalidPassword = errors.New("password is invalid")
	ErrInvalidSession  = errors.New("session is invalid")
	ErrAuthentication  = errors.New("authentication failed")
)

type Player struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	PlayerID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func New(username, password string, now time.Time) (Player, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return Player{}, ErrInvalidPlayer
	}
	if password == "" {
		return Player{}, ErrInvalidPassword
	}
	hash, err := hashPassword(password)
	if err != nil {
		return Player{}, err
	}
	id, err := randomID()
	if err != nil {
		return Player{}, err
	}
	return Player{ID: id, Username: username, PasswordHash: hash, CreatedAt: now.UTC()}, nil
}

func (p Player) Authenticate(password string) bool {
	if password == "" || p.PasswordHash == "" {
		return false
	}
	hash, err := verifyPassword(password, p.PasswordHash)
	return err == nil && hash
}

func NewSession(playerID string, now time.Time, duration time.Duration) (Session, error) {
	if playerID == "" || duration <= 0 {
		return Session{}, ErrInvalidSession
	}
	id, err := randomID()
	if err != nil {
		return Session{}, err
	}
	now = now.UTC()
	return Session{ID: id, PlayerID: playerID, CreatedAt: now, ExpiresAt: now.Add(duration)}, nil
}

func (s Session) Active(now time.Time) bool {
	return s.ID != "" && s.PlayerID != "" && s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	digest := derive(password, salt)
	return fmt.Sprintf("sha256$%d$%s$%s", passwordIterations, hex.EncodeToString(salt), hex.EncodeToString(digest)), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "sha256" {
		return false, ErrAuthentication
	}
	var iterations int
	if _, err := fmt.Sscanf(parts[1], "%d", &iterations); err != nil || iterations != passwordIterations {
		return false, ErrAuthentication
	}
	saltText, digestText := parts[2], parts[3]
	salt, err := hex.DecodeString(saltText)
	if err != nil {
		return false, ErrAuthentication
	}
	expected, err := hex.DecodeString(digestText)
	if err != nil {
		return false, ErrAuthentication
	}
	actual := derive(password, salt)
	return len(expected) == len(actual) && subtle.ConstantTimeCompare(expected, actual) == 1, nil
}

func derive(password string, salt []byte) []byte {
	value := append(append([]byte(nil), salt...), []byte(password)...)
	for i := 0; i < passwordIterations; i++ {
		sum := sha256.Sum256(value)
		value = sum[:]
	}
	return value
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
