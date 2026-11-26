package home

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

const futureMemoryName = "未来のカケラ"

// FutureMemorySaver stores a future memory snapshot for a character the caller
// holds locked in its transaction (item-207 未来のカケラ).
type FutureMemorySaver interface {
	SaveFutureMemorySnapshot(ctx context.Context, char corecharacter.Character) (corecharacter.FutureMemory, error)
}

// SetFutureMemorySaver registers the cross-domain future memory saver.
func (s *Service) SetFutureMemorySaver(f FutureMemorySaver) { s.futureMemory = f }

// applyFutureMemoryConsumable reports the legacy refusal (_data.cgi:1843,1871)
// as a no-effect outcome so the caller rolls back and keeps the item.
func (s *Service) applyFutureMemoryConsumable(ctx context.Context, char corecharacter.Character) (HomeItemOutcome, error) {
	if s.futureMemory == nil {
		return HomeItemOutcome{}, ItemRefusal{ItemName: futureMemoryName}
	}
	_, err := s.futureMemory.SaveFutureMemorySnapshot(ctx, char)
	if errors.Is(err, corejob.ErrJobUnavailable) || errors.Is(err, corecharacter.ErrFutureMemorySlotLimit) {
		return HomeItemOutcome{Effect: HomeItemEffectFutureMemoryFull}, errNoEffect
	}
	if err != nil {
		return HomeItemOutcome{}, err
	}
	return HomeItemOutcome{Effect: HomeItemEffectFutureMemory}, nil
}
