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
)

type RescueRecord struct {
	ID             string    `json:"id"`
	CharacterID    string    `json:"character_id"`
	Reason         string    `json:"reason"`
	PenaltySeconds int       `json:"penalty_seconds"`
	CreatedAt      time.Time `json:"created_at"`
}
