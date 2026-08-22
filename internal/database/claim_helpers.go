package database

import (
	"context"
	"database/sql"
	"errors"
)

func claimFailure(ctx context.Context, tx *sql.Tx, table, id string, notFound, alreadyClaimed error) error {
	var claimed bool
	err := tx.QueryRowContext(ctx, "SELECT claimed FROM "+table+" WHERE id = ? FOR UPDATE", id).Scan(&claimed)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound
	}
	if err != nil {
		return err
	}
	if claimed {
		return alreadyClaimed
	}
	return notFound
}
