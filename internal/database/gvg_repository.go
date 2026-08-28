package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

type GvGRepository struct {
	db *sql.DB
}

func NewGvGRepository(db *sql.DB) (*GvGRepository, error) {
	if db == nil {
		return nil, errors.New("nil database connection")
	}
	return &GvGRepository{db: db}, nil
}

func (r *GvGRepository) GetOrCreateStanding(ctx context.Context, guildID string) (gvg.GvGStanding, error) {
	var st gvg.GvGStanding
	var gName sql.NullString

	query := `
		SELECT s.guild_id, g.name, s.rating, s.wins, s.losses, s.draws, s.victory_points,
		       s.bronze_medals, s.silver_medals, s.gold_medals, s.trophies, s.championship_cups,
		       s.champion_cups, s.updated_at
		FROM gvg_standings s
		JOIN guilds g ON s.guild_id = g.id
		WHERE s.guild_id = ?
	`
	err := r.db.QueryRowContext(ctx, query, guildID).Scan(
		&st.GuildID, &gName, &st.Rating, &st.Wins, &st.Losses, &st.Draws, &st.VictoryPoints,
		&st.BronzeMedals, &st.SilverMedals, &st.GoldMedals, &st.Trophies, &st.ChampionshipCups,
		&st.ChampionCups, &st.UpdatedAt,
	)
	if err == nil {
		if gName.Valid {
			st.GuildName = gName.String
		}
		return st, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return gvg.GvGStanding{}, fmt.Errorf("query gvg standing: %w", err)
	}

	// Create default standing
	insertQuery := `
		INSERT INTO gvg_standings (
			guild_id, rating, wins, losses, draws, victory_points,
			bronze_medals, silver_medals, gold_medals, trophies, championship_cups, champion_cups, updated_at
		) VALUES (?, 1000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, NOW())
		ON DUPLICATE KEY UPDATE updated_at = updated_at
	`
	if _, err := r.db.ExecContext(ctx, insertQuery, guildID); err != nil {
		return gvg.GvGStanding{}, fmt.Errorf("create gvg standing: %w", err)
	}

	return r.GetOrCreateStanding(ctx, guildID)
}

func (r *GvGRepository) FindOpponentGuilds(ctx context.Context, challengerGuildID string, limit int) ([]gvg.GuildCandidate, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT g.id, g.name, COALESCE(c.name, ''), g.level,
		       (SELECT COUNT(*) FROM guild_members gm WHERE gm.guild_id = g.id) AS member_count,
		       COALESCE(s.rating, 1000) AS rating,
		       COALESCE(s.victory_points, 0) AS victory_points,
		       COALESCE(s.wins, 0) AS wins,
		       COALESCE(s.losses, 0) AS losses
		FROM guilds g
		LEFT JOIN characters c ON g.leader_character_id = c.id
		LEFT JOIN gvg_standings s ON g.id = s.guild_id
		WHERE g.id != ?
		ORDER BY ABS(COALESCE(s.rating, 1000) - COALESCE((SELECT rating FROM gvg_standings WHERE guild_id = ?), 1000)) ASC,
		         g.level DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, challengerGuildID, challengerGuildID, limit)
	if err != nil {
		return nil, fmt.Errorf("find opponent guilds: %w", err)
	}
	defer rows.Close()

	var list []gvg.GuildCandidate
	for rows.Next() {
		var cand gvg.GuildCandidate
		if err := rows.Scan(
			&cand.GuildID,
			&cand.GuildName,
			&cand.LeaderName,
			&cand.Level,
			&cand.MemberCount,
			&cand.Rating,
			&cand.VictoryPoints,
			&cand.Wins,
			&cand.Losses,
		); err != nil {
			return nil, fmt.Errorf("scan opponent guild: %w", err)
		}
		list = append(list, cand)
	}
	return list, rows.Err()
}

