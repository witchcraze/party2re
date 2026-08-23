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
	err := r.db.QueryRowContext(ctx, `
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	res, err := tx.ExecContext(ctx, `
		UPDATE characters
		SET money = money - ?
		WHERE id = ? AND money >= ?
	`, amount, characterID, amount)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	if affected == 0 {
		var exists int
		_ = tx.QueryRowContext(ctx, "SELECT COUNT(1) FROM characters WHERE id = ?", characterID).Scan(&exists)
		if exists == 0 {
			return bank.Account{}, corecharacter.Character{}, corecharacter.ErrNotFound
		}
		return bank.Account{}, corecharacter.Character{}, bank.ErrInsufficientFunds
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO bank_accounts (player_id, balance)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE balance = balance + VALUES(balance)
	`, playerID, int64(amount))
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	var acc bank.Account
	err = tx.QueryRowContext(ctx, `
		SELECT player_id, balance, updated_at
		FROM bank_accounts
		WHERE player_id = ?
	`, playerID).Scan(&acc.PlayerID, &acc.Balance, &acc.UpdatedAt)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	var char corecharacter.Character
	var gender, jobID string
	err = tx.QueryRowContext(ctx, `
		SELECT id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count
		FROM characters
		WHERE id = ?
	`, characterID).Scan(
		&char.ID,
		&char.Name,
		&jobID,
		&gender,
		&char.Stats.MaxHP,
		&char.Stats.MaxMP,
		&char.Stats.HP,
		&char.Stats.MP,
		&char.Stats.Attack,
		&char.Stats.Defense,
		&char.Stats.Agility,
		&char.Money,
		&char.Level,
		&char.Experience,
		&char.RebirthCount,
	)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	char.JobID = jobID
	char.Gender = gender

	if err := tx.Commit(); err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	return acc, char, nil
}

func (r *BankRepository) Withdraw(ctx context.Context, playerID string, characterID string, amount int) (bank.Account, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	res, err := tx.ExecContext(ctx, `
		UPDATE bank_accounts
		SET balance = balance - ?
		WHERE player_id = ? AND balance >= ?
	`, int64(amount), playerID, int64(amount))
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	if affected == 0 {
		return bank.Account{}, corecharacter.Character{}, bank.ErrInsufficientBalance
	}

	resChar, err := tx.ExecContext(ctx, `
		UPDATE characters
		SET money = money + ?
		WHERE id = ?
	`, amount, characterID)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	affectedChar, err := resChar.RowsAffected()
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	if affectedChar == 0 {
		return bank.Account{}, corecharacter.Character{}, corecharacter.ErrNotFound
	}

	var acc bank.Account
	err = tx.QueryRowContext(ctx, `
		SELECT player_id, balance, updated_at
		FROM bank_accounts
		WHERE player_id = ?
	`, playerID).Scan(&acc.PlayerID, &acc.Balance, &acc.UpdatedAt)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	var char corecharacter.Character
	var gender, jobID string
	err = tx.QueryRowContext(ctx, `
		SELECT id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count
		FROM characters
		WHERE id = ?
	`, characterID).Scan(
		&char.ID,
		&char.Name,
		&jobID,
		&gender,
		&char.Stats.MaxHP,
		&char.Stats.MaxMP,
		&char.Stats.HP,
		&char.Stats.MP,
		&char.Stats.Attack,
		&char.Stats.Defense,
		&char.Stats.Agility,
		&char.Money,
		&char.Level,
		&char.Experience,
		&char.RebirthCount,
	)
	if err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}
	char.JobID = jobID
	char.Gender = gender

	if err := tx.Commit(); err != nil {
		return bank.Account{}, corecharacter.Character{}, err
	}

	return acc, char, nil
}

func (r *BankRepository) Transfer(ctx context.Context, record bank.TransferRecord) (bank.Account, bank.Account, error) {
	var toPlayerExists int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM players WHERE id = ?", record.ToPlayerID).Scan(&toPlayerExists)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}
	if toPlayerExists == 0 {
		return bank.Account{}, bank.Account{}, bank.ErrAccountNotFound
	}

	// Ensure accounts exist to avoid insert gap locks
	_, _ = r.db.ExecContext(ctx, `
		INSERT IGNORE INTO bank_accounts (player_id, balance)
		VALUES (?, 0), (?, 0)
	`, record.FromPlayerID, record.ToPlayerID)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	p1, p2 := record.FromPlayerID, record.ToPlayerID
	if p1 > p2 {
		p1, p2 = p2, p1
	}

	var bal1, bal2 int64
	err = tx.QueryRowContext(ctx, "SELECT balance FROM bank_accounts WHERE player_id = ? FOR UPDATE", p1).Scan(&bal1)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}
	err = tx.QueryRowContext(ctx, "SELECT balance FROM bank_accounts WHERE player_id = ? FOR UPDATE", p2).Scan(&bal2)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	var fromBalance, toBalance int64
	if record.FromPlayerID == p1 {
		fromBalance, toBalance = bal1, bal2
	} else {
		fromBalance, toBalance = bal2, bal1
	}

	if fromBalance < record.Amount {
		return bank.Account{}, bank.Account{}, bank.ErrInsufficientBalance
	}

	newFromBalance := fromBalance - record.Amount
	_, err = tx.ExecContext(ctx, `
		UPDATE bank_accounts
		SET balance = ?
		WHERE player_id = ?
	`, newFromBalance, record.FromPlayerID)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	newToBalance := toBalance + record.Amount
	_, err = tx.ExecContext(ctx, `
		UPDATE bank_accounts
		SET balance = ?
		WHERE player_id = ?
	`, newToBalance, record.ToPlayerID)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO bank_transfers (id, from_player_id, to_player_id, amount, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, record.ID, record.FromPlayerID, record.ToPlayerID, record.Amount, record.CreatedAt)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	var fromAcc, toAcc bank.Account
	err = tx.QueryRowContext(ctx, `
		SELECT player_id, balance, updated_at
		FROM bank_accounts
		WHERE player_id = ?
	`, record.FromPlayerID).Scan(&fromAcc.PlayerID, &fromAcc.Balance, &fromAcc.UpdatedAt)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	err = tx.QueryRowContext(ctx, `
		SELECT player_id, balance, updated_at
		FROM bank_accounts
		WHERE player_id = ?
	`, record.ToPlayerID).Scan(&toAcc.PlayerID, &toAcc.Balance, &toAcc.UpdatedAt)
	if err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	if err := tx.Commit(); err != nil {
		return bank.Account{}, bank.Account{}, err
	}

	return fromAcc, toAcc, nil
}

func (r *BankRepository) ListTransfers(ctx context.Context, playerID string, limit int) ([]bank.TransferRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
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
