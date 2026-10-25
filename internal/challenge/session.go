package challenge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) StartSession(ctx context.Context, characterID string, tierID string) (*ChallengeSession, error) {
	return s.startRun(ctx, characterID, func(ctx context.Context, runID string) (*ChallengeSession, error) {
		var result *ChallengeSession
		start := func(ctx context.Context) error {
			var err error
			result, err = s.startRoster(ctx, runID, characterID, []string{characterID}, tierID, "", "#FFFFFF", "")
			return err
		}
		var err error
		if s.admission != nil {
			err = s.admission.AdmitSolo(ctx, characterID, start)
		} else {
			err = start(ctx)
		}
		return result, err
	})
}

func (s *Service) AdvanceRound(ctx context.Context, characterID string, sessionID string, expectedRevision int) (*RoundResult, *ChallengeSession, error) {
	result, err := s.mutate(ctx, characterID, sessionID, expectedRevision, "advance", func(ctx context.Context, run ChallengeSession) (*CommandResult, []string, error) {
		if run.CurrentRound == 1 && run.CharacterID != characterID {
			return nil, nil, ErrForbidden
		}
		return s.computeRound(ctx, &run)
	})
	if result == nil {
		return nil, nil, err
	}
	return result.Round, &result.Session, err
}

func (s *Service) rollTreasureThreshold(tier *ChallengeTier) int {
	if tier.TreasureRoundMax > tier.TreasureRound {
		return tier.TreasureRound + s.rng.Intn(tier.TreasureRoundMax-tier.TreasureRound+1)
	}
	return tier.TreasureRound
}

func (s *Service) updateHallOfFameIfNeeded(ctx context.Context, session *ChallengeSession, char corecharacter.Character, round int) error {
	hof, err := s.repo.GetHallOfFame(ctx, session.TierID)
	if err != nil {
		return fmt.Errorf("getting hall of fame: %w", err)
	}
	if hof == nil || round > hof.HighestRound {
		hofMembers := make([]HallOfFameMember, len(session.Members))
		for i, m := range session.Members {
			icon := m.Icon
			if m.CharacterCurrentHP <= 0 {
				icon = "chr/099.gif"
			}
			hofMembers[i] = HallOfFameMember{
				CharacterID:   m.CharacterID,
				CharacterName: m.CharacterName,
				Icon:          icon,
				JobID:         m.JobID,
				OldJobID:      m.OldJobID,
				HP:            m.MaxHP,
				MP:            m.MaxMP,
				Attack:        m.Attack,
				Defense:       m.Defense,
				Agility:       m.Agility,
			}
		}
		pName := session.PartyName
		if pName == "" {
			pName = char.Name
		}
		pColor := session.PartyColor
		if pColor == "" {
			pColor = "#FFFFFF"
		}
		if err := s.repo.SaveHallOfFame(ctx, HallOfFameEntry{
			TierID:       session.TierID,
			HighestRound: round,
			PartyName:    pName,
			PartyColor:   pColor,
			ClearedAt:    time.Now().UTC(),
			Members:      hofMembers,
		}); err != nil {
			return fmt.Errorf("saving hall of fame: %w", err)
		}
	}
	return nil
}

