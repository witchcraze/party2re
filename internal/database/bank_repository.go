package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type BankRepository struct {
	db *sql.DB
}

func NewBankRepository(db *sql.DB) (*BankRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BankRepository{db: db}, nil
}

func (r *BankRepository) GetCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	executor := ExecutorFromContext(ctx, r.db)
	return scanCharacterRow(executor.QueryRowContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ?
	`, characterID))
}

func (r *BankRepository) Deposit(ctx context.Context, characterID string, amount int64) (corecharacter.Character, error) {
	var char corecharacter.Character
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		c, err := scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
			FOR UPDATE
		`, characterID))
		if err != nil {
			return err
		}

		newMoney, newDeposit, err := bank.CalculateDeposit(c.Money, c.Deposit, amount)
		if err != nil {
			return err
		}

		_, err = executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = ?, deposit = ?
			WHERE id = ?
		`, newMoney, newDeposit, characterID)
		if err != nil {
			return err
		}

		c.Money = newMoney
		c.Deposit = newDeposit
		char = c
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return char, nil
}

func (r *BankRepository) Withdraw(ctx context.Context, characterID string, amount int64) (corecharacter.Character, int, int64, error) {
	var (
		char            corecharacter.Character
		actualWithdrawn int
		refunded        int64
	)
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		c, err := scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
			FOR UPDATE
		`, characterID))
		if err != nil {
			return err
		}

		newMoney, newDeposit, actual, ref, err := bank.CalculateWithdrawal(c.Money, c.Deposit, amount)
		if err != nil {
			return err
		}

		_, err = executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = ?, deposit = ?
			WHERE id = ?
		`, newMoney, newDeposit, characterID)
		if err != nil {
			return err
		}

		c.Money = newMoney
		c.Deposit = newDeposit
		char = c
		actualWithdrawn = actual
		refunded = ref
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, 0, 0, err
	}
	return char, actualWithdrawn, refunded, nil
}
