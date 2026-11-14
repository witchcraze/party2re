package character

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// MaxMemories is the number of newest trajectory entries shown (legacy $max_log).
const MaxMemories = 999

// Memory is one trajectory entry with the character's state when it was written.
type Memory struct {
	Kind      corecharacter.MemoryKind
	Content   string
	Name      string
	GuildName string
	Color     string
	Level     int
	JobID     string
	Gender    string
	SP        int
	CreatedAt time.Time
}

// MemoryRepository stores trajectory entries.
type MemoryRepository interface {
	AppendMemory(ctx context.Context, characterID string, kind corecharacter.MemoryKind, content string) error
	// ListMemories returns up to limit entries, newest first.
	ListMemories(ctx context.Context, characterID string, limit int) ([]Memory, error)
}

// WithMemoryRepository enables trajectory recording and reading.
func WithMemoryRepository(repo MemoryRepository) Option {
	return func(s *Service) {
		s.memories = repo
	}
}

func (s *Service) recordMemory(ctx context.Context, characterID string, kind corecharacter.MemoryKind, content string) error {
	if s.memories == nil {
		return nil
	}
	return s.memories.AppendMemory(ctx, characterID, kind, content)
}

// Trajectory returns the character's newest milestones, newest first.
func (s *Service) Trajectory(ctx context.Context, characterID string) ([]Memory, error) {
	if s.memories == nil {
		return nil, errors.New("memory repository is not configured")
	}
	if _, err := s.repository.FindByID(ctx, characterID); err != nil {
		return nil, err
	}
	return s.memories.ListMemories(ctx, characterID, MaxMemories)
}
