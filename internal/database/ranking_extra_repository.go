package database

import (
	"context"

	"github.com/witchcraze/party2re/internal/ranking"
)

// GetCasinoWinsRanking returns character rankings sorted by casino showdown wins (cas_c).
func (r *RankingRepository) GetCasinoWinsRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, c.casino_wins AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.casino_wins DESC, c.level DESC, c.id ASC
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

// GetAlchemyRanking returns character rankings sorted by total alchemy syntheses (alc_c).
func (r *RankingRepository) GetAlchemyRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, COALESCE(ca.total_crafts, 0) AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN character_alchemy ca ON c.id = ca.character_id
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY score DESC, c.level DESC, c.id ASC
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

// IncrementJobChangeCount increments the active weekly job change count for a character.
func (r *RankingRepository) IncrementJobChangeCount(ctx context.Context, characterID string) error {
	query := `
		INSERT INTO weekly_job_changes (character_id, change_count)
		VALUES (?, 1)
		ON DUPLICATE KEY UPDATE change_count = change_count + 1
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, characterID)
	return err
}

// GetActiveWeeklyJobChangeRanking retrieves the current week's active job change leaderboard.
func (r *RankingRepository) GetActiveWeeklyJobChangeRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	var total int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, "SELECT COUNT(*) FROM weekly_job_changes").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, wjc.change_count AS score, c.level AS secondary_score
		FROM weekly_job_changes wjc
		JOIN characters c ON wjc.character_id = c.id
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY wjc.change_count DESC, c.level DESC, c.id ASC
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

// GetWeeklyJobChangeRanking implements CharacterRankingRepository.
func (r *RankingRepository) GetWeeklyJobChangeRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	return r.GetActiveWeeklyJobChangeRanking(ctx, limit, offset)
}

// ResetWeeklyJobChanges clears all active weekly job change counts upon Sunday midnight rotation.
func (r *RankingRepository) ResetWeeklyJobChanges(ctx context.Context) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, "DELETE FROM weekly_job_changes")
	return err
}

// RecordLegend records a character's completion in the permanent Hall of Fame.
func (r *RankingRepository) RecordLegend(ctx context.Context, entry ranking.LegendEntry) (bool, error) {
	query := `
		INSERT IGNORE INTO legend_records (
			category, character_id, character_name, guild_name, color, icon, message, inducted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		entry.Category,
		entry.CharacterID,
		entry.CharacterName,
		entry.GuildName,
		entry.Color,
		entry.Icon,
		entry.Message,
		entry.InductedAt,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// GetLegendInductees returns chronological inductees for a Hall of Fame category.
func (r *RankingRepository) GetLegendInductees(ctx context.Context, category ranking.LegendCategory) ([]ranking.LegendEntry, error) {
	query := `
		SELECT id, category, character_id, character_name, guild_name, color, icon, message, inducted_at
		FROM legend_records
		WHERE category = ?
		ORDER BY inducted_at ASC, id ASC
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ranking.LegendEntry
	for rows.Next() {
		var e ranking.LegendEntry
		var cat string
		if err := rows.Scan(
			&e.ID,
			&cat,
			&e.CharacterID,
			&e.CharacterName,
			&e.GuildName,
			&e.Color,
			&e.Icon,
			&e.Message,
			&e.InductedAt,
		); err != nil {
			return nil, err
		}
		e.Category = ranking.LegendCategory(cat)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetLegendCategoryCounts returns total inductee counts for all Hall of Fame categories.
func (r *RankingRepository) GetLegendCategoryCounts(ctx context.Context) (map[ranking.LegendCategory]int, error) {
	query := `SELECT category, COUNT(*) FROM legend_records GROUP BY category`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[ranking.LegendCategory]int)
	for rows.Next() {
		var cat string
		var cnt int
		if err := rows.Scan(&cat, &cnt); err != nil {
			return nil, err
		}
		counts[ranking.LegendCategory(cat)] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}

// GetMonsterKillsRanking returns character rankings sorted by defeated strong enemies (kill_m / 英雄ランキング).
func (r *RankingRepository) GetMonsterKillsRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, c.monster_kills AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.monster_kills DESC, c.level DESC, c.id ASC
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

// GetMaoCountRanking returns character rankings sorted by unsealed demon king defeats (mao_c / 魔王ランキング).
func (r *RankingRepository) GetMaoCountRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, c.mao_count AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.mao_count DESC, c.level DESC, c.id ASC
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

// GetHeroCountRanking returns character rankings sorted by hero achievements (hero_c / 勇者ランキング).
func (r *RankingRepository) GetHeroCountRanking(ctx context.Context, limit, offset int) ([]ranking.CharacterRankingEntry, int, error) {
	total, err := r.countCharacters(ctx)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT c.id, c.player_id, COALESCE(p.username, ''), c.name, c.job_id, c.gender,
		       c.level, c.experience, c.sp, c.hero_count AS score, c.level AS secondary_score
		FROM characters c
		LEFT JOIN players p ON c.player_id = p.id
		ORDER BY c.hero_count DESC, c.level DESC, c.id ASC
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
