package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/ranking"
)

type RankingRepository struct {
	db *sql.DB
}

func NewRankingRepository(db *sql.DB) (*RankingRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &RankingRepository{db: db}, nil
}

func (r *RankingRepository) countCharacters(ctx context.Context) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters").Scan(&total)
	return total, err
}

func (r *RankingRepository) countPlayers(ctx context.Context) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM players").Scan(&total)
	return total, err
}

func (r *RankingRepository) GetLevelRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count, c.level AS score, c.experience AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.level DESC, c.experience DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetPlayerWealthRanking(ctx context.Context, limit, offset int) ([]ranking.PlayerWealthRankingEntry, int, error) {
	total, err := r.countPlayers(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT p.id, p.username,
		       (COALESCE(b.balance, 0) + COALESCE(SUM(c.money), 0)) AS total_wealth,
		       COALESCE(b.balance, 0) AS bank_balance,
		       COALESCE(SUM(c.money), 0) AS characters_money,
		       COUNT(c.id) AS character_count
		FROM players p
		LEFT JOIN bank_accounts b ON p.id = b.player_id
		LEFT JOIN characters c ON p.id = c.player_id
		GROUP BY p.id, p.username, b.balance
		ORDER BY total_wealth DESC, p.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []ranking.PlayerWealthRankingEntry
	idx := 0
	for rows.Next() {
		var e ranking.PlayerWealthRankingEntry
		if err := rows.Scan(
			&e.PlayerID,
			&e.Username,
			&e.TotalWealth,
			&e.BankBalance,
			&e.CharactersMoney,
			&e.CharacterCount,
		); err != nil {
			return nil, 0, err
		}
		e.Rank = offset + idx + 1
		idx++
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetCharacterWealthRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count, c.money AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.money DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetBattleVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count,
		       (COALESCE(ar.wins, 0) + COALESCE(br.total_boss_defeats, 0) + COALESCE(adv.adventure_wins, 0)) AS total_victories,
		       COALESCE(ar.wins, 0) AS pvp_wins
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN arena_ratings ar ON c.id = ar.character_id
		LEFT JOIN character_boss_records br ON c.id = br.character_id
		LEFT JOIN (
			SELECT character_id, COUNT(*) AS adventure_wins
			FROM adventures
			WHERE outcome = 'WIN'
			GROUP BY character_id
		) adv ON c.id = adv.character_id
		ORDER BY total_victories DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetPvPVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count,
		       COALESCE(ar.wins, 0) AS pvp_wins,
		       COALESCE(ar.rating, 1000) AS rating
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN arena_ratings ar ON c.id = ar.character_id
		ORDER BY pvp_wins DESC, rating DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetBossDefeatRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count,
		       COALESCE(br.total_boss_defeats, 0) AS boss_defeats,
		       COALESCE(br.highest_tier_cleared, 0) AS highest_tier
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN character_boss_records br ON c.id = br.character_id
		ORDER BY boss_defeats DESC, highest_tier DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetAdventureVictoryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count,
		       COALESCE(adv.adventure_wins, 0) AS adventure_wins,
		       c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN (
			SELECT character_id, COUNT(*) AS adventure_wins
			FROM adventures
			WHERE outcome = 'WIN'
			GROUP BY character_id
		) adv ON c.id = adv.character_id
		ORDER BY adventure_wins DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetJobMasteryRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count,
		       COUNT(jm.job_id) AS mastered_count,
		       c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN character_job_masteries jm ON c.id = jm.character_id
		GROUP BY c.id, c.player_id, p.username, c.name, c.job_id, c.gender, c.level, c.experience, c.rebirth_count
		ORDER BY mastered_count DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetJobPopularityRanking(ctx context.Context) ([]ranking.JobPopularityEntry, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT c.job_id,
		       COUNT(*) AS total_count,
		       SUM(CASE WHEN c.gender IN ('m', 'male') THEN 1 ELSE 0 END) AS male_count,
		       SUM(CASE WHEN c.gender IN ('f', 'female') THEN 1 ELSE 0 END) AS female_count
		FROM characters c
		GROUP BY c.job_id
		ORDER BY total_count DESC, c.job_id ASC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ranking.JobPopularityEntry
	idx := 0
	for rows.Next() {
		var e ranking.JobPopularityEntry
		if err := rows.Scan(&e.JobID, &e.TotalCount, &e.MaleCount, &e.FemaleCount); err != nil {
			return nil, err
		}
		e.Rank = idx + 1
		idx++
		if total > 0 {
			e.Percentage = float64(e.TotalCount) / float64(total) * 100.0
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *RankingRepository) GetHelperRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count, c.help_count AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.help_count DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetRebirthRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count, c.rebirth_count AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.rebirth_count DESC, c.level DESC, c.experience DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) GetSmallMedalRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.rebirth_count, c.small_medals AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.small_medals DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := scanCharacterRankingEntries(rows, offset)
	if err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

func (r *RankingRepository) SaveSnapshot(ctx context.Context, snapshot ranking.RankingSnapshot) error {
	query := `
		INSERT INTO ranking_snapshots (ranking_type, snapshot_data, total_count, calculated_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			snapshot_data = VALUES(snapshot_data),
			total_count = VALUES(total_count),
			calculated_at = VALUES(calculated_at),
			updated_at = VALUES(updated_at)
	`
	_, err := r.db.ExecContext(ctx, query, string(snapshot.RankingType), snapshot.SnapshotData, snapshot.TotalCount, snapshot.CalculatedAt, snapshot.UpdatedAt)
	return err
}

func (r *RankingRepository) GetSnapshot(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, error) {
	query := `
		SELECT ranking_type, snapshot_data, total_count, calculated_at, updated_at
		FROM ranking_snapshots
		WHERE ranking_type = ?
	`
	var s ranking.RankingSnapshot
	var typeStr string
	err := r.db.QueryRowContext(ctx, query, string(rankingType)).Scan(&typeStr, &s.SnapshotData, &s.TotalCount, &s.CalculatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ranking.RankingSnapshot{}, ranking.ErrSnapshotNotFound
	}
	if err != nil {
		return ranking.RankingSnapshot{}, err
	}
	s.RankingType = ranking.RankingType(typeStr)
	return s, nil
}

func (r *RankingRepository) GetAllSnapshots(ctx context.Context) (map[ranking.RankingType]ranking.RankingSnapshot, error) {
	query := `
		SELECT ranking_type, snapshot_data, total_count, calculated_at, updated_at
		FROM ranking_snapshots
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[ranking.RankingType]ranking.RankingSnapshot)
	for rows.Next() {
		var s ranking.RankingSnapshot
		var typeStr string
		if err := rows.Scan(&typeStr, &s.SnapshotData, &s.TotalCount, &s.CalculatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.RankingType = ranking.RankingType(typeStr)
		result[s.RankingType] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanCharacterRankingEntries(rows *sql.Rows, offset int) ([]ranking.CharacterRankingEntry, error) {
	var entries []ranking.CharacterRankingEntry
	idx := 0
	for rows.Next() {
		var e ranking.CharacterRankingEntry
		var secondary sql.NullInt64
		if err := rows.Scan(
			&e.CharacterID,
			&e.PlayerID,
			&e.PlayerUsername,
			&e.CharacterName,
			&e.JobID,
			&e.Gender,
			&e.Level,
			&e.Experience,
			&e.RebirthCount,
			&e.Score,
			&secondary,
		); err != nil {
			return nil, err
		}
		if secondary.Valid {
			e.SecondaryScore = secondary.Int64
		}
		e.Rank = offset + idx + 1
		idx++
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
