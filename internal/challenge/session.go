package challenge

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) StartSession(ctx context.Context, characterID string, tierID string) (*ChallengeSession, error) {
	return s.StartPartySession(ctx, characterID, []string{characterID}, tierID, "", "#FFFFFF")
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

	monsterParticipant := corebattle.MustNewParticipant(mName, mHP, mAtk, mDef)
	var battleRes corebattle.Result
	var won bool
	var remainingHPs map[string]int

	if pResolver, ok := s.battleEngine.(corebattle.PartyBattleResolver); ok && len(session.Members) > 1 {
		allies := make([]corebattle.Participant, 0, len(session.Members))
		for _, m := range session.Members {
			if m.CharacterCurrentHP > 0 {
				mChar := corecharacter.Character{
					ID:    m.CharacterID,
					Name:  m.CharacterName,
					JobID: m.JobID,
					Level: m.Level,
					Stats: corecharacter.Stats{
						HP:      m.CharacterCurrentHP,
						MaxHP:   m.MaxHP,
						MP:      m.MaxMP,
						MaxMP:   m.MaxMP,
						Attack:  m.Attack,
						Defense: m.Defense,
						Agility: m.Agility,
					},
				}
				allies = append(allies, corebattle.NewParticipantFromCharacterWithHP(mChar, m.CharacterCurrentHP))
			}
		}
		if len(allies) == 0 {
			allies = append(allies, corebattle.NewParticipantFromCharacterWithHP(char, session.CharacterCurrentHP))
		}
		pRes, pErr := pResolver.ResolvePartyBattle(corebattle.PartyBattleRequest{
			Allies:  allies,
			Enemies: []corebattle.Participant{monsterParticipant},
		})
		if pErr != nil {
			return nil, nil, pErr
		}
		won = pRes.Outcome == corebattle.OutcomeWin && pRes.WinnerSide == "allies"
		remainingHPs = pRes.RemainingHP
		battleRes = corebattle.Result{
			Outcome:  pRes.Outcome,
			WinnerID: char.ID,
			LoserID:  monsterParticipant.ID,
			Turns:    pRes.Turns,
			Logs:     pRes.Logs,
		}
	} else {
		charParticipant := corebattle.NewParticipantFromCharacterWithHP(char, session.CharacterCurrentHP)
		battleReq := corebattle.Request{
			Participants: []corebattle.Participant{charParticipant, monsterParticipant},
		}
		var bErr error
		battleRes, bErr = s.battleEngine.Resolve(battleReq)
		if bErr != nil {
			return nil, nil, bErr
		}
		won = battleRes.Outcome == corebattle.OutcomeWin && battleRes.WinnerID != monsterParticipant.ID
		if len(battleRes.Logs) > 0 {
			remainingHPs = battleRes.Logs[len(battleRes.Logs)-1].RemainingHP
		}
	}

	if won {
		// Update member HPs and 20% recovery
		leaderSurvivingHP := 1
		for i := range session.Members {
			m := &session.Members[i]
			if m.CharacterCurrentHP > 0 {
				remHP := 0
				if hp, ok := remainingHPs[m.CharacterID]; ok {
					remHP = hp
				}
				if remHP > 0 {
					recovery := int(float64(m.MaxHP) * 0.20)
					m.CharacterCurrentHP = remHP + recovery
					if m.CharacterCurrentHP > m.MaxHP {
						m.CharacterCurrentHP = m.MaxHP
					}
				} else {
					m.CharacterCurrentHP = 0
				}
			}
			if m.CharacterID == char.ID && m.CharacterCurrentHP > 0 {
				leaderSurvivingHP = m.CharacterCurrentHP
			}
		}

		if len(session.Members) == 0 {
			survivingHP := 1
			if hp, ok := remainingHPs[char.ID]; ok && hp > 0 {
				survivingHP = hp
			}
			recovery := int(float64(maxHP) * 0.20)
			survivingHP += recovery
			if survivingHP > maxHP {
				survivingHP = maxHP
			}
			leaderSurvivingHP = survivingHP
		}

		// Milestone item check
		var awardedItem string
		if tier.MilestoneInterval > 0 && round%tier.MilestoneInterval == 0 && len(tier.MilestoneItemPool) > 0 {
			awardedItem = tier.MilestoneItemPool[(round/tier.MilestoneInterval-1)%len(tier.MilestoneItemPool)]
		}

		// Update Hall of Fame if round > highestRound for tier
		hof, _ := s.repo.GetHallOfFame(ctx, session.TierID)
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
			if len(hofMembers) == 0 {
				hofMembers = []HallOfFameMember{
					{
						CharacterID:   char.ID,
						CharacterName: char.Name,
						Icon:          "chr/001.gif",
						JobID:         char.JobID,
						HP:            char.Stats.MaxHP,
						MP:            char.Stats.MaxMP,
						Attack:        char.Stats.Attack,
						Defense:       char.Stats.Defense,
						Agility:       char.Stats.Agility,
					},
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
			_ = s.repo.SaveHallOfFame(ctx, HallOfFameEntry{
				TierID:       session.TierID,
				HighestRound: round,
				PartyName:    pName,
				PartyColor:   pColor,
				ClearedAt:    time.Now().UTC(),
				Members:      hofMembers,
			})
		}

		// Atomically advance round and buffer rewards in Valkey Master
		outcome, err := s.activeStore.AdvanceRound(ctx, characterID, AdvanceRoundParams{
			ExpectedSessionID: sessionID,
			SurvivingHP:       leaderSurvivingHP,
			ExpDelta:          mExp,
			GoldDelta:         mGold,
			RewardItemID:      awardedItem,
			Now:               time.Now().UTC(),
		})
		if err != nil {
			return nil, nil, err
		}

		outcome.Session.PartyID = session.PartyID
		outcome.Session.Members = session.Members
		outcome.Session.PartyName = session.PartyName
		outcome.Session.PartyColor = session.PartyColor
		_ = s.activeStore.SaveActiveSession(ctx, outcome.Session)

		return &RoundResult{
			Round:              round,
			MonsterName:        mName,
			BattleResult:       battleRes,
			Won:                true,
			RecoveredHP:        int(float64(maxHP) * 0.20),
			CharacterCurrentHP: leaderSurvivingHP,
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
