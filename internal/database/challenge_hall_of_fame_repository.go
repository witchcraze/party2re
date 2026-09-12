package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/challenge"
)

// SaveHallOfFame persists or updates a Hall of Fame entry for the given tier.
func (r *ChallengeRepository) SaveHallOfFame(ctx context.Context, entry challenge.HallOfFameEntry) error {
	membersJSON := challenge.EncodeJSON(entry.Members)
	query := `
		INSERT INTO challenge_hall_of_fame (
			tier_id, highest_round, party_name, party_color, cleared_at, members_json
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			highest_round = VALUES(highest_round),
			party_name = VALUES(party_name),
			party_color = VALUES(party_color),
			cleared_at = VALUES(cleared_at),
			members_json = VALUES(members_json)
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		entry.TierID,
		entry.HighestRound,
		entry.PartyName,
		entry.PartyColor,
		entry.ClearedAt,
		membersJSON,
	)
	return err
}

// GetHallOfFame retrieves the current record-holding party for a challenge tier.
func (r *ChallengeRepository) GetHallOfFame(ctx context.Context, tierID string) (*challenge.HallOfFameEntry, error) {
	query := `
		SELECT tier_id, highest_round, party_name, party_color, cleared_at, members_json
		FROM challenge_hall_of_fame
		WHERE tier_id = ?
	`
	var entry challenge.HallOfFameEntry
	var membersJSON string

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, tierID).Scan(
		&entry.TierID,
		&entry.HighestRound,
		&entry.PartyName,
		&entry.PartyColor,
		&entry.ClearedAt,
		&membersJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	members, err := challenge.DecodeJSON[[]challenge.HallOfFameMember](membersJSON)
	if err != nil {
		return nil, err
	}
	entry.Members = members

	return &entry, nil
}

// ListHallOfFame returns all Hall of Fame records ordered by tier.
func (r *ChallengeRepository) ListHallOfFame(ctx context.Context) ([]challenge.HallOfFameEntry, error) {
	query := `
		SELECT tier_id, highest_round, party_name, party_color, cleared_at, members_json
		FROM challenge_hall_of_fame
		ORDER BY tier_id ASC
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []challenge.HallOfFameEntry
	for rows.Next() {
		var entry challenge.HallOfFameEntry
		var membersJSON string
		if err := rows.Scan(
			&entry.TierID,
			&entry.HighestRound,
			&entry.PartyName,
			&entry.PartyColor,
			&entry.ClearedAt,
			&membersJSON,
		); err != nil {
			return nil, err
		}
		members, err := challenge.DecodeJSON[[]challenge.HallOfFameMember](membersJSON)
		if err != nil {
			return nil, err
		}
		entry.Members = members
		list = append(list, entry)
	}
	return list, rows.Err()
}
