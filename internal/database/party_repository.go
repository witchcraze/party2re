package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/party"
)

type PartyRepository struct {
	db *sql.DB
}

func NewPartyRepository(db *sql.DB) (*PartyRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &PartyRepository{db: db}, nil
}

func (r *PartyRepository) SaveParty(ctx context.Context, p party.Party) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO parties
			(id, leader_character_id, name, password_hash, stage_id, speed, max_members, min_level, max_level, min_hp, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.LeaderCharacterID, p.Name, p.PasswordHash, p.StageID, p.Speed, p.MaxMembers, p.MinLevel, p.MaxLevel, p.MinHP, p.Status, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *PartyRepository) GetParty(ctx context.Context, id string) (party.Party, error) {
	return r.getPartyWithQuery(ctx, id, `
		SELECT id, leader_character_id, name, password_hash, stage_id, speed, max_members, min_level, max_level, min_hp, status, created_at, updated_at
		FROM parties
		WHERE id = ?
	`)
}

func (r *PartyRepository) GetPartyForUpdate(ctx context.Context, id string) (party.Party, error) {
	return r.getPartyWithQuery(ctx, id, `
		SELECT id, leader_character_id, name, password_hash, stage_id, speed, max_members, min_level, max_level, min_hp, status, created_at, updated_at
		FROM parties
		WHERE id = ? FOR UPDATE
	`)
}

func (r *PartyRepository) getPartyWithQuery(ctx context.Context, id string, query string) (party.Party, error) {
	var p party.Party
	var passHash sql.NullString
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.LeaderCharacterID, &p.Name, &passHash, &p.StageID, &p.Speed,
		&p.MaxMembers, &p.MinLevel, &p.MaxLevel, &p.MinHP, &p.Status,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return party.Party{}, party.ErrNotFound
	}
	if err != nil {
		return party.Party{}, err
	}
	if passHash.Valid {
		p.PasswordHash = passHash.String
	}
	return p, nil
}

func (r *PartyRepository) ListParties(ctx context.Context, status string, limit, offset int) ([]party.PartySummary, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT
			p.id, p.name, p.leader_character_id, c.name, p.stage_id, '', p.speed,
			(SELECT COUNT(*) FROM party_members pm WHERE pm.party_id = p.id),
			p.max_members, (p.password_hash IS NOT NULL AND p.password_hash != ''),
			p.min_level, p.max_level, p.min_hp, p.status, p.created_at
		FROM parties p
		JOIN characters c ON p.leader_character_id = c.id
		WHERE (? = '' OR p.status = ?)
		ORDER BY p.created_at DESC
		LIMIT ? OFFSET ?
	`, status, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []party.PartySummary
	for rows.Next() {
		var s party.PartySummary
		if err := rows.Scan(
			&s.ID, &s.Name, &s.LeaderCharacterID, &s.LeaderName, &s.StageID, &s.StageName,
			&s.Speed, &s.CurrentMembers, &s.MaxMembers, &s.HasPassword,
			&s.MinLevel, &s.MaxLevel, &s.MinHP, &s.Status, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *PartyRepository) UpdateParty(ctx context.Context, p party.Party) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE parties
		SET name = ?, stage_id = ?, speed = ?, max_members = ?, min_level = ?, max_level = ?, min_hp = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, p.Name, p.StageID, p.Speed, p.MaxMembers, p.MinLevel, p.MaxLevel, p.MinHP, p.Status, p.UpdatedAt, p.ID)
	return err
}

func (r *PartyRepository) DeleteParty(ctx context.Context, id string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, "DELETE FROM parties WHERE id = ?", id)
	return err
}

func (r *PartyRepository) AddMember(ctx context.Context, m party.Member) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO party_members
			(party_id, character_id, is_leader, ready_state, joined_at)
		VALUES (?, ?, ?, ?, ?)
	`, m.PartyID, m.CharacterID, m.IsLeader, m.ReadyState, m.JoinedAt)
	return err
}

