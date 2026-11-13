package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
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

func (r *DungeonRepository) GetActiveExpedition(ctx context.Context, characterID string) (*dungeon.ActiveExpedition, error) {
	query := `
		SELECT id, character_id, party_id, dungeon_id, pos_x, pos_y,
		       current_hp, turns_remaining, accumulated_exp, accumulated_gold,
		       accumulated_items_json, members_json, status, started_at, updated_at
		FROM dungeon_active_expeditions
		WHERE character_id = ?
	`
	var exp dungeon.ActiveExpedition
	var itemsJSON string
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
		&exp.TurnsRemaining,
		&exp.AccumulatedExp,
		&exp.AccumulatedGold,
		&itemsJSON,
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
	exp.AccumulatedItems = dungeon.DecodeItems(itemsJSON)

	return &exp, nil
}

func (r *DungeonRepository) SaveActiveExpedition(ctx context.Context, exp dungeon.ActiveExpedition) error {
	itemsJSON := dungeon.EncodeItems(exp.AccumulatedItems)
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
		exp.TurnsRemaining,
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
// Lock order: Rank 2 (characters FOR UPDATE in ascending ID order) -> Rank 8 (dungeon_expedition_history claim) -> Rank 3 (inventory_items) -> Rank 5 (character_depots).
// Returns dungeon.ErrExpeditionAlreadyFinished if history.ID has already been settled.
func (r *DungeonRepository) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
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
				exp_reward, gold_reward, items_reward_count, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
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
			history.ItemsRewardCount,
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
		applied, err := adapter.ApplyPostBattleResult(txCtx, battle.ApplyPostBattleRequest{
			CharacterIDs: ids,
			BattleResult: state,
		})
		if err != nil {
			return err
		}

		// 4. Apply rewards onto updated characters and save (Rank 2)
		totalMemberRewards := 0
		for _, m := range sortedMembers {
			totalMemberRewards += m.AccumulatedExp + m.AccumulatedGold
		}
		legacyFallback := totalMemberRewards == 0 && (history.ExpReward > 0 || history.GoldReward > 0)

		for i := range sortedMembers {
			m := &sortedMembers[i]
			c := applied.UpdatedCharacters[m.CharacterID]
			if history.Outcome == dungeon.StatusEscaped {
				earnedExp := m.AccumulatedExp
				earnedGold := m.AccumulatedGold
				if legacyFallback {
					earnedExp = history.ExpReward
					earnedGold = history.GoldReward
				}
				if earnedExp > 0 {
					if _, err := progression.ApplyExperience(&c, earnedExp); err != nil {
						return fmt.Errorf("applying experience: %w", err)
					}
				}
				if earnedGold > 0 {
					if err := c.AddMoney(earnedGold); err != nil {
						return fmt.Errorf("adding gold: %w", err)
					}
				}
			}
			if err := updateCharacter(txCtx, executor, c); err != nil {
				return err
			}
			if m.CharacterID == charID && character != nil {
				*character = c
			}
		}

		// 5. Deliver reward item instances (Rank 3 Inventory -> Rank 5 Depot)
		if len(rewardItems) > 0 {
			for i := range rewardItems {
				if rewardItems[i].ID == "" {
					rewardItems[i].ID = id.New()
				}
			}
			leaderChar := applied.UpdatedCharacters[charID]
			if err := deliverRewardItems(txCtx, r.db, leaderChar, rewardItems); err != nil {
				return err
			}
		}

		// 6. Delete only this owner's finalized run, preserving any replacement.
		if err := r.DeleteActiveExpedition(txCtx, charID, history.ID); err != nil {
			return err
		}

		return nil
	})
}

func (r *DungeonRepository) GetHistory(ctx context.Context, characterID string, limit int) ([]dungeon.DungeonExpeditionHistory, error) {
	query := `
		SELECT id, character_id, dungeon_id, outcome,
		       exp_reward, gold_reward, items_reward_count, created_at
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
