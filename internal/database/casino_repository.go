package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
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
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
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

func (r *CasinoRepository) GetAccountForUpdate(ctx context.Context, characterID string) (casino.Account, error) {
	var acc casino.Account
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, coins, updated_at
		FROM casino_accounts
		WHERE character_id = ?
		FOR UPDATE
	`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
			INSERT IGNORE INTO casino_accounts (character_id, coins)
			VALUES (?, 0)
		`, characterID)
		if err != nil {
			return casino.Account{}, err
		}
		err = ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
			SELECT character_id, coins, updated_at
			FROM casino_accounts
			WHERE character_id = ?
			FOR UPDATE
		`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
		if err != nil {
			return casino.Account{}, err
		}
	}
	if err != nil {
		return casino.Account{}, err
	}
	return acc, nil
}

func (r *CasinoRepository) DeductBetAndCreditPayout(ctx context.Context, characterID string, bet int64, payout int64) (casino.Account, error) {
	if bet < 0 || payout < 0 {
		return casino.Account{}, errors.New("bet and payout must be non-negative")
	}

	var acc casino.Account
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. If bet > 0, atomically verify and deduct bet
		if bet > 0 {
			res, err := executor.ExecContext(txCtx, `
				UPDATE casino_accounts
				SET coins = coins - ?
				WHERE character_id = ? AND coins >= ?
			`, bet, characterID, bet)
			if err != nil {
				return err
			}
			rows, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if rows == 0 {
				return casino.ErrInsufficientCoins
			}
		}

		// 2. If payout > 0, credit payout to account
		if payout > 0 {
			if bet > 0 {
				// Account already exists and is locked by step 1. Update coins directly
				// without INSERT to avoid unnecessary FK shared lock inversion on characters.
				_, err := executor.ExecContext(txCtx, `
					UPDATE casino_accounts
					SET coins = coins + ?
					WHERE character_id = ?
				`, payout, characterID)
				if err != nil {
					return err
				}
			} else {
				// bet == 0: attempt UPDATE first
				res, err := executor.ExecContext(txCtx, `
					UPDATE casino_accounts
					SET coins = coins + ?
					WHERE character_id = ?
				`, payout, characterID)
				if err != nil {
					return err
				}
				rows, err := res.RowsAffected()
				if err != nil {
					return err
				}
				if rows == 0 {
					// Account does not exist yet. Lock character first (Rank 2) before INSERT
					// to strictly satisfy lock hierarchy before InnoDB checks foreign key constraint.
					var exists string
					err := executor.QueryRowContext(txCtx, `
						SELECT id
						FROM characters
						WHERE id = ?
						FOR UPDATE
					`, characterID).Scan(&exists)
					if err != nil {
						return err
					}

					_, err = executor.ExecContext(txCtx, `
						INSERT INTO casino_accounts (character_id, coins)
						VALUES (?, ?)
						ON DUPLICATE KEY UPDATE coins = coins + VALUES(coins)
					`, characterID, payout)
					if err != nil {
						return err
					}
				}
			}
		}

		// 3. Scan updated account
		err := executor.QueryRowContext(txCtx, `
			SELECT character_id, coins, updated_at
			FROM casino_accounts
			WHERE character_id = ?
		`, characterID).Scan(&acc.CharacterID, &acc.Coins, &acc.UpdatedAt)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return casino.Account{}, err
	}

	return acc, nil
}

func (r *CasinoRepository) AdjustCoins(ctx context.Context, characterID string, delta int64) (casino.Account, error) {
	if delta < 0 {
		return r.DeductBetAndCreditPayout(ctx, characterID, -delta, 0)
	}
	return r.DeductBetAndCreditPayout(ctx, characterID, 0, delta)
}
