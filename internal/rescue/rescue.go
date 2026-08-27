package rescue

import (
	"errors"
	"time"
)

const (
	DefaultPenaltySeconds = 600 // 10 minutes sleep penalty
)

var (
	ErrInvalidCharacterID    = errors.New("invalid character ID")
	ErrInvalidReason         = errors.New("rescue reason cannot be empty")
	ErrCharacterUnderPenalty = errors.New("character is currently under rescue penalty cooldown")
	ErrNoRescueRecord        = errors.New("no rescue record found")
)

type RescueRecord struct {
	ID             string    `json:"id"`
	CharacterID    string    `json:"character_id"`
	Reason         string    `json:"reason"`
	PenaltySeconds int       `json:"penalty_seconds"`
	CreatedAt      time.Time `json:"created_at"`
}

// ExpiresAt returns the timestamp when the rescue penalty expires.
func (r RescueRecord) ExpiresAt() time.Time {
	return r.CreatedAt.Add(time.Duration(r.PenaltySeconds) * time.Second)
}

// IsActive returns true if the rescue penalty is still active at the given time.
func (r RescueRecord) IsActive(now time.Time) bool {
	return now.Before(r.ExpiresAt())
}
