package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/challenge"
)

// RaiseHallOfFame replaces a tier's record only with a higher round, so a
// concurrent lower record never overwrites it. It reports whether it wrote.
func (r *ChallengeRepository) RaiseHallOfFame(ctx context.Context, entry challenge.HallOfFameEntry) (bool, error) {
	// party_name etc. precede highest_round: MariaDB assigns left to right.
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO challenge_hall_of_fame (tier_id, highest_round, party_name, party_color, cleared_at, members_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			party_name = IF(VALUES(highest_round) > highest_round, VALUES(party_name), party_name),
			party_color = IF(VALUES(highest_round) > highest_round, VALUES(party_color), party_color),
			cleared_at = IF(VALUES(highest_round) > highest_round, VALUES(cleared_at), cleared_at),
			members_json = IF(VALUES(highest_round) > highest_round, VALUES(members_json), members_json),
			highest_round = GREATEST(highest_round, VALUES(highest_round))`,
		entry.TierID, entry.HighestRound, entry.PartyName, entry.PartyColor, entry.ClearedAt, challenge.EncodeJSON(entry.Members))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
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
