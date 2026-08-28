package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type CasinoRepository struct {
	db *sql.DB
}

func NewCasinoRepository(db *sql.DB) (*CasinoRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CasinoRepository{db: db}, nil
}

func (r *CasinoRepository) GetAccount(ctx context.Context, characterID string) (casino.Account, error) {
	var acc casino.Account
	err := r.db.QueryRowContext(ctx, `
		SELECT character_id, coins, updated_at
		FROM casino_accounts
		WHERE character_id = ?
	`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return casino.Account{
			CharacterID: characterID,
			Coins:       0,
			UpdatedAt:   time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return casino.Account{}, err
	}
	return acc, nil
}

func (r *CasinoRepository) ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64, goldCost int) (casino.Account, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Deduct gold from character
	res, err := tx.ExecContext(ctx, `
		UPDATE characters
		SET money = money - ?
		WHERE id = ? AND money >= ?
	`, goldCost, characterID, goldCost)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}
	if rows == 0 {
		return casino.Account{}, corecharacter.Character{}, casino.ErrInsufficientGold
	}

	// 2. Upsert casino coins
	_, err = tx.ExecContext(ctx, `
		INSERT INTO casino_accounts (character_id, coins)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE coins = coins + VALUES(coins)
	`, characterID, coins)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	// 3. Scan updated account & character
	var acc casino.Account
	err = tx.QueryRowContext(ctx, `
		SELECT character_id, coins, updated_at
		FROM casino_accounts
		WHERE character_id = ?
	`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	char, err := scanCharacterRow(tx.QueryRowContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ?
	`, characterID))
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	if err := tx.Commit(); err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	return acc, char, nil
}

func (r *CasinoRepository) ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64, goldReward int) (casino.Account, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Deduct coins from casino account
	res, err := tx.ExecContext(ctx, `
		UPDATE casino_accounts
		SET coins = coins - ?
		WHERE character_id = ? AND coins >= ?
	`, coins, characterID, coins)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}
	if rows == 0 {
		return casino.Account{}, corecharacter.Character{}, casino.ErrInsufficientCoins
	}

	// 2. Add gold to character
	_, err = tx.ExecContext(ctx, `
		UPDATE characters
		SET money = money + ?
		WHERE id = ?
	`, goldReward, characterID)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	// 3. Scan updated account & character
	var acc casino.Account
	err = tx.QueryRowContext(ctx, `
		SELECT character_id, coins, updated_at
		FROM casino_accounts
		WHERE character_id = ?
	`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	char, err := scanCharacterRow(tx.QueryRowContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ?
	`, characterID))
	if err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	if err := tx.Commit(); err != nil {
		return casino.Account{}, corecharacter.Character{}, err
	}

	return acc, char, nil
}

func (r *CasinoRepository) AdjustCoins(ctx context.Context, characterID string, delta int64) (casino.Account, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return casino.Account{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if delta >= 0 {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO casino_accounts (character_id, coins)
			VALUES (?, ?)
			ON DUPLICATE KEY UPDATE coins = coins + VALUES(coins)
		`, characterID, delta)
		if err != nil {
			return casino.Account{}, err
		}
	} else {
		deduct := -delta
		res, err := tx.ExecContext(ctx, `
			UPDATE casino_accounts
			SET coins = coins - ?
			WHERE character_id = ? AND coins >= ?
		`, deduct, characterID, deduct)
		if err != nil {
			return casino.Account{}, err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return casino.Account{}, err
		}
		if rows == 0 {
			return casino.Account{}, casino.ErrInsufficientCoins
		}
	}

	var acc casino.Account
	err = tx.QueryRowContext(ctx, `
		SELECT character_id, coins, updated_at
		FROM casino_accounts
		WHERE character_id = ?
	`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
	if err != nil {
		return casino.Account{}, err
	}

	if err := tx.Commit(); err != nil {
		return casino.Account{}, err
	}

	return acc, nil
}
