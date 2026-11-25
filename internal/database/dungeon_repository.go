package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
)

type DungeonRepository struct {
	db         *sql.DB
	postBattle PostBattleSettler
}

func NewDungeonRepository(db *sql.DB) (*DungeonRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &DungeonRepository{db: db}, nil
}

func (r *DungeonRepository) GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
	query := `
		SELECT id, character_id, party_id, dungeon_id, pos_x, pos_y,
		       current_hp, turns_remaining, accumulated_exp, accumulated_gold,
		       members_json, status, started_at, updated_at
		FROM dungeon_active_expeditions
		WHERE character_id = ?
	`
	var exp dungeon.ActiveExpedition
	var statusStr string
	var partyID sql.NullString
	var membersJSON sql.NullString

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, characterID).Scan(
		&exp.ID,
		&exp.CharacterID,
		&partyID,
		&exp.DungeonID,
		&exp.PosX,
		&exp.PosY,
		&exp.CurrentHP,
		&exp.Round, // shortcut: round is stored in the old turns_remaining column; drop with the unused table (#2365)
		&exp.AccumulatedExp,
		&exp.AccumulatedGold,
		&membersJSON,
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

	if partyID.Valid {
		exp.PartyID = partyID.String
	}
	if membersJSON.Valid && membersJSON.String != "" {
		if err := json.Unmarshal([]byte(membersJSON.String), &exp.Members); err != nil {
			return nil, err
		}
	}

	exp.Status = dungeon.ExpeditionStatus(statusStr)

	return &exp, nil
}

func (r *DungeonRepository) SaveActiveExpedition(ctx context.Context, exp dungeon.ActiveExpedition) error {
	itemsJSON := "[]"
	membersJSON := dungeon.EncodeMembers(exp.Members)
	now := time.Now().UTC()

	var partyID sql.NullString
	if exp.PartyID != "" {
		partyID = sql.NullString{String: exp.PartyID, Valid: true}
	}

	query := `
		INSERT INTO dungeon_active_expeditions (
			id, character_id, party_id, dungeon_id, pos_x, pos_y,
			current_hp, turns_remaining, accumulated_exp, accumulated_gold,
			accumulated_items_json, members_json, status, started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			id = VALUES(id),
			party_id = VALUES(party_id),
			dungeon_id = VALUES(dungeon_id),
			pos_x = VALUES(pos_x),
			pos_y = VALUES(pos_y),
			current_hp = VALUES(current_hp),
			turns_remaining = VALUES(turns_remaining),
			accumulated_exp = VALUES(accumulated_exp),
			accumulated_gold = VALUES(accumulated_gold),
			accumulated_items_json = VALUES(accumulated_items_json),
			members_json = VALUES(members_json),
			status = VALUES(status),
			started_at = VALUES(started_at),
			updated_at = VALUES(updated_at)
	`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(
		ctx,
		query,
		exp.ID,
		exp.CharacterID,
		partyID,
		exp.DungeonID,
		exp.PosX,
		exp.PosY,
		exp.CurrentHP,
		exp.Round,
		exp.AccumulatedExp,
		exp.AccumulatedGold,
		itemsJSON,
		membersJSON,
		string(exp.Status),
		exp.StartedAt,
		now,
	)
	return err
}

func (r *DungeonRepository) DeleteActiveExpedition(ctx context.Context, characterID, expeditionID string) error {
	query := `DELETE FROM dungeon_active_expeditions WHERE character_id = ? AND id = ?`
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, query, characterID, expeditionID)
	return err
}

