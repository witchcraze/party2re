package database

import (
	"context"
	"fmt"

	"github.com/witchcraze/party2re/internal/depot"
)

func (r *DepotRepository) AddRemittance(ctx context.Context, value depot.Remittance) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_pending_remittances (id, character_id, amount, source, sender_name, created_at)
		VALUES (?, ?, ?, ?, ?, UTC_TIMESTAMP(6))
	`, value.ID, value.CharacterID, value.Amount, string(value.Source), value.SenderName)
	return err
}

func (r *DepotRepository) ListRemittances(ctx context.Context, characterID string) ([]depot.Remittance, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, character_id, amount, source, sender_name
		FROM character_pending_remittances
		WHERE character_id = ?
		ORDER BY created_at, id
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []depot.Remittance
	for rows.Next() {
		var v depot.Remittance
		if err := rows.Scan(&v.ID, &v.CharacterID, &v.Amount, &v.Source, &v.SenderName); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *DepotRepository) SetRemittanceAmount(ctx context.Context, characterID, remittanceID string, amount int) error {
	query, args := `DELETE FROM character_pending_remittances WHERE id = ? AND character_id = ?`, []any{remittanceID, characterID}
	if amount > 0 {
		query = `UPDATE character_pending_remittances SET amount = ? WHERE id = ? AND character_id = ?`
		args = append([]any{amount}, args...)
	}
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		// Callers hold the character lock, so a missing row is a broken invariant.
		return fmt.Errorf("pending remittance %s not found for %s", remittanceID, characterID)
	}
	return nil
}
