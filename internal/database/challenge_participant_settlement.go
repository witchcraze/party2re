package database

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/economy"
)

// settleParticipants runs inside the accepted command's run lock and transaction.
func (r *ChallengeRepository) settleParticipants(ctx context.Context, s challenge.ChallengeSession, cleared int) error {
	e := ExecutorFromContext(ctx, r.db)
	seen := map[string]bool{}
	members := s.Members
	for _, m := range members {
		if seen[m.CharacterID] {
			return challenge.ErrForbidden
		}
		seen[m.CharacterID] = true
		var settled bool
		if err := e.QueryRowContext(ctx, `SELECT settled FROM challenge_participants WHERE run_id=? AND character_id=?`, s.ID, m.CharacterID).Scan(&settled); err != nil {
			return err
		}
		if settled {
			return challenge.ErrSessionNotActive
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].CharacterID < members[j].CharacterID })
	chars := &CharacterRepository{db: r.db}
	for _, m := range members {
		if _, err := chars.FindByIDForUpdate(ctx, m.CharacterID); err != nil {
			return err
		}
	}
	bonusRound := cleared
	if s.Status == challenge.StatusDefeated {
		bonusRound = s.CurrentRound
	}
	fatigue, err := economy.SafeMultiply(bonusRound, 3)
	if err != nil {
		return err
	}
	fatigue, err = economy.SafeAdd(fatigue, 1)
	if err != nil {
		return err
	}
	treasures := map[string][]string{}
	for _, award := range s.TreasureAwards {
		if !award.Delivered && seen[award.RecipientID] {
			treasures[award.RecipientID] = append(treasures[award.RecipientID], award.ItemID)
		}
	}
	if err := r.applySettlement(ctx, s, members, treasures, cleared, fatigue, bonusRound); err != nil {
		return err
	}
	for _, m := range members {
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		res, err := e.ExecContext(ctx, `UPDATE challenge_participants SET settled=TRUE,settlement_json=? WHERE run_id=? AND character_id=? AND settled=FALSE`, data, s.ID, m.CharacterID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return challenge.ErrSessionNotActive
		}
	}
	return nil
}

// Read the persisted equipment after shared consumption has removed used accessories.
func (r *ChallengeRepository) applyChallengeExitBonus(ctx context.Context, m *challenge.ChallengeMember, round int) error {
	var bonus string
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT COALESCE(MAX(i.definition_id),'') FROM equipment_slots e
        JOIN inventory_items i ON i.id=e.instance_id AND i.character_id=e.character_id
        WHERE e.character_id=? AND e.slot='accessory' AND i.definition_id IN ('item-105','item-106')`, m.CharacterID).Scan(&bonus)
	if err != nil {
		return err
	}
	draw := 0.0
	if bonus == "item-106" {
		draw = random.Float64()
	}
	m.ExitBonusExp, m.ExitBonusGold, err = challenge.ExitBonus(bonus, round, draw)
	return err
}
