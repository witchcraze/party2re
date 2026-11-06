package main

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
)

// Single-actor mutation services use this writer. Mixed actor/recipient services
// check the initiating actor explicitly and keep passive deliveries unguarded.
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

// LockCharacterRecipientByIDForUpdate locks a passive referral recipient. It is not the initiating
// actor, so the normal-run guard does not apply.
func (r normalCharacterWriter) LockCharacterRecipientByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.CharacterRepository.FindByIDForUpdate(ctx, id)
}

func (r normalCharacterWriter) Delete(ctx context.Context, actor string) error {
	return r.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := r.FindByIDForUpdate(ctx, actor); err != nil && !errors.Is(err, corecharacter.ErrNotFound) {
			return err
		}
		return r.CharacterRepository.Delete(ctx, actor)
	})
}