func (r *GvGRepository) GetLeaderboard(ctx context.Context, limit int) ([]gvg.GvGStanding, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT s.guild_id, g.name, s.rating, s.wins, s.losses, s.draws, s.victory_points,
		       s.bronze_medals, s.silver_medals, s.gold_medals, s.trophies, s.championship_cups,
		       s.champion_cups, s.updated_at
		FROM gvg_standings s
		JOIN guilds g ON s.guild_id = g.id
		ORDER BY s.rating DESC, s.victory_points DESC, s.wins DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query gvg leaderboard: %w", err)
	}
	defer rows.Close()

	var list []gvg.GvGStanding
	for rows.Next() {
		var st gvg.GvGStanding
		var gName sql.NullString
		if err := rows.Scan(
			&st.GuildID, &gName, &st.Rating, &st.Wins, &st.Losses, &st.Draws, &st.VictoryPoints,
			&st.BronzeMedals, &st.SilverMedals, &st.GoldMedals, &st.Trophies, &st.ChampionshipCups,
			&st.ChampionCups, &st.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan gvg leaderboard row: %w", err)
		}
		if gName.Valid {
			st.GuildName = gName.String
		}
		list = append(list, st)
	}
	return list, rows.Err()
}

func (r *GvGRepository) GetMatchHistory(ctx context.Context, guildID string, limit int) ([]gvg.MatchRecord, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT m.id, m.challenger_guild_id, cg.name, m.defender_guild_id, dg.name,
		       COALESCE(m.winner_guild_id, ''), m.challenger_score, m.defender_score, m.total_rounds,
		       m.challenger_rating_before, m.challenger_rating_after,
		       m.defender_rating_before, m.defender_rating_after, m.created_at
		FROM gvg_matches m
		JOIN guilds cg ON m.challenger_guild_id = cg.id
		JOIN guilds dg ON m.defender_guild_id = dg.id
		WHERE m.challenger_guild_id = ? OR m.defender_guild_id = ?
		ORDER BY m.created_at DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, guildID, guildID, limit)
	if err != nil {
		return nil, fmt.Errorf("query gvg match history: %w", err)
	}
	defer rows.Close()

	var list []gvg.MatchRecord
	for rows.Next() {
		var m gvg.MatchRecord
		var cgName, dgName sql.NullString
		if err := rows.Scan(
			&m.ID, &m.ChallengerGuildID, &cgName, &m.DefenderGuildID, &dgName,
			&m.WinnerGuildID, &m.ChallengerScore, &m.DefenderScore, &m.TotalRounds,
			&m.ChallengerRatingBefore, &m.ChallengerRatingAfter,
			&m.DefenderRatingBefore, &m.DefenderRatingAfter, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan gvg match history row: %w", err)
		}
		if cgName.Valid {
			m.ChallengerGuildName = cgName.String
		}
		if dgName.Valid {
			m.DefenderGuildName = dgName.String
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (r *GvGRepository) GetMatchDetail(ctx context.Context, matchID string) (gvg.MatchRecord, error) {
	query := `
		SELECT m.id, m.challenger_guild_id, cg.name, m.defender_guild_id, dg.name,
		       COALESCE(m.winner_guild_id, ''), m.challenger_score, m.defender_score, m.total_rounds,
		       m.challenger_rating_before, m.challenger_rating_after,
		       m.defender_rating_before, m.defender_rating_after, m.created_at
		FROM gvg_matches m
		JOIN guilds cg ON m.challenger_guild_id = cg.id
		JOIN guilds dg ON m.defender_guild_id = dg.id
		WHERE m.id = ?
	`
	var m gvg.MatchRecord
	var cgName, dgName sql.NullString
	err := r.db.QueryRowContext(ctx, query, matchID).Scan(
		&m.ID, &m.ChallengerGuildID, &cgName, &m.DefenderGuildID, &dgName,
		&m.WinnerGuildID, &m.ChallengerScore, &m.DefenderScore, &m.TotalRounds,
		&m.ChallengerRatingBefore, &m.ChallengerRatingAfter,
		&m.DefenderRatingBefore, &m.DefenderRatingAfter, &m.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return gvg.MatchRecord{}, gvg.ErrMatchNotFound
		}
		return gvg.MatchRecord{}, fmt.Errorf("query match detail: %w", err)
	}
	if cgName.Valid {
		m.ChallengerGuildName = cgName.String
	}
	if dgName.Valid {
		m.DefenderGuildName = dgName.String
	}

	// Fetch rounds
	roundsQuery := `
		SELECT id, match_id, round_index, challenger_character_id, challenger_character_name,
		       defender_character_id, defender_character_name, COALESCE(winner_character_id, ''), turns, created_at
		FROM gvg_match_rounds
		WHERE match_id = ?
		ORDER BY round_index ASC
	`
	rRows, err := r.db.QueryContext(ctx, roundsQuery, matchID)
	if err != nil {
		return gvg.MatchRecord{}, fmt.Errorf("query match rounds: %w", err)
	}
	defer rRows.Close()

	for rRows.Next() {
		var rd gvg.MatchRound
		if err := rRows.Scan(
			&rd.ID, &rd.MatchID, &rd.RoundIndex, &rd.ChallengerCharacterID, &rd.ChallengerCharacterName,
			&rd.DefenderCharacterID, &rd.DefenderCharacterName, &rd.WinnerCharacterID, &rd.Turns, &rd.CreatedAt,
		); err != nil {
			return gvg.MatchRecord{}, fmt.Errorf("scan match round: %w", err)
		}
		m.Rounds = append(m.Rounds, rd)
	}

	return m, rRows.Err()
}

func (r *GvGRepository) RecordMatchAndUpdateStandings(
	ctx context.Context,
	match gvg.MatchRecord,
	challengerDelta, defenderDelta int,
	challengerExp, defenderExp int64,
	challengerVP, defenderVP int64,
	challengerMedal, defenderMedal bool,
	memberRewards map[string]gvg.MemberReward,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Insert match record
	var winnerID sql.NullString
	if match.WinnerGuildID != "" {
		winnerID = sql.NullString{String: match.WinnerGuildID, Valid: true}
	}

	insertMatchQuery := `
		INSERT INTO gvg_matches (
			id, challenger_guild_id, defender_guild_id, winner_guild_id,
			challenger_score, defender_score, total_rounds,
			challenger_rating_before, challenger_rating_after,
			defender_rating_before, defender_rating_after, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if _, err := tx.ExecContext(
		ctx,
		insertMatchQuery,
		match.ID,
		match.ChallengerGuildID,
		match.DefenderGuildID,
		winnerID,
		match.ChallengerScore,
		match.DefenderScore,
		match.TotalRounds,
		match.ChallengerRatingBefore,
		match.ChallengerRatingAfter,
		match.DefenderRatingBefore,
		match.DefenderRatingAfter,
		match.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert gvg match: %w", err)
	}

	// 2. Insert match rounds
	insertRoundQuery := `
		INSERT INTO gvg_match_rounds (
			id, match_id, round_index, challenger_character_id, challenger_character_name,
			defender_character_id, defender_character_name, winner_character_id, turns, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	for _, rd := range match.Rounds {
		var rdWinner sql.NullString
		if rd.WinnerCharacterID != "" {
			rdWinner = sql.NullString{String: rd.WinnerCharacterID, Valid: true}
		}
		if _, err := tx.ExecContext(
			ctx,
			insertRoundQuery,
			rd.ID,
			rd.MatchID,
			rd.RoundIndex,
			rd.ChallengerCharacterID,
			rd.ChallengerCharacterName,
			rd.DefenderCharacterID,
			rd.DefenderCharacterName,
			rdWinner,
			rd.Turns,
			rd.CreatedAt,
		); err != nil {
			return fmt.Errorf("insert gvg round %d: %w", rd.RoundIndex, err)
		}
	}

	// 3. Update challenger standing
	if err := updateStandingInTx(ctx, tx, match.ChallengerGuildID, challengerDelta, challengerVP, challengerMedal, defenderMedal); err != nil {
		return fmt.Errorf("update challenger standing: %w", err)
	}

	// 4. Update defender standing
	if err := updateStandingInTx(ctx, tx, match.DefenderGuildID, defenderDelta, defenderVP, defenderMedal, challengerMedal); err != nil {
		return fmt.Errorf("update defender standing: %w", err)
	}

	// 5. Update Guild EXP & Levels
	if err := updateGuildExpInTx(ctx, tx, match.ChallengerGuildID, challengerExp); err != nil {
		return fmt.Errorf("update challenger guild exp: %w", err)
	}
	if err := updateGuildExpInTx(ctx, tx, match.DefenderGuildID, defenderExp); err != nil {
		return fmt.Errorf("update defender guild exp: %w", err)
	}

	// 6. Update individual participating member rewards
	charRepo, err := NewCharacterRepository(r.db)
	if err != nil {
		return fmt.Errorf("new char repo: %w", err)
	}

	for charID, rew := range memberRewards {
		char, err := charRepo.FindByID(ctx, charID)
		if err != nil {
			continue // ignore if character vanished
		}
		if rew.Experience > 0 {
			if _, err := progression.ApplyExperience(&char, int(rew.Experience)); err != nil {
				return fmt.Errorf("apply member exp %s: %w", charID, err)
			}
		}
		char.Money += int(rew.Gold)

		// Save character in tx
		if err := updateCharacter(ctx, tx, char); err != nil {
			return fmt.Errorf("update character %s: %w", char.ID, err)
		}
	}

	return tx.Commit()
}

func updateStandingInTx(
	ctx context.Context,
	tx *sql.Tx,
	guildID string,
	delta int,
	vp int64,
	won bool,
	lost bool,
) error {
	// Lock or init standing
	var st gvg.GvGStanding
	query := `
		SELECT guild_id, rating, wins, losses, draws, victory_points,
		       bronze_medals, silver_medals, gold_medals, trophies, championship_cups,
		       champion_cups
		FROM gvg_standings
		WHERE guild_id = ?
		FOR UPDATE
	`
	err := tx.QueryRowContext(ctx, query, guildID).Scan(
		&st.GuildID, &st.Rating, &st.Wins, &st.Losses, &st.Draws, &st.VictoryPoints,
		&st.BronzeMedals, &st.SilverMedals, &st.GoldMedals, &st.Trophies, &st.ChampionshipCups,
		&st.ChampionCups,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			st = gvg.GvGStanding{
				GuildID: guildID,
				Rating:  gvg.DefaultRating,
			}
			// Insert initial
			initQuery := `
				INSERT INTO gvg_standings (
					guild_id, rating, wins, losses, draws, victory_points,
					bronze_medals, silver_medals, gold_medals, trophies, championship_cups, champion_cups, updated_at
				) VALUES (?, ?, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, NOW())
			`
			if _, err := tx.ExecContext(ctx, initQuery, guildID, st.Rating); err != nil {
				return fmt.Errorf("init standing row: %w", err)
			}
		} else {
			return fmt.Errorf("query standing for update: %w", err)
		}
	}

	st.Rating = max(0, st.Rating+delta)
	st.VictoryPoints += vp
	if won {
		st.Wins++
		st.BronzeMedals++
	} else if lost {
		st.Losses++
	} else {
		st.Draws++
	}
	st.PromoteMedals()

	updateQuery := `
		UPDATE gvg_standings
		SET rating = ?, wins = ?, losses = ?, draws = ?, victory_points = ?,
		    bronze_medals = ?, silver_medals = ?, gold_medals = ?, trophies = ?,
		    championship_cups = ?, champion_cups = ?, updated_at = NOW()
		WHERE guild_id = ?
	`
	_, err = tx.ExecContext(
		ctx,
		updateQuery,
		st.Rating,
		st.Wins,
		st.Losses,
		st.Draws,
		st.VictoryPoints,
		st.BronzeMedals,
		st.SilverMedals,
		st.GoldMedals,
		st.Trophies,
		st.ChampionshipCups,
		st.ChampionCups,
		guildID,
	)
	return err
}

func updateGuildExpInTx(ctx context.Context, tx *sql.Tx, guildID string, expGain int64) error {
	var currentExp int64
	var currentLvl int
	query := `SELECT exp, level FROM guilds WHERE id = ? FOR UPDATE`
	if err := tx.QueryRowContext(ctx, query, guildID).Scan(&currentExp, &currentLvl); err != nil {
		return fmt.Errorf("query guild exp for update: %w", err)
	}

	newExp := currentExp + expGain
	newLvl := guild.CalculateLevel(newExp)

	updateQuery := `UPDATE guilds SET exp = ?, level = ?, updated_at = NOW() WHERE id = ?`
	_, err := tx.ExecContext(ctx, updateQuery, newExp, newLvl, guildID)
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
