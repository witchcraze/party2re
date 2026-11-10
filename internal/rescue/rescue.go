package rescue

import (
	"errors"
	"time"
)

const (
	DefaultPenaltySeconds = 600 // 10 minutes sleep penalty
)

var (
	ErrInvalidCharacterID = errors.New("invalid character ID")
	ErrInvalidReason      = errors.New("rescue reason cannot be empty")
	ErrNoRescueRecord     = errors.New("no rescue record found")
	ErrActivityRemaining  = errors.New("character still has unrecovered feature activity")
)

type RescueRecord struct {
	ID             string    `json:"id"`
	CharacterID    string    `json:"character_id"`
	Reason         string    `json:"reason"`
	PenaltySeconds int       `json:"penalty_seconds"`
	CreatedAt      time.Time `json:"created_at"`
	// SleepUntil is the owed sleep deadline: prior remaining sleep plus the penalty.
	SleepUntil time.Time `json:"-"`
	// AppliedAt is zero until scheduled work is cleared and SleepUntil reaches the sleep timer.
	AppliedAt time.Time `json:"-"`
}