func (s *Service) computeRound(ctx context.Context, session *ChallengeSession) (*CommandResult, []string, error) {
	tier, err := s.GetTier(session.TierID)
	if err != nil {
		return nil, nil, err
	}

	char, err := s.charRepo.FindByID(ctx, session.CharacterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) || errors.Is(err, ErrCharacterNotFound) {
			return nil, nil, ErrCharacterNotFound
		}
		return nil, nil, err
	}

	round := session.CurrentRound
	treasureThreshold := s.rollTreasureThreshold(tier)

	// Authentic Treasure Room branch (vs_challenge.cgi:64-66):
	// When no treasure has been obtained yet, round >= threshold, and 10% roll succeeds,
	// add_treasure() replaces add_monster() with no combat.
	if len(session.AccumulatedItems) == 0 && len(tier.TreasureItemPool) > 0 && round >= treasureThreshold && s.rng.Float64() < 0.10 {
		awardedItem := tier.TreasureItemPool[s.rng.Intn(len(tier.TreasureItemPool))]

		if err := s.updateHallOfFameIfNeeded(ctx, session, char, round); err != nil {
			return nil, nil, err
		}

		session.CurrentRound++
		session.AccumulatedItems = append(session.AccumulatedItems, awardedItem)
		session.TreasureAwards = append(session.TreasureAwards, TreasureAward{ItemID: awardedItem, RecipientID: session.CharacterID})

		return &CommandResult{Round: &RoundResult{
			Round:              round,
			MonsterName:        "宝箱",
			BattleResult:       corebattle.Result{Outcome: corebattle.OutcomeWin, WinnerID: char.ID},
			Won:                true,
			RecoveredHP:        0,
			CharacterCurrentHP: session.CharacterCurrentHP,
			RoundExp:           0,
			RoundGold:          0,
			AwardedItem:        awardedItem,
			SessionEnded:       false,
			SessionStatus:      StatusActive,
		}, Session: *session}, nil, nil
	}

	// Monster encounter branch (vs_challenge.cgi:68, _npc_action.cgi:add_monster):
	// Count c = int(rand(2.5 + @partys * 0.6)), spawning c + 1 monsters.
	partyCount := len(session.Members)
	if partyCount <= 0 {
		partyCount = 1
	}
	c := int(s.rng.Float64() * (2.5 + float64(partyCount)*0.6))
	monsterCount := c + 1

	scale := 1.0 + 0.10*float64(round)
	enemies := make([]corebattle.Participant, 0, monsterCount)
	var mExp, mGold int
	var firstMonsterName string

	for i := 0; i < monsterCount; i++ {
		var baseM ChallengeMonster
		if len(tier.MonsterPool) > 0 {
			baseM = tier.MonsterPool[s.rng.Intn(len(tier.MonsterPool))]
		} else {
			baseM = tier.BaseMonster
		}
		if i == 0 {
			firstMonsterName = fmt.Sprintf("%s (Round %d)", baseM.Name, round)
		}

		statFactor := func() float64 {
			return (0.9 + s.rng.Float64()*0.3 + float64(partyCount-2)*0.1) * scale
		}
		mHP := max(1, int(float64(baseM.BaseHP)*statFactor()))
		mAtk := max(0, int(float64(baseM.BaseAttack)*statFactor()))
		mDef := max(0, int(float64(baseM.BaseDefense)*statFactor()))
		mAg := max(0, int(float64(baseM.BaseAgility)*statFactor()))

		suffix := rune('A' + (i % 26))
		enemyID := fmt.Sprintf("%s-%d-%c", baseM.Name, round, suffix)
		p := corebattle.MustNewParticipant(enemyID, mHP, mAtk, mDef)
		p.Name = fmt.Sprintf("%s%c", baseM.Name, suffix)
		p.MaxHP = mHP
		p.Agility = mAg
		p.ExperienceReward = baseM.BaseExp
		p.GoldReward = baseM.BaseGold
		enemies = append(enemies, p)

		mExp += baseM.BaseExp
		mGold += baseM.BaseGold
	}

	battleRes, won, err := s.resolveRound(ctx, session, char, enemies...)
	if err != nil {
		return nil, nil, err
	}
	leaderSurvivingHP := session.CharacterCurrentHP

	if won {
		for i := range session.Members {
			m := &session.Members[i]
			if m.CharacterCurrentHP > 0 {
				m.AccumulatedExp += mExp
				m.AccumulatedGold += mGold
			}
		}

		if err := s.updateHallOfFameIfNeeded(ctx, session, char, round); err != nil {
			return nil, nil, err
		}

		session.CurrentRound++
		session.AccumulatedExp += mExp
		session.AccumulatedGold += mGold

		return &CommandResult{Round: &RoundResult{
			Round:              round,
			MonsterName:        firstMonsterName,
			BattleResult:       battleRes,
			Won:                true,
			RecoveredHP:        0,
			CharacterCurrentHP: leaderSurvivingHP,
			RoundExp:           mExp,
			RoundGold:          mGold,
			SessionEnded:       false,
			SessionStatus:      StatusActive,
		}, Session: *session}, nil, nil
	}

	// Defeat: session terminates
	session.Status = StatusDefeated
	session.CharacterCurrentHP = 0
	session.UpdatedAt = time.Now().UTC()

	// On defeat, award 100% of accumulated EXP, Gold, and items
	awardedExp := session.AccumulatedExp
	awardedGold := session.AccumulatedGold
	awardedItems := session.AccumulatedItems
	clearedRounds := round - 1

	// Two-Phase Settlement: commit durable state to MariaDB first
	if err := s.repo.FinalizeSession(ctx, *session, awardedExp, awardedGold, awardedItems, clearedRounds); err != nil {
		return nil, nil, err
	}

	removed := make([]string, len(session.Members))
	for i, m := range session.Members {
		removed[i] = m.CharacterID
		if s.fullness != nil {
			if err := s.fullness.ResetFullness(ctx, m.CharacterID); err != nil {
				return nil, nil, err
			}
		}
	}
	for i := range session.TreasureAwards {
		session.TreasureAwards[i].Delivered = true
	}

	return &CommandResult{Round: &RoundResult{
		Round:              round,
		MonsterName:        firstMonsterName,
		BattleResult:       battleRes,
		Won:                false,
		RecoveredHP:        0,
		CharacterCurrentHP: 0,
		RoundExp:           0,
		RoundGold:          0,
		SessionEnded:       true,
		SessionStatus:      StatusDefeated,
	}, Session: *session, Settled: session.Members}, removed, nil
}

