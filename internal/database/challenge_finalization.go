package database

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/economy"
)

// FinalizeSession shares settlement for departing and terminal participants.
// Coordinated runs require the accepted revision and the caller's run lock.
// Transaction: RunInTx (reuses an ambient transaction)
// Lock Order: caller's run coordination(0) -> characters(2, ascending IDs) -> inventory_items(3) -> character_depots(5) -> secondary records(8).
func (r *ChallengeRepository) FinalizeSession(ctx context.Context, s challenge.ChallengeSession, expReward int, goldReward int, items []string, newStreak int) error {
	if s.Status != challenge.StatusDefeated && s.Status != challenge.StatusClaimed {
		return challenge.ErrSessionNotActive
	}
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		transition, err := r.FindTransition(txCtx, s.ID)
		if err != nil {
			return err
		}
		if transition.Revision > 0 {
			if txCtx.Value(challengeRunLock{}) != s.ID || transition.Pending || transition.Revision != s.Revision {
				return challenge.ErrRevisionConflict
			}
			return r.settleParticipants(txCtx, s, newStreak)
		}
		executor := ExecutorFromContext(txCtx, r.db)

		stored, err := r.FindSessionByID(txCtx, s.ID)
		if err != nil {
			return err
		}
		if stored.CharacterID != s.CharacterID {
			return challenge.ErrForbidden
		}
		members, err := settlementMembers(*stored, s, expReward, goldReward)
		if err != nil {
			return err
		}
		charRepo := &CharacterRepository{db: r.db}
		for _, m := range members {
			if _, err := charRepo.FindByIDForUpdate(txCtx, m.CharacterID); err != nil {
				return err
			}
		}

		// The start row is durable before Valkey publication; never recreate it here.
		itemsJSON := challenge.EncodeJSON(s.AccumulatedItems)
		var partyID sql.NullString
		if s.PartyID != "" {
			partyID = sql.NullString{String: s.PartyID, Valid: true}
		}
		var membersJSON sql.NullString
		if len(s.Members) > 0 {
			membersJSON = sql.NullString{String: challenge.EncodeJSON(s.Members), Valid: true}
		}
		claimSessionQuery := `
			UPDATE challenge_sessions
			SET party_id = ?, members_json = ?, current_round = ?, character_current_hp = ?,
			    accumulated_exp = ?, accumulated_gold = ?, accumulated_items_json = ?,
			    status = ?, updated_at = ?, write_protocol = write_protocol + 1
			WHERE id = ? AND character_id = ? AND status = 'active'
		`
		result, err := executor.ExecContext(
			txCtx,
			claimSessionQuery,
			partyID,
			membersJSON,
			s.CurrentRound,
			s.CharacterCurrentHP,
			s.AccumulatedExp,
			s.AccumulatedGold,
			itemsJSON,
			string(s.Status),
			s.UpdatedAt,
			s.ID,
			s.CharacterID,
		)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			stored, err := r.FindSessionByID(txCtx, s.ID)
			if err != nil {
				return err
			}
			if stored.CharacterID != s.CharacterID {
				return challenge.ErrForbidden
			}
			return challenge.ErrSessionNotActive
		}

		return r.applySettlement(txCtx, s, members, map[string][]string{s.CharacterID: items}, newStreak, 0, 0)
	})
}

// settlementMembers checks the terminal roster against the durable admission snapshot.
func settlementMembers(stored, terminal challenge.ChallengeSession, exp, gold int) ([]challenge.ChallengeMember, error) {
	expected := stored.Members
	if len(expected) == 0 {
		expected = []challenge.ChallengeMember{{CharacterID: stored.CharacterID}}
	}
	members := append([]challenge.ChallengeMember(nil), terminal.Members...)
	if len(members) == 0 {
		if len(expected) != 1 || expected[0].CharacterID != terminal.CharacterID {
			return nil, challenge.ErrForbidden
		}
		members = []challenge.ChallengeMember{{CharacterID: terminal.CharacterID, CharacterCurrentHP: terminal.CharacterCurrentHP}}
	}
	if len(members) != len(expected) {
		return nil, challenge.ErrForbidden
	}
	allowed := make(map[string]bool, len(expected))
	for _, m := range expected {
		allowed[m.CharacterID] = true
	}
	for i := range members {
		m := &members[i]
		if !allowed[m.CharacterID] {
			return nil, challenge.ErrForbidden
		}
		delete(allowed, m.CharacterID)
		if m.CharacterCurrentMP == nil {
			m.AccumulatedExp, m.AccumulatedGold = exp, gold
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].CharacterID < members[j].CharacterID })
	return members, nil
}

