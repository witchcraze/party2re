package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/witchcraze/party2re/internal/gvg"
)

// FindRoundTransition returns the committed state when the cached room is behind SQL.
func (r *GvGRepository) FindRoundTransition(ctx context.Context, roomID string, round int) (gvg.RoundTransition, bool, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT transition_json FROM gvg_match_transitions WHERE room_id = ? AND round_number >= ?
	`, roomID, round).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return gvg.RoundTransition{}, false, nil
	}
	if err != nil {
		return gvg.RoundTransition{}, false, err
	}
	var transition gvg.RoundTransition
	if err := json.Unmarshal(data, &transition); err != nil {
		return gvg.RoundTransition{}, false, err
	}
	return transition, true, nil
}

// CommitRoundTransition atomically claims a round, its rewards and final settlement.
// A completed match cannot be claimed again, even with a different round number.
// Transaction: RunInTx
// Lock Order: gvg_match_transitions(0) -> gvg_standings(8, ascending guild ID)
func (r *GvGRepository) CommitRoundTransition(ctx context.Context, roomID string, round int, transition gvg.RoundTransition) (gvg.RoundTransition, error) {
	if roomID == "" || round < 1 || round > gvg.MaxRounds || transition.Detail.Room.ID != roomID {
		return gvg.RoundTransition{}, fmt.Errorf("invalid gvg round transition")
	}
	var committed gvg.RoundTransition
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)
		if _, err := executor.ExecContext(txCtx, `
			INSERT INTO gvg_match_transitions (room_id) VALUES (?)
			ON DUPLICATE KEY UPDATE room_id = room_id
		`, roomID); err != nil {
			return err
		}
		var previous int
		var data []byte
		if err := executor.QueryRowContext(txCtx, `
			SELECT round_number, transition_json FROM gvg_match_transitions WHERE room_id = ? FOR UPDATE
		`, roomID).Scan(&previous, &data); err != nil {
			return err
		}
		if previous > 0 {
			if err := json.Unmarshal(data, &committed); err != nil {
				return err
			}
			if previous >= round || committed.Result.MatchCompleted {
				return nil
			}
		}

		// Acquire all standing rows before awarding any reward, in one stable order.
		guildIDs := []string{}
		if transition.Result.WinnerGuildID != "" {
			guildIDs = append(guildIDs, transition.Result.WinnerGuildID)
		}
		if transition.Settlement != nil {
			guildIDs = append(guildIDs, transition.Settlement.GuildIDs...)
		}
		sort.Strings(guildIDs)
		for i, guildID := range guildIDs {
			if i > 0 && guildID == guildIDs[i-1] {
				continue
			}
			if _, err := r.GetOrCreateStanding(txCtx, guildID); err != nil {
				return err
			}
			var lockedID string
			if err := executor.QueryRowContext(txCtx, "SELECT guild_id FROM gvg_standings WHERE guild_id = ? FOR UPDATE", guildID).Scan(&lockedID); err != nil {
				return err
			}
		}
		if err := r.addRoundWinGP(txCtx, transition.Result.WinnerGuildID, gvg.RoundWinGP); err != nil {
			return err
		}
		if transition.Settlement != nil {
			if err := r.recordMatchSettlement(txCtx, *transition.Settlement); err != nil {
				return err
			}
		}
		data, err := json.Marshal(transition)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `
			UPDATE gvg_match_transitions SET round_number = ?, transition_json = ? WHERE room_id = ?
		`, round, data, roomID); err != nil {
			return err
		}
		committed = transition
		return nil
	})
	if err != nil {
		return gvg.RoundTransition{}, err
	}
	return committed, nil
}
