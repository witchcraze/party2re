package player

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

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
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	if err != nil {
		return false, ErrAuthentication
	}
	return true, nil
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
