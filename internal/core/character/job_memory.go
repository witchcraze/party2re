package character

import (
	"errors"
	"strings"
	"time"
)

// JobMemoryKind defines the persistence lifecycle of a JobMemory.
type JobMemoryKind string

const (
	// JobMemoryKindPersistent persists across sleep until manually reverted (item-168 記憶のカケラ).
	JobMemoryKindPersistent JobMemoryKind = "persistent"
	// JobMemoryKindTemporary is sleep-scoped and automatically reverts upon resting (item-243 陽炎の追憶記).
	JobMemoryKindTemporary JobMemoryKind = "temporary"
)

// JobMemory is the pair of job states used by the job exchange operation.
// It tracks its lifecycle (Kind) so temporary memory can be reverted on sleep
// while persistent memory is retained until manually released.
type JobMemory struct {
	Kind     JobMemoryKind `json:"kind"`
	JobID    string        `json:"job_id"`
	SP       int           `json:"sp"`
	OldJobID string        `json:"old_job_id"`
	OldSP    int           `json:"old_sp"`
}

// IsTemporary reports whether this JobMemory is sleep-scoped (item-243).
func (m *JobMemory) IsTemporary() bool {
	return m != nil && m.Kind == JobMemoryKindTemporary
}

// IsPersistent reports whether this JobMemory persists across sleep (item-168).
// Empty or unspecified Kind defaults to persistent for backward compatibility.
func (m *JobMemory) IsPersistent() bool {
	return m != nil && m.Kind != JobMemoryKindTemporary
}

// FutureMemory represents a saved snapshot of a character's state created via
// item-207 (未来のカケラ) and restored through the legacy "よびおこす" action.
type FutureMemory struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	JobID       string    `json:"job_id"`
	OldJobID    string    `json:"old_job_id"`
	Level       int       `json:"level"`
	Experience  int       `json:"experience"`
	MaxHP       int       `json:"max_hp"`
	MaxMP       int       `json:"max_mp"`
	Attack      int       `json:"attack"`
	Defense     int       `json:"defense"`
	Agility     int       `json:"agility"`
	Gender      string    `json:"gender"`
	OverLevel   bool      `json:"over_level"`
	CreatedAt   time.Time `json:"created_at"`
}

// ApplyJobMemory switches to a remembered job pair without applying the
// level/stat penalty used by a normal job change.
func (c *Character) ApplyJobMemory(jobID string, sp int, oldJobID string, oldSP int) error {
	if c == nil || strings.TrimSpace(jobID) == "" || strings.TrimSpace(oldJobID) == "" ||
		sp < 0 || oldSP < 0 {
		return errors.New("job memory is invalid")
	}
	c.JobID, c.SP = strings.TrimSpace(jobID), sp
	c.OldJobID, c.OldSP = strings.TrimSpace(oldJobID), oldSP
	return nil
}

// CanSaveFutureMemory checks whether a future memory snapshot can be saved.
// It requires that no temporary or persistent job exchange is active, and that existing snapshots
// do not exceed the OverFuture capacity limit (0 allows 1 snapshot).
func (c *Character) CanSaveFutureMemory(currentSnapshotCount int) bool {
	if c == nil || c.JobMemory != nil {
		return false
	}
	return currentSnapshotCount <= c.OverFuture
}

// CreateFutureMemory builds a future memory snapshot from current character state.
func (c *Character) CreateFutureMemory(id string, createdAt time.Time) FutureMemory {
	return FutureMemory{
		ID:          id,
		CharacterID: c.ID,
		JobID:       c.JobID,
		OldJobID:    c.OldJobID,
		Level:       c.Level,
		Experience:  c.Experience,
		MaxHP:       c.Stats.MaxHP,
		MaxMP:       c.Stats.MaxMP,
		Attack:      c.Stats.Attack,
		Defense:     c.Stats.Defense,
		Agility:     c.Stats.Agility,
		Gender:      c.Gender,
		OverLevel:   c.OverLevel,
		CreatedAt:   createdAt,
	}
}

// ApplyFutureMemory restores the character's state from a future memory snapshot
// and resets current HP/MP to their restored maxima, setting SP and OldSP from mastery data.
func (c *Character) ApplyFutureMemory(memory FutureMemory, currentSP, oldSP int) error {
	if c == nil || strings.TrimSpace(memory.JobID) == "" {
		return errors.New("future memory is invalid")
	}
	if currentSP < 0 || oldSP < 0 {
		return ErrInvalidAmount
	}
	c.JobID = memory.JobID
	c.OldJobID = memory.OldJobID
	c.Level = memory.Level
	c.Experience = memory.Experience
	c.Stats.MaxHP = memory.MaxHP
	c.Stats.HP = memory.MaxHP
	c.Stats.MaxMP = memory.MaxMP
	c.Stats.MP = memory.MaxMP
	c.Stats.Attack = memory.Attack
	c.Stats.Defense = memory.Defense
	c.Stats.Agility = memory.Agility
	c.Gender = memory.Gender
	c.OverLevel = memory.OverLevel
	c.SP = currentSP
	c.OldSP = oldSP
	return nil
}

// RevertJobMemory restores character's original job and SP from JobMemory if present.
func (c *Character) RevertJobMemory() bool {
	if c == nil || c.JobMemory == nil {
		return false
	}
	memory := *c.JobMemory
	_ = c.ApplyJobMemory(memory.JobID, memory.SP, memory.OldJobID, memory.OldSP)
	c.JobMemory = nil
	return true
}