// FinalizeExpedition claims expedition completion once by history.ID (run ID) inside a SQL transaction.
// Lock order: Rank 2 (characters FOR UPDATE in ascending ID order) -> Rank 8 (dungeon_expedition_history claim) -> Rank 3 (inventory_items via shared post-battle settlement) -> Rank 8 (tavern_character_status on Escape).
// Returns dungeon.ErrExpeditionAlreadyFinished if history.ID has already been settled.
func (r *DungeonRepository) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	character *corecharacter.Character,
	members ...dungeon.ExpeditionMember,
) error {
	charID := history.CharacterID
	if charID == "" || (character != nil && character.ID != "" && character.ID != charID) {
		return dungeon.ErrForbidden
	}
	if history.ID == "" {
		history.ID = id.New()
	}

	roster := members
	if len(roster) == 0 {
		leaderHP := 100
		if character != nil {
			leaderHP = character.Stats.HP
		}
		roster = []dungeon.ExpeditionMember{
			{
				CharacterID: charID,
				CurrentHP:   leaderHP,
			},
		}
	} else {
		hasLeader := false
		seen := make(map[string]bool, len(roster))
		for _, m := range roster {
			if m.CharacterID == "" || seen[m.CharacterID] {
				return dungeon.ErrForbidden
			}
			seen[m.CharacterID] = true
			if m.CharacterID == charID {
				hasLeader = true
			}
		}
		if !hasLeader {
			return dungeon.ErrForbidden
		}
	}

	sortedMembers := append([]dungeon.ExpeditionMember(nil), roster...)
	sort.Slice(sortedMembers, func(i, j int) bool {
		return sortedMembers[i].CharacterID < sortedMembers[j].CharacterID
	})

	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Lock Character (Rank 2) before dependent inventory/depot
		charRepo := &CharacterRepository{db: r.db}
		for _, m := range sortedMembers {
			if _, err := charRepo.FindByIDForUpdate(txCtx, m.CharacterID); err != nil {
				if errors.Is(err, corecharacter.ErrNotFound) {
					return dungeon.ErrCharacterNotFound
				}
				return err
			}
		}

		// 2. Claim expedition finalization durably by run ID and owner
		insertHistoryQuery := `
			INSERT INTO dungeon_expedition_history (
				id, character_id, dungeon_id, outcome,
				exp_reward, gold_reward, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		_, err := executor.ExecContext(
			txCtx,
			insertHistoryQuery,
			history.ID,
			history.CharacterID,
			history.DungeonID,
			string(history.Outcome),
			history.ExpReward,
			history.GoldReward,
			history.CreatedAt,
		)
		if err != nil {
			var mysqlErr *mysql.MySQLError
			if (errors.As(err, &mysqlErr) && mysqlErr.Number == 1062) || strings.Contains(err.Error(), "Duplicate entry") {
				return dungeon.ErrExpeditionAlreadyFinished
			}
			return err
		}

		// 3. Apply post-battle vitality (HP, MP) and item consumptions via shared battle adapter
		ids := make([]string, len(sortedMembers))
		state := corebattle.PartyBattleResult{
			Outcome:       corebattle.OutcomeWin,
			RemainingHP:   make(map[string]int, len(sortedMembers)),
			RemainingMP:   make(map[string]int, len(sortedMembers)),
			ConsumedItems: make(map[string][]corebattle.ConsumedItem, len(sortedMembers)),
		}
		if history.Outcome == dungeon.StatusWipedOut {
			state.Outcome = corebattle.OutcomeDefeat
		}
		for i, m := range sortedMembers {
			ids[i] = m.CharacterID
			state.RemainingHP[m.CharacterID] = m.CurrentHP
			if m.HasCurrentMP() {
				state.RemainingMP[m.CharacterID] = m.CurrentMP
			}
			if len(m.ConsumedItems) > 0 {
				state.ConsumedItems[m.CharacterID] = m.ConsumedItems
			}
		}

		adapter := battle.NewService(
			battle.WithCharacterRepository(charRepo),
			battle.WithInventoryRepository(&InventoryRepository{db: r.db}),
			battle.WithEquipmentRepository(&EquipmentRepository{db: r.db}),
			battle.WithTransactionProvider(NewTransactionProvider(r.db)),
		)
		req := battle.ApplyPostBattleRequest{CharacterIDs: ids, BattleResult: state}
		if history.Outcome == dungeon.StatusEscaped {
			// Escape is nigeru: round×3+1 fatigue and the held item105/106 bonus (_battle.cgi:1208–1220).
			fatigue, err := economy.SafeMultiply(history.ExitRound, 3)
			if err != nil {
				return err
			}
			req.ExitFatigue = fatigue + 1
			req.ExitBonusRounds = history.ExitRound
			// nigeru applies each member's stock first (get_stock, _battle.cgi:1459–1473).
			req.RecipientRewards = make(map[string]corebattle.Reward, len(ids))
			for _, m := range sortedMembers {
				req.RecipientRewards[m.CharacterID] = corebattle.Reward{Experience: m.AccumulatedExp, Currency: m.AccumulatedGold, Crystals: m.AccumulatedCrystals}
			}
		}
		applied, err := adapter.ApplyPostBattleResult(txCtx, req)
		if err != nil {
			return err
		}

		// 4. Save updated characters (Rank 2)
		for _, m := range sortedMembers {
			c := applied.UpdatedCharacters[m.CharacterID]
			if err := updateCharacter(txCtx, executor, c); err != nil {
				return err
			}
			if m.CharacterID == charID && character != nil {
				*character = c
			}
		}

		// nigeru clears is_eat (_battle.cgi:1211).
		if history.Outcome == dungeon.StatusEscaped {
			tavern := &TavernRepository{db: r.db}
			for _, m := range sortedMembers {
				if err := tavern.ResetFullness(txCtx, m.CharacterID); err != nil {
					return err
				}
			}
		}

		// The claimed run has no unsettled credits left (Escape settles them before
		// freezing), so its credit receipts are no longer needed.
		if _, err := executor.ExecContext(txCtx, `DELETE FROM dungeon_defeat_credits WHERE run_id = ?`, history.ID); err != nil {
			return err
		}

		// 5. Delete only this owner's finalized run, preserving any replacement.
		if err := r.DeleteActiveExpedition(txCtx, charID, history.ID); err != nil {
			return err
		}

		return nil
	})
}

func (r *DungeonRepository) GetHistory(ctx context.Context, characterID string, limit int) ([]dungeon.DungeonExpeditionHistory, error) {
	query := `
		SELECT id, character_id, dungeon_id, outcome,
		       exp_reward, gold_reward, created_at
		FROM dungeon_expedition_history
		WHERE character_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, query, characterID, limit)
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
			&outcomeStr,
			&h.ExpReward,
			&h.GoldReward,
			&h.CreatedAt,
		); err != nil {
			return nil, err
		}
		h.Outcome = dungeon.ExpeditionStatus(outcomeStr)
		list = append(list, h)
	}

	return list, rows.Err()
}
