package main

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
)

// Only services changing the actor's live combat state use this writer.
// Peer deliveries keep the ordinary repository so recipients can receive assets.
type normalCharacterWriter struct {
	*database.CharacterRepository
	check func(context.Context, string) error
	tx    economy.TransactionProvider
}

func normalWriter(core *coreServices) normalCharacterWriter {
	return normalCharacterWriter{CharacterRepository: core.charRepo, tx: core.txProvider, check: func(ctx context.Context, actor string) error {
		if core.normalGuard != nil {
			return core.normalGuard(ctx, actor)
		}
		return nil
	}}
}

func (r normalCharacterWriter) FindByIDForUpdate(ctx context.Context, actor string) (corecharacter.Character, error) {
	c, err := r.CharacterRepository.FindByIDForUpdate(ctx, actor)
	if err != nil {
		return c, err
	}
	return c, r.check(ctx, actor)
}

func (r normalCharacterWriter) Delete(ctx context.Context, actor string) error {
	return r.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := r.FindByIDForUpdate(ctx, actor); err != nil && !errors.Is(err, corecharacter.ErrNotFound) {
			return err
		}
		return r.CharacterRepository.Delete(ctx, actor)
	})
}
