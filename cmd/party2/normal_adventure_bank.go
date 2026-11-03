package main

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
)

// Bank owns wallet/savings calculations; composition adds Adventure admission
// in the same SQL transaction before delegating to the existing repository.
type normalBankRepository struct {
	*database.BankRepository
	writer normalCharacterWriter
}

func (r normalBankRepository) Deposit(ctx context.Context, actor string, amount int64) (c corecharacter.Character, err error) {
	err = r.writer.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := r.writer.FindByIDForUpdate(ctx, actor); err != nil {
			return err
		}
		c, err = r.BankRepository.Deposit(ctx, actor, amount)
		return err
	})
	return
}

func (r normalBankRepository) Withdraw(ctx context.Context, actor string, amount int64) (c corecharacter.Character, actual int, refunded int64, err error) {
	err = r.writer.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := r.writer.FindByIDForUpdate(ctx, actor); err != nil {
			return err
		}
		c, actual, refunded, err = r.BankRepository.Withdraw(ctx, actor, amount)
		return err
	})
	return
}
