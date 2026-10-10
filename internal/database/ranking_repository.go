package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/witchcraze/party2re/internal/ranking"
)

type RankingRepository struct {
	db *sql.DB
}

var _ ranking.Repository = (*RankingRepository)(nil)

func NewRankingRepository(db *sql.DB) (*RankingRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &RankingRepository{db: db}, nil
}

func (r *RankingRepository) countCharacters(ctx context.Context) (int, error) {
	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, "SELECT COUNT(*) FROM characters").Scan(&total)
	return total, err
}

func (r *RankingRepository) countPlayers(ctx context.Context) (int, error) {
	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, "SELECT COUNT(*) FROM players").Scan(&total)
	return total, err
}

func (r *RankingRepository) GetLevelRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, c.level AS score, c.experience AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.level DESC, c.experience DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       COALESCE(SUM(c.money + c.deposit), 0) AS total_wealth,
		       COALESCE(SUM(c.deposit), 0) AS bank_balance,
		       COALESCE(SUM(c.money), 0) AS characters_money,
		       COUNT(c.id) AS character_count
		FROM players p
		LEFT JOIN characters c ON p.id = c.player_id
		GROUP BY p.id, p.username
		ORDER BY total_wealth DESC, p.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       c.level, c.experience, c.sp, c.money AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.money DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       c.level, c.experience, c.sp,
		       COALESCE(c.pvp_wins, 0) AS pvp_wins,
		       0 AS rating
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY pvp_wins DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       c.level, c.experience, c.sp,
		       COALESCE(br.total_boss_defeats, 0) AS boss_defeats,
		       COALESCE(br.highest_tier_cleared, 0) AS highest_tier
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN character_boss_records br ON c.id = br.character_id
		ORDER BY boss_defeats DESC, highest_tier DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       c.level, c.experience, c.sp,
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
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       c.level, c.experience, c.sp,
		       COUNT(jm.job_id) AS mastered_count,
		       c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		LEFT JOIN character_job_masteries jm ON c.id = jm.character_id
		GROUP BY c.id, c.player_id, p.username, c.name, c.job_id, c.gender, c.level, c.experience, c.sp
		ORDER BY mastered_count DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
	query := `
		SELECT job_id, total_points, male_points, female_points
		FROM job_popularity_stats
		ORDER BY total_points DESC, job_id ASC
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ranking.JobPopularityEntry
	var totalPoints int
	idx := 0
	for rows.Next() {
		var e ranking.JobPopularityEntry
		if err := rows.Scan(&e.JobID, &e.TotalCount, &e.MaleCount, &e.FemaleCount); err != nil {
			return nil, err
		}
		e.Rank = idx + 1
		idx++
		totalPoints += e.TotalCount
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []ranking.JobPopularityEntry{}
	}
	if totalPoints > 0 {
		for i := range entries {
			entries[i].Percentage = float64(entries[i].TotalCount) / float64(totalPoints) * 100.0
		}
	}
	return entries, nil
}

func (r *RankingRepository) RecordJobPopularity(ctx context.Context, jobID string, gender string, points int) error {
	if jobID == "" {
		return errors.New("job ID required")
	}
	if points < 0 {
		points = 0
	}

	normGender := strings.ToLower(strings.TrimSpace(gender))
	var malePoints, femalePoints int
	if normGender == "m" || normGender == "male" || normGender == "男" {
		malePoints = points
	} else {
		femalePoints = points
	}

	query := `
		INSERT INTO job_popularity_stats (job_id, male_points, female_points, total_points)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			male_points = male_points + VALUES(male_points),
			female_points = female_points + VALUES(female_points),
			total_points = total_points + VALUES(total_points)
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, jobID, malePoints, femalePoints, points)
	return err
}

func (r *RankingRepository) GetHelperRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, c.help_count AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.help_count DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
		       c.level, c.experience, c.sp, c.small_medals AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.small_medals DESC, c.level DESC, c.id ASC
		LIMIT ? OFFSET ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit, offset)
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
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, string(snapshot.RankingType), snapshot.SnapshotData, snapshot.TotalCount, snapshot.CalculatedAt, snapshot.UpdatedAt)
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
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, string(rankingType)).Scan(&typeStr, &s.SnapshotData, &s.TotalCount, &s.CalculatedAt, &s.UpdatedAt)
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
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query)
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
			&e.SP,
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
