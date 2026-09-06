package challenge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
)

func (s *Service) StartSession(ctx context.Context, characterID string, tierID string) (*ChallengeSession, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}

	tier, err := s.GetTier(tierID)
	if err != nil {
		return nil, err
	}

	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	level := char.Level
	if level <= 0 {
		level = 1
	}
	if level < tier.MinLevel {
		return nil, ErrLevelTooLow
	}

	existing, err := s.activeStore.GetActiveSession(ctx, characterID)
	if err == nil && existing != nil && existing.Status == StatusActive {
		return nil, ErrActiveSessionExists
	}

	sessionID := id.New()

	maxHP := char.Stats.MaxHP
	if maxHP <= 0 {
		maxHP = char.Stats.HP
	}
	if maxHP <= 0 {
		maxHP = 100
	}

	now := time.Now().UTC()
	session := ChallengeSession{
		ID:                 sessionID,
		CharacterID:        characterID,
		TierID:             tierID,
		CurrentRound:       1,
		CharacterCurrentHP: maxHP,
		AccumulatedExp:     0,
		AccumulatedGold:    0,
		AccumulatedItems:   []string{},
		Status:             StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.activeStore.SaveActiveSession(ctx, session); err != nil {
		return nil, err
	}

	// Persist initial durable session placeholder in SQL repository
	_ = s.repo.SaveSession(ctx, session)

	return &session, nil
}

func (s *Service) AdvanceRound(ctx context.Context, characterID string, sessionID string) (*RoundResult, *ChallengeSession, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, nil, ErrCharacterNotFound
	}

	session, err := s.activeStore.GetActiveSession(ctx, characterID)
	if err != nil {
		return nil, nil, err
	}
	if session == nil || session.ID != sessionID {
		if targetSession, fErr := s.repo.FindSessionByID(ctx, sessionID); fErr == nil && targetSession != nil {
			if targetSession.CharacterID != characterID {
				return nil, nil, ErrForbidden
			}
		}
		return nil, nil, ErrSessionNotFound
	}
	if session.CharacterID != characterID {
		return nil, nil, ErrForbidden
	}
	if session.Status != StatusActive {
		return nil, nil, ErrSessionNotActive
	}

	tier, err := s.GetTier(session.TierID)
	if err != nil {
		return nil, nil, err
	}

	char, err := s.charRepo.FindByID(ctx, session.CharacterID)
	if err != nil {
		return nil, nil, ErrCharacterNotFound
	}

	maxHP := char.Stats.MaxHP
	if maxHP <= 0 {
		maxHP = char.Stats.HP
	}
	if maxHP <= 0 {
		maxHP = 100
	}

	// Scale monster for round
	round := session.CurrentRound
	scale := 1.0 + float64(round-1)*tier.ScaleFactor
	mHP := int(math.Round(float64(tier.BaseMonster.BaseHP) * scale))
	mAtk := int(math.Round(float64(tier.BaseMonster.BaseAttack) * scale))
	mDef := int(math.Round(float64(tier.BaseMonster.BaseDefense) * scale))
	mExp := int(math.Round(float64(tier.BaseMonster.BaseExp) * scale))
	mGold := int(math.Round(float64(tier.BaseMonster.BaseGold) * scale))
	mName := fmt.Sprintf("%s (Wave %d)", tier.BaseMonster.Name, round)

	// Resolve Battle
	charParticipant := corebattle.NewParticipantFromCharacterWithHP(char, session.CharacterCurrentHP)
	monsterParticipant := corebattle.MustNewParticipant(mName, mHP, mAtk, mDef)

	battleReq := corebattle.Request{
		Participants: []corebattle.Participant{charParticipant, monsterParticipant},
	}
	battleRes, err := s.battleEngine.Resolve(battleReq)
	if err != nil {
		return nil, nil, err
	}

	won := battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID == char.ID

	if won {
		// Calculate surviving HP
		survivingHP := 1
		if len(battleRes.Logs) > 0 {
			lastLog := battleRes.Logs[len(battleRes.Logs)-1]
			if hp, ok := lastLog.RemainingHP[char.ID]; ok && hp > 0 {
				survivingHP = hp
			}
		}

		// 20% MaxHP Recovery between rounds
		recovery := int(float64(maxHP) * 0.20)
		survivingHP += recovery
		if survivingHP > maxHP {
			survivingHP = maxHP
		}

		// Milestone item check
		var awardedItem string
		if tier.MilestoneInterval > 0 && round%tier.MilestoneInterval == 0 && len(tier.MilestoneItemPool) > 0 {
			awardedItem = tier.MilestoneItemPool[(round/tier.MilestoneInterval-1)%len(tier.MilestoneItemPool)]
		}

		// Atomically advance round and buffer rewards in Valkey Master (Zero MariaDB SQL writes)
		outcome, err := s.activeStore.AdvanceRound(ctx, characterID, AdvanceRoundParams{
			ExpectedSessionID: sessionID,
			SurvivingHP:       survivingHP,
			ExpDelta:          mExp,
			GoldDelta:         mGold,
			RewardItemID:      awardedItem,
			Now:               time.Now().UTC(),
		})
		if err != nil {
			return nil, nil, err
		}

		return &RoundResult{
			Round:              round,
			MonsterName:        mName,
			BattleResult:       battleRes,
			Won:                true,
			RecoveredHP:        recovery,
			CharacterCurrentHP: survivingHP,
			RoundExp:           mExp,
			RoundGold:          mGold,
			AwardedItem:        awardedItem,
			SessionEnded:       false,
			SessionStatus:      StatusActive,
		}, &outcome.Session, nil
	}

	// Defeat: session terminates
	session.Status = StatusDefeated
	session.CharacterCurrentHP = 0
	session.UpdatedAt = time.Now().UTC()

	// On defeat, half exp/gold awarded, items forfeited
	awardedExp := session.AccumulatedExp / 2
	awardedGold := session.AccumulatedGold / 2
	clearedRounds := round - 1

	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeSession(ctx, *session, awardedExp, awardedGold, nil, clearedRounds); err != nil {
		return nil, nil, err
	}

	// Upon successful MariaDB commit, purge transient buffer from Valkey Master
	_ = s.activeStore.DeleteActiveSession(ctx, characterID)

	return &RoundResult{
		Round:              round,
		MonsterName:        mName,
		BattleResult:       battleRes,
		Won:                false,
		RecoveredHP:        0,
		CharacterCurrentHP: 0,
		RoundExp:           0,
		RoundGold:          0,
		SessionEnded:       true,
		SessionStatus:      StatusDefeated,
	}, session, nil
}

