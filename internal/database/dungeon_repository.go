package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
)

type DungeonRepository struct {
	db *sql.DB
}

func NewDungeonRepository(db *sql.DB) (*DungeonRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &DungeonRepository{db: db}, nil
}

func (r *DungeonRepository) GetRecord(ctx context.Context, characterID string) (dungeon.CharacterDungeonRecord, error) {
	var rec dungeon.CharacterDungeonRecord
	query := `
		SELECT character_id, highest_dungeon_cleared, total_expeditions,
		       total_floors_cleared, total_chests_opened, total_monsters_slain,
		       created_at, updated_at
		FROM character_dungeon_records
		WHERE character_id = ?
	`
	err := r.db.QueryRowContext(ctx, query, characterID).Scan(
		&rec.CharacterID,
		&rec.HighestDungeonCleared,
		&rec.TotalExpeditions,
		&rec.TotalFloorsCleared,
		&rec.TotalChestsOpened,
		&rec.TotalMonstersSlain,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		insertQuery := `
			INSERT INTO character_dungeon_records (
				character_id, highest_dungeon_cleared, total_expeditions,
				total_floors_cleared, total_chests_opened, total_monsters_slain,
				created_at, updated_at
			) VALUES (?, 0, 0, 0, 0, 0, ?, ?)
		`
		if _, err := r.db.ExecContext(ctx, insertQuery, characterID, now, now); err != nil {
			return dungeon.CharacterDungeonRecord{}, err
		}
		return dungeon.CharacterDungeonRecord{
			CharacterID: characterID,
			CreatedAt:   now,
			UpdatedAt:   now,
		}, nil
	}
	if err != nil {
		return dungeon.CharacterDungeonRecord{}, err
	}
	return rec, nil
}

func (r *DungeonRepository) GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
	query := `
		SELECT id, character_id, dungeon_id, current_floor, pos_x, pos_y,
		       current_hp, turns_remaining, accumulated_exp, accumulated_gold,
		       accumulated_items_json, status, started_at, updated_at
		FROM dungeon_active_expeditions
		WHERE character_id = ?
	`
	var exp dungeon.ActiveExpedition
	var itemsJSON string
	var statusStr string

	err := r.db.QueryRowContext(ctx, query, characterID).Scan(
		&exp.ID,
		&exp.CharacterID,
		&exp.DungeonID,
		&exp.CurrentFloor,
		&exp.PosX,
		&exp.PosY,
		&exp.CurrentHP,
		&exp.TurnsRemaining,
		&exp.AccumulatedExp,
		&exp.AccumulatedGold,
		&itemsJSON,
		&statusStr,
		&exp.StartedAt,
		&exp.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	exp.Status = dungeon.ExpeditionStatus(statusStr)
	exp.AccumulatedItems = dungeon.DecodeItems(itemsJSON)

	return &exp, nil
}

func (r *DungeonRepository) SaveActiveExpedition(ctx context.Context, exp dungeon.ActiveExpedition) error {
	itemsJSON := dungeon.EncodeItems(exp.AccumulatedItems)
	now := time.Now().UTC()

	query := `
		INSERT INTO dungeon_active_expeditions (
			id, character_id, dungeon_id, current_floor, pos_x, pos_y,
			current_hp, turns_remaining, accumulated_exp, accumulated_gold,
			accumulated_items_json, status, started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			current_floor = VALUES(current_floor),
			pos_x = VALUES(pos_x),
			pos_y = VALUES(pos_y),
			current_hp = VALUES(current_hp),
			turns_remaining = VALUES(turns_remaining),
			accumulated_exp = VALUES(accumulated_exp),
			accumulated_gold = VALUES(accumulated_gold),
			accumulated_items_json = VALUES(accumulated_items_json),
			status = VALUES(status),
			updated_at = VALUES(updated_at)
	`
	_, err := r.db.ExecContext(
		ctx,
		query,
		exp.ID,
		exp.CharacterID,
		exp.DungeonID,
		exp.CurrentFloor,
		exp.PosX,
		exp.PosY,
		exp.CurrentHP,
		exp.TurnsRemaining,
		exp.AccumulatedExp,
		exp.AccumulatedGold,
		itemsJSON,
		string(exp.Status),
		exp.StartedAt,
		now,
	)
	return err
}