func (s *Service) GetSession(ctx context.Context, characterID string, sessionID string) (*ChallengeSession, error) {
	active, err := s.GetActiveSession(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if active != nil && active.ID == sessionID {
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
	claim, err := s.transitions.FindClaim(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if claim != "" {
		t, err := s.transitions.FindTransition(ctx, claim)
		if err != nil {
			return nil, err
		}
		if !t.Pending {
			run, err := s.runStore.ReadRun(ctx, claim)
			if err != nil {
				return nil, err
			}
			if run != nil && run.ContainsCharacter(characterID) {
				return run, nil
			}
			if t.Revision == 0 {
				admission, err := s.repo.FindSessionByID(ctx, claim)
				if err != nil {
					return nil, err
				}
				old, _, err := s.runStore.ReadLegacyRun(ctx, admission.CharacterID)
				if err != nil && !errors.Is(err, ErrRecoveryRequired) {
					return nil, err
				}
				if old != nil && old.ID == claim && old.ContainsCharacter(characterID) {
					return old, nil
				}
			}
		}
		return &ChallengeSession{ID: claim, Revision: t.Revision, Status: StatusActive, RecoveryPending: true, Members: []ChallengeMember{{CharacterID: characterID}}}, nil
	}
	run, err := s.activeStore.GetActiveSession(ctx, characterID)
	if err != nil {
		return nil, err
	}
	if run != nil && !run.ContainsCharacter(characterID) {
		return nil, ErrForbidden
	}
	return run, nil
}

func buildFallbackParticipantWithHP(char corecharacter.Character, currentHP int) corebattle.Participant {
	hp := currentHP
	if hp <= 0 {
		hp = char.Stats.HP
		if hp <= 0 && char.Stats.MaxHP > 0 {
			hp = char.Stats.MaxHP
		}
	}
	p := corebattle.MustNewParticipant(char.ID, hp, char.Stats.Attack, char.Stats.Defense)
	p.Name = char.Name
	p.MaxHP = char.Stats.MaxHP
	p.MP = char.Stats.MP
	p.MaxMP = char.Stats.MaxMP
	p.Agility = char.Stats.Agility
	return p
}
