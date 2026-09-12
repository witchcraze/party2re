package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

	executor := ExecutorFromContext(ctx, r.db)
	query := `
		SELECT s.guild_id, g.name, s.wins, s.losses, s.draws, s.victory_points,
		       s.bronze_medals, s.silver_medals, s.gold_medals, s.orders, s.trophies,
		       s.championship_cups, s.champion_cups, s.updated_at
		FROM gvg_standings s
		JOIN guilds g ON s.guild_id = g.id
		WHERE s.guild_id = ?
	`
	err := executor.QueryRowContext(ctx, query, guildID).Scan(
		&st.GuildID, &gName, &st.Wins, &st.Losses, &st.Draws, &st.VictoryPoints,
		&st.BronzeMedals, &st.SilverMedals, &st.GoldMedals, &st.Orders, &st.Trophies,
		&st.ChampionshipCups, &st.ChampionCups, &st.UpdatedAt,
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

	// Verify guild exists
	var exists bool
	if err := executor.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM guilds WHERE id = ?)", guildID).Scan(&exists); err != nil || !exists {
		return gvg.GvGStanding{}, gvg.ErrGuildNotFound
	}

	insertQuery := `
		INSERT INTO gvg_standings (
			guild_id, wins, losses, draws, victory_points,
			bronze_medals, silver_medals, gold_medals, orders, trophies, championship_cups, champion_cups, updated_at
		) VALUES (?, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, NOW())
		ON DUPLICATE KEY UPDATE updated_at = updated_at
	`
	if _, err := executor.ExecContext(ctx, insertQuery, guildID); err != nil {
		return gvg.GvGStanding{}, fmt.Errorf("create gvg standing: %w", err)
	}

	return r.GetOrCreateStanding(ctx, guildID)
}

func (r *GvGRepository) GetLeaderboard(ctx context.Context, limit int) ([]gvg.GvGStanding, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT s.guild_id, g.name, s.wins, s.losses, s.draws, s.victory_points,
		       s.bronze_medals, s.silver_medals, s.gold_medals, s.orders, s.trophies,
		       s.championship_cups, s.champion_cups, s.updated_at
		FROM gvg_standings s
		JOIN guilds g ON s.guild_id = g.id
		ORDER BY s.champion_cups DESC, s.championship_cups DESC, s.trophies DESC, s.orders DESC,
		         s.gold_medals DESC, s.silver_medals DESC, s.bronze_medals DESC, s.victory_points DESC, s.wins DESC
		LIMIT ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("get gvg leaderboard: %w", err)
	}
	defer rows.Close()

	var list []gvg.GvGStanding
	for rows.Next() {
		var st gvg.GvGStanding
		var gName sql.NullString
		if err := rows.Scan(
			&st.GuildID, &gName, &st.Wins, &st.Losses, &st.Draws, &st.VictoryPoints,
			&st.BronzeMedals, &st.SilverMedals, &st.GoldMedals, &st.Orders, &st.Trophies,
			&st.ChampionshipCups, &st.ChampionCups, &st.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan gvg leaderboard row: %w", err)
		}
		if gName.Valid {
			st.GuildName = gName.String
		}
		list = append(list, st)
	}
	return list, nil
}

func (r *GvGRepository) AddRoundWinGP(ctx context.Context, guildID string, gp int) error {
	if guildID == "" || gp <= 0 {
		return nil
	}
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		if _, err := r.GetOrCreateStanding(txCtx, guildID); err != nil {
			return err
		}
		_, err := executor.ExecContext(txCtx, `
			UPDATE gvg_standings
			SET victory_points = victory_points + ?, updated_at = NOW()
			WHERE guild_id = ?
		`, gp, guildID)
		return err
	})
}

func (r *GvGRepository) RecordMatchSettlement(ctx context.Context, settlement gvg.MatchSettlement) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		for _, gID := range settlement.GuildIDs {
			if _, err := r.GetOrCreateStanding(txCtx, gID); err != nil {
				return err
			}
		}

		if settlement.IsDraw {
			for _, gID := range settlement.GuildIDs {
				extraGP := settlement.ParticipantGP[gID]
				_, err := executor.ExecContext(txCtx, `
					UPDATE gvg_standings
					SET draws = draws + 1, victory_points = victory_points + ?, updated_at = NOW()
					WHERE guild_id = ?
				`, extraGP, gID)
				if err != nil {
					return err
				}
			}
			return nil
		}

		for _, gID := range settlement.GuildIDs {
			extraGP := settlement.ParticipantGP[gID]
			if gID == settlement.WinnerGuildID {
				var st gvg.GvGStanding
				var gName sql.NullString
				err := executor.QueryRowContext(txCtx, `
					SELECT s.guild_id, g.name, s.wins, s.losses, s.draws, s.victory_points,
					       s.bronze_medals, s.silver_medals, s.gold_medals, s.orders, s.trophies,
					       s.championship_cups, s.champion_cups, s.updated_at
					FROM gvg_standings s
					JOIN guilds g ON s.guild_id = g.id
					WHERE s.guild_id = ? FOR UPDATE
				`, gID).Scan(
					&st.GuildID, &gName, &st.Wins, &st.Losses, &st.Draws, &st.VictoryPoints,
					&st.BronzeMedals, &st.SilverMedals, &st.GoldMedals, &st.Orders, &st.Trophies,
					&st.ChampionshipCups, &st.ChampionCups, &st.UpdatedAt,
				)
				if err != nil {
					return err
				}

				st.Wins++
				st.VictoryPoints += int64(settlement.WinnerPrizeGP + extraGP)
				st.BronzeMedals++
				st.PromoteMedals()

				_, err = executor.ExecContext(txCtx, `
					UPDATE gvg_standings
					SET wins = ?, victory_points = ?,
					    bronze_medals = ?, silver_medals = ?, gold_medals = ?, orders = ?, trophies = ?,
					    championship_cups = ?, champion_cups = ?, updated_at = NOW()
					WHERE guild_id = ?
				`, st.Wins, st.VictoryPoints,
					st.BronzeMedals, st.SilverMedals, st.GoldMedals, st.Orders, st.Trophies,
					st.ChampionshipCups, st.ChampionCups, gID)
				if err != nil {
					return err
				}
			} else {
				_, err := executor.ExecContext(txCtx, `
					UPDATE gvg_standings
					SET losses = losses + 1, victory_points = victory_points + ?, updated_at = NOW()
					WHERE guild_id = ?
				`, extraGP, gID)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}
