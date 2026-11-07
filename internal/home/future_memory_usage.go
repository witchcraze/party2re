package home

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

// FutureMemorySaver stores a future memory snapshot for a character the caller
// holds locked in its transaction (item-207 未来のカケラ).
type FutureMemorySaver interface {
	SaveFutureMemorySnapshot(ctx context.Context, char corecharacter.Character) (corecharacter.FutureMemory, error)
}

// SetFutureMemorySaver registers the cross-domain future memory saver.
func (s *Service) SetFutureMemorySaver(f FutureMemorySaver) { s.futureMemory = f }

// applyFutureMemoryConsumable maps the legacy rejection (_data.cgi:1828–1873)
// to ErrCannotUseHere so the caller rolls back and keeps the item.
func (s *Service) applyFutureMemoryConsumable(ctx context.Context, char corecharacter.Character) (string, error) {
	if s.futureMemory == nil {
		return "", fmt.Errorf("%w: 未来のカケラはここでは使えません", ErrCannotUseHere)
	}
	_, err := s.futureMemory.SaveFutureMemorySnapshot(ctx, char)
	if errors.Is(err, corejob.ErrJobUnavailable) || errors.Is(err, corecharacter.ErrFutureMemorySlotLimit) {
		return "", fmt.Errorf("%w: これ以上職業を覚えられない", ErrCannotUseHere)
	}
	if err != nil {
		return "", err
	}
	return "未来のカケラに記憶が写った！", nil
}