func (r *PartyRepository) GetMembers(ctx context.Context, partyID string) ([]party.Member, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT
			pm.party_id, pm.character_id, c.name, c.job_id, c.level, c.hp, c.max_hp,
			pm.is_leader, pm.ready_state, pm.joined_at
		FROM party_members pm
		JOIN characters c ON pm.character_id = c.id
		WHERE pm.party_id = ?
		ORDER BY pm.is_leader DESC, pm.joined_at ASC
	`, partyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []party.Member
	for rows.Next() {
		var m party.Member
		if err := rows.Scan(
			&m.PartyID, &m.CharacterID, &m.CharacterName, &m.JobID, &m.Level,
			&m.HP, &m.MaxHP, &m.IsLeader, &m.ReadyState, &m.JoinedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func (r *PartyRepository) GetMember(ctx context.Context, partyID, characterID string) (party.Member, error) {
	var m party.Member
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT
			pm.party_id, pm.character_id, c.name, c.job_id, c.level, c.hp, c.max_hp,
			pm.is_leader, pm.ready_state, pm.joined_at
		FROM party_members pm
		JOIN characters c ON pm.character_id = c.id
		WHERE pm.party_id = ? AND pm.character_id = ?
	`, partyID, characterID).Scan(
		&m.PartyID, &m.CharacterID, &m.CharacterName, &m.JobID, &m.Level,
		&m.HP, &m.MaxHP, &m.IsLeader, &m.ReadyState, &m.JoinedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return party.Member{}, party.ErrCharacterNotInParty
	}
	if err != nil {
		return party.Member{}, err
	}
	return m, nil
}

func (r *PartyRepository) GetActivePartyByCharacter(ctx context.Context, characterID string) (party.Party, party.Member, error) {
	var p party.Party
	var m party.Member
	var passHash sql.NullString
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT
			p.id, p.leader_character_id, p.name, p.password_hash, p.stage_id, p.speed,
			p.max_members, p.min_level, p.max_level, p.min_hp, p.status, p.created_at, p.updated_at,
			pm.party_id, pm.character_id, c.name, c.job_id, c.level, c.hp, c.max_hp,
			pm.is_leader, pm.ready_state, pm.joined_at
		FROM party_members pm
		JOIN parties p ON pm.party_id = p.id
		JOIN characters c ON pm.character_id = c.id
		WHERE pm.character_id = ? AND p.status != 'disbanded'
	`, characterID).Scan(
		&p.ID, &p.LeaderCharacterID, &p.Name, &passHash, &p.StageID, &p.Speed,
		&p.MaxMembers, &p.MinLevel, &p.MaxLevel, &p.MinHP, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		&m.PartyID, &m.CharacterID, &m.CharacterName, &m.JobID, &m.Level,
		&m.HP, &m.MaxHP, &m.IsLeader, &m.ReadyState, &m.JoinedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return party.Party{}, party.Member{}, party.ErrNotFound
	}
	if err != nil {
		return party.Party{}, party.Member{}, err
	}
	if passHash.Valid {
		p.PasswordHash = passHash.String
	}
	return p, m, nil
}

func (r *PartyRepository) RemoveMember(ctx context.Context, partyID, characterID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM party_members
		WHERE party_id = ? AND character_id = ?
	`, partyID, characterID)
	return err
}

func (r *PartyRepository) UpdateMemberReady(ctx context.Context, partyID, characterID string, ready bool) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE party_members
		SET ready_state = ?
		WHERE party_id = ? AND character_id = ?
	`, ready, partyID, characterID)
	return err
}

func (r *PartyRepository) CountMembers(ctx context.Context, partyID string) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM party_members
		WHERE party_id = ?
	`, partyID).Scan(&count)
	return count, err
}

func (r *PartyRepository) SaveAdventureLog(ctx context.Context, log party.PartyAdventureLog) error {
	var details sql.NullString
	if log.DetailsJSON != "" {
		details.String = log.DetailsJSON
		details.Valid = true
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO party_adventure_logs
			(id, party_id, stage_id, outcome, turns, total_exp, total_gold, synergy_bonus_percent, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.PartyID, log.StageID, log.Outcome, log.Turns, log.TotalEXP, log.TotalGold, log.SynergyBonusPercent, details, log.CreatedAt)
	return err
}
