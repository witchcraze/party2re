package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

func (r *BankRepository) GetAccount(ctx context.Context, playerID string) (bank.Account, error) {
	var acc bank.Account
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT player_id, balance, updated_at
		FROM bank_accounts
		WHERE player_id = ?
	`, playerID).Scan(&acc.PlayerID, &acc.Balance, &acc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return bank.Account{
			PlayerID:  playerID,
			Balance:   0,
			UpdatedAt: time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return bank.Account{}, err
	}
	return acc, nil
}

func (r *BankRepository) Deposit(ctx context.Context, playerID string, characterID string, amount int) (bank.Account, corecharacter.Character, error) {
	var acc bank.Account
	var char corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		res, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money - ?
			WHERE id = ? AND money >= ?
		`, amount, characterID, amount)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			var exists int
			_ = executor.QueryRowContext(txCtx, "SELECT COUNT(1) FROM characters WHERE id = ?", characterID).Scan(&exists)
			if exists == 0 {
				return corecharacter.ErrNotFound
			}
			return bank.ErrInsufficientFunds
		}

		_, err = executor.ExecContext(txCtx, `
			INSERT INTO bank_accounts (player_id, balance)
			VALUES (?, ?)
			ON DUPLICATE KEY UPDATE balance = balance + VALUES(balance)
		`, playerID, int64(amount))
		if err != nil {
			return err
		}

		err = executor.QueryRowContext(txCtx, `
			SELECT player_id, balance, updated_at
			FROM bank_accounts
			WHERE player_id = ?
		`, playerID).Scan(&acc.PlayerID, &acc.Balance, &acc.UpdatedAt)
		if err != nil {
			return err
		}

		char, err = scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, characterID))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	return acc, char, nil
}

func (r *BankRepository) Withdraw(ctx context.Context, playerID string, characterID string, amount int) (bank.Account, corecharacter.Character, error) {
	var acc bank.Account
	var char corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		res, err := executor.ExecContext(txCtx, `
			UPDATE bank_accounts
			SET balance = balance - ?
			WHERE player_id = ? AND balance >= ?
		`, int64(amount), playerID, int64(amount))
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return bank.ErrInsufficientBalance
		}

		resChar, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money + ?
			WHERE id = ?
		`, amount, characterID)
		if err != nil {
			return err
		}
		affectedChar, err := resChar.RowsAffected()
		if err != nil {
			return err
		}
		if affectedChar == 0 {
			return corecharacter.ErrNotFound
		}

		err = executor.QueryRowContext(txCtx, `
			SELECT player_id, balance, updated_at
			FROM bank_accounts
			WHERE player_id = ?
		`, playerID).Scan(&acc.PlayerID, &acc.Balance, &acc.UpdatedAt)
		if err != nil {
			return err
		}

		char, err = scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, characterID))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	return acc, char, nil
}

func (r *BankRepository) Transfer(ctx context.Context, record bank.TransferRecord) (bank.Account, bank.Account, error) {
	var fromAcc, toAcc bank.Account

	var toPlayerExists int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, "SELECT COUNT(1) FROM players WHERE id = ?", record.ToPlayerID).Scan(&toPlayerExists)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}
	if toPlayerExists == 0 {
		return bank.Account{}, bank.Account{}, bank.ErrAccountNotFound
	}

	p1, p2 := record.FromPlayerID, record.ToPlayerID
	if p1 > p2 {
		p1, p2 = p2, p1
	}

	// Ensure accounts exist to avoid insert gap locks
	_, _ = ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT IGNORE INTO bank_accounts (player_id, balance)
		VALUES (?, 0), (?, 0)
	`, p1, p2)

	err = RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		var bal1, bal2 int64
		err := executor.QueryRowContext(txCtx, "SELECT balance FROM bank_accounts WHERE player_id = ? FOR UPDATE", p1).Scan(&bal1)
		if err != nil {
			return err
		}
		err = executor.QueryRowContext(txCtx, "SELECT balance FROM bank_accounts WHERE player_id = ? FOR UPDATE", p2).Scan(&bal2)
		if err != nil {
			return err
		}

		var fromBalance, toBalance int64
		if record.FromPlayerID == p1 {
			fromBalance, toBalance = bal1, bal2
		} else {
			fromBalance, toBalance = bal2, bal1
		}

		if fromBalance < record.Amount {
			return bank.ErrInsufficientBalance
		}

		newFromBalance := fromBalance - record.Amount
		_, err = executor.ExecContext(txCtx, `
			UPDATE bank_accounts
			SET balance = ?
			WHERE player_id = ?
		`, newFromBalance, record.FromPlayerID)
		if err != nil {
			return err
		}

		newToBalance := toBalance + record.Amount
		_, err = executor.ExecContext(txCtx, `
			UPDATE bank_accounts
			SET balance = ?
			WHERE player_id = ?
		`, newToBalance, record.ToPlayerID)
		if err != nil {
			return err
		}

		_, err = executor.ExecContext(txCtx, `
			INSERT INTO bank_transfers (id, from_player_id, to_player_id, amount, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, record.ID, record.FromPlayerID, record.ToPlayerID, record.Amount, record.CreatedAt)
		if err != nil {
			return err
		}

		err = executor.QueryRowContext(txCtx, `
			SELECT player_id, balance, updated_at
			FROM bank_accounts
			WHERE player_id = ?
		`, record.FromPlayerID).Scan(&fromAcc.PlayerID, &fromAcc.Balance, &fromAcc.UpdatedAt)
		if err != nil {
			return err
		}

		err = executor.QueryRowContext(txCtx, `
			SELECT player_id, balance, updated_at
			FROM bank_accounts
			WHERE player_id = ?
		`, record.ToPlayerID).Scan(&toAcc.PlayerID, &toAcc.Balance, &toAcc.UpdatedAt)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	return fromAcc, toAcc, nil
}

func (r *BankRepository) ListTransfers(ctx context.Context, playerID string, limit int) ([]bank.TransferRecord, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, from_player_id, to_player_id, amount, created_at
		FROM bank_transfers
		WHERE from_player_id = ? OR to_player_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, playerID, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []bank.TransferRecord
	for rows.Next() {
		var rec bank.TransferRecord
		if err := rows.Scan(&rec.ID, &rec.FromPlayerID, &rec.ToPlayerID, &rec.Amount, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