func (s *Service) ExecuteRound(ctx context.Context, sessionID string) (*RoundResult, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	res, _, err := s.AdvanceRound(ctx, session.CharacterID, sessionID)
	return res, err
}

func (s *Service) RetireSession(ctx context.Context, characterID string, sessionID string) (*ChallengeSession, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}

	session, err := s.activeStore.GetActiveSession(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.ID != sessionID {
		if targetSession, fErr := s.repo.FindSessionByID(ctx, sessionID); fErr == nil && targetSession != nil {
			if targetSession.CharacterID != characterID {
				return nil, ErrForbidden
			}
		}
		return nil, ErrSessionNotFound
	}
	if session.CharacterID != characterID {
		return nil, ErrForbidden
	}
	if session.Status != StatusActive {
		return nil, ErrSessionNotActive
	}

	clearedRounds := session.CurrentRound - 1
	session.Status = StatusClaimed
	session.UpdatedAt = time.Now().UTC()

	exp := session.AccumulatedExp
	gold := session.AccumulatedGold
	items := session.AccumulatedItems

	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeSession(ctx, *session, exp, gold, items, clearedRounds); err != nil {
		return nil, err
	}

	// Upon successful MariaDB commit, purge transient buffer from Valkey Master
	_ = s.activeStore.DeleteActiveSession(ctx, characterID)

	return session, nil
}

func (s *Service) Cashout(ctx context.Context, sessionID string) (*CashoutResult, error) {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	retired, err := s.RetireSession(ctx, session.CharacterID, sessionID)
	if err != nil {
		return nil, err
	}
	clearedRounds := retired.CurrentRound - 1
	return &CashoutResult{
		RoundsCleared:  clearedRounds,
		AwardedExp:     retired.AccumulatedExp,
		AwardedGold:    retired.AccumulatedGold,
		AwardedItems:   retired.AccumulatedItems,
		NewRecordRound: clearedRounds,
	}, nil
}

func (s *Service) GetSession(ctx context.Context, characterID string, sessionID string) (*ChallengeSession, error) {
	active, err := s.activeStore.GetActiveSession(ctx, characterID)
	if err == nil && active != nil && active.ID == sessionID {
		return active, nil
	}

	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.CharacterID != characterID {
		return nil, ErrForbidden
	}
	return session, nil
}

func (s *Service) GetActiveSession(ctx context.Context, characterID string) (*ChallengeSession, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, errors.New("character id is required")
	}
	return s.activeStore.GetActiveSession(ctx, characterID)
}