func (r *DungeonRepository) DeleteActiveExpedition(ctx context.Context, characterID string) error {
	query := `DELETE FROM dungeon_active_expeditions WHERE character_id = ?`
	_, err := r.db.ExecContext(ctx, query, characterID)
	return err
}

func (r *DungeonRepository) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	record dungeon.CharacterDungeonRecord,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC()

	// 1. Upsert Character Dungeon Record
	upsertRecordQuery := `
		INSERT INTO character_dungeon_records (
			character_id, highest_dungeon_cleared, total_expeditions,
			total_floors_cleared, total_chests_opened, total_monsters_slain,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			highest_dungeon_cleared = VALUES(highest_dungeon_cleared),
			total_expeditions = VALUES(total_expeditions),
			total_floors_cleared = VALUES(total_floors_cleared),
			total_chests_opened = VALUES(total_chests_opened),
			total_monsters_slain = VALUES(total_monsters_slain),
			updated_at = VALUES(updated_at)
	`
	_, err = tx.ExecContext(
		ctx,
		upsertRecordQuery,
		record.CharacterID,
		record.HighestDungeonCleared,
		record.TotalExpeditions,
		record.TotalFloorsCleared,
		record.TotalChestsOpened,
		record.TotalMonstersSlain,
		now,
		now,
	)
	if err != nil {
		return err
	}

	// 2. Insert Dungeon Expedition History
	insertHistoryQuery := `
		INSERT INTO dungeon_expedition_history (
			id, character_id, dungeon_id, floors_reached, outcome,
			exp_reward, gold_reward, items_reward_count, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.ExecContext(
		ctx,
		insertHistoryQuery,
		history.ID,
		history.CharacterID,
		history.DungeonID,
		history.FloorsReached,
		string(history.Outcome),
		history.ExpReward,
		history.GoldReward,
		history.ItemsRewardCount,
		history.CreatedAt,
	)
	if err != nil {
		return err
	}

	// 3. Update Character if rewards were applied
	if character != nil {
		if err := updateCharacter(ctx, tx, *character); err != nil {
			return err
		}
	}

	// 4. Insert rewarded item instances into inventory_items
	for _, item := range rewardItems {
		insertItemQuery := `
			INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
			VALUES (?, ?, ?, ?, ?)
		`
		_, err = tx.ExecContext(
			ctx,
			insertItemQuery,
			item.ID,
			record.CharacterID,
			item.DefinitionID,
			item.Quantity,
			item.EnhancementLevel,
		)
		if err != nil {
			return err
		}
	}

	// 5. Delete active expedition
	deleteActiveQuery := `DELETE FROM dungeon_active_expeditions WHERE character_id = ?`
	if _, err := tx.ExecContext(ctx, deleteActiveQuery, record.CharacterID); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *DungeonRepository) GetHistory(ctx context.Context, characterID string, limit int) ([]dungeon.DungeonExpeditionHistory, error) {
	query := `
		SELECT id, character_id, dungeon_id, floors_reached, outcome,
		       exp_reward, gold_reward, items_reward_count, created_at
		FROM dungeon_expedition_history
		WHERE character_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []dungeon.DungeonExpeditionHistory
	for rows.Next() {
		var h dungeon.DungeonExpeditionHistory
		var outcomeStr string

		if err := rows.Scan(
			&h.ID,
			&h.CharacterID,
			&h.DungeonID,
			&h.FloorsReached,
			&outcomeStr,
			&h.ExpReward,
			&h.GoldReward,
			&h.ItemsRewardCount,
			&h.CreatedAt,
		); err != nil {
			return nil, err
		}
		h.Outcome = dungeon.ExpeditionStatus(outcomeStr)
		list = append(list, h)
	}

	return list, rows.Err()
}