func (r *ChallengeRepository) applySettlement(txCtx context.Context, s challenge.ChallengeSession, members []challenge.ChallengeMember, treasures map[string][]string, newStreak, fatigue, bonusRound int) error {
	executor := ExecutorFromContext(txCtx, r.db)
	charRepo := &CharacterRepository{db: r.db}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.CharacterID
	}
	// Apply shared combat persistence within the same claim transaction.
	state := corebattle.PartyBattleResult{RemainingHP: map[string]int{}, RemainingMP: map[string]int{},
		ConsumedItems: map[string][]corebattle.ConsumedItem{}}
	for _, m := range members {
		state.RemainingHP[m.CharacterID] = m.CharacterCurrentHP
		if m.CharacterCurrentMP != nil {
			state.RemainingMP[m.CharacterID] = *m.CharacterCurrentMP
		}
		state.ConsumedItems[m.CharacterID] = m.ConsumedItems
	}
	adapter := battle.NewService(battle.WithCharacterRepository(charRepo),
		battle.WithInventoryRepository(&InventoryRepository{db: r.db}),
		battle.WithEquipmentRepository(&EquipmentRepository{db: r.db}),
		battle.WithTransactionProvider(NewTransactionProvider(r.db)))
	applied, err := adapter.ApplyPostBattleResult(txCtx, battle.ApplyPostBattleRequest{CharacterIDs: ids, BattleResult: state, ExitFatigue: fatigue})
	if err != nil {
		return err
	}
	for i := range members {
		m := &members[i]
		char := applied.UpdatedCharacters[m.CharacterID]
		if fatigue > 0 {
			if err := r.applyChallengeExitBonus(txCtx, m, bonusRound); err != nil {
				return err
			}
		}
		gold, err := economy.SafeAdd(m.AccumulatedGold, m.ExitBonusGold)
		if err != nil {
			return err
		}
		exp, err := economy.SafeAdd(m.AccumulatedExp, m.ExitBonusExp)
		if err != nil {
			return err
		}
		if gold > 0 {
			if err := char.AddMoney(gold); err != nil {
				return err
			}
		}
		if exp > 0 {
			if _, err := progression.ApplyExperience(&char, exp); err != nil {
				return err
			}
		}
		if err := charRepo.Update(txCtx, char); err != nil {
			return err
		}
	}
	// Shared treasure keeps its designated recipient; it is never cloned.
	for _, member := range members {
		items := treasures[member.CharacterID]
		if len(items) == 0 {
			continue
		}
		rewardInstances := make([]coreitem.Instance, 0, len(items))
		for _, itemDefID := range items {
			if itemDefID == "" {
				continue
			}
			inst, err := coreitem.NewInstance(itemDefID, 1)
			if err != nil {
				return err
			}
			rewardInstances = append(rewardInstances, inst)
		}
		if err := deliverRewardItems(txCtx, r.db, applied.UpdatedCharacters[member.CharacterID], rewardInstances); err != nil {
			return err
		}
	}

	// 4. Update Character Challenge Record
	now := time.Now().UTC()
	attemptsInc := 0
	victoriesInc := 0
	if s.Status == challenge.StatusDefeated || s.Status == challenge.StatusClaimed {
		attemptsInc = 1
	} else {
		victoriesInc = 1
	}

	upsertRecordQuery := `
			INSERT INTO character_challenge_records (
				character_id, tier_id, highest_round, total_attempts,
				total_victories, best_cleared_at
			) VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				best_cleared_at = IF(VALUES(highest_round) > highest_round, VALUES(best_cleared_at), best_cleared_at),
				highest_round = IF(VALUES(highest_round) > highest_round, VALUES(highest_round), highest_round),
				total_attempts = total_attempts + VALUES(total_attempts),
				total_victories = total_victories + VALUES(total_victories)
		`
	for _, m := range members {
		if _, err := executor.ExecContext(txCtx, upsertRecordQuery, m.CharacterID, s.TierID, newStreak, attemptsInc, victoriesInc, now); err != nil {
			return err
		}
	}

	return nil
}
