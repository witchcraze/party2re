package challenge

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

var (
	ErrTooManyPartyMembers  = errors.New("challenge party exceeds stage maximum participants")
	ErrPartyEmpty           = errors.New("challenge party must have at least 1 member")
	ErrNeedJoinNotMet       = errors.New("character does not meet stage join condition")
	ErrNeedOverLvNotMet     = errors.New("character must be at least level 100 for this challenge tier")
	ErrLevelTooHigh         = errors.New("character level must be 99 or below for this challenge tier")
	ErrPendingLevelUp       = errors.New("character has pending level-up and cannot start challenge")
	ErrCharacterUnconscious = errors.New("character is unconscious (HP <= 0) and cannot adventure")
	ErrCharacterExhausted   = errors.New("character is exhausted (tired >= 100) and must rest")
)

// StartPartySession initializes a survival challenge run for up to stage MaxParticipants party members.
func (s *Service) StartPartySession(
	ctx context.Context,
	leaderCharID string,
	memberIDs []string,
	tierID string,
	partyName string,
	partyColor string,
) (*ChallengeSession, error) {
	if s.admission == nil {
		return nil, ErrPartyUnavailable
	}
	return s.startRun(ctx, leaderCharID, func(ctx context.Context, runID string) (*ChallengeSession, error) {
		var session *ChallengeSession
		err := s.admission.AdmitParty(ctx, leaderCharID, "", memberIDs, func(ctx context.Context, partyID string, ids []string) error {
			var err error
			session, err = s.startRoster(ctx, runID, leaderCharID, ids, tierID, partyName, partyColor, partyID)
			return err
		})
		return session, err
	})
}

func (s *Service) startRoster(
	ctx context.Context,
	runID string,
	leaderCharID string,
	memberIDs []string,
	tierID string,
	partyName string,
	partyColor string,
	partyID string,
) (*ChallengeSession, error) {
	if strings.TrimSpace(leaderCharID) == "" {
		return nil, ErrCharacterNotFound
	}

	if len(memberIDs) == 0 {
		return nil, ErrPartyEmpty
	}
	seen := map[string]bool{}
	for _, actor := range memberIDs {
		if actor == "" || seen[actor] {
			return nil, ErrForbidden
		}
		seen[actor] = true
	}
	if !seen[leaderCharID] {
		return nil, ErrForbidden
	}
	tier, err := s.GetTier(tierID)
	if err != nil {
		return nil, err
	}

	maxParts := tier.MaxParticipants
	if maxParts <= 0 {
		maxParts = 4
	}
	if len(memberIDs) > maxParts {
		return nil, ErrTooManyPartyMembers
	}

	members := make([]ChallengeMember, 0, len(memberIDs))
	leaderHP := 0

	for _, mID := range memberIDs {
		existing, err := s.GetActiveSession(ctx, mID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrActiveSessionExists
		}
		char, err := s.charRepo.FindByID(ctx, mID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) || errors.Is(err, ErrCharacterNotFound) {
				return nil, ErrCharacterNotFound
			}
			return nil, err
		}

		if char.Stats.HP <= 0 {
			return nil, ErrCharacterUnconscious
		}
		if char.IsExhausted() {
			return nil, ErrCharacterExhausted
		}

		level := char.Level
		if level <= 0 {
			level = 1
		}
		if level < tier.MinLevel {
			return nil, ErrLevelTooLow
		}
		if level < progression.MaxLevel {
			if nextExp, err := progression.ExperienceForNextLevel(level); err == nil && char.Experience >= nextExp {
				return nil, ErrPendingLevelUp
			}
		}

		if tier.NeedJoin != "" {
			if err := validateNeedJoin(tier.NeedJoin, char); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrNeedJoinNotMet, err)
			}
		}

		if tier.NeedOverLv {
			if level < 100 {
				return nil, ErrNeedOverLvNotMet
			}
		} else if level >= 100 {
			return nil, ErrLevelTooHigh
		}

		maxHP := char.Stats.MaxHP
		if maxHP <= 0 {
			maxHP = char.Stats.HP
		}
		currHP := char.Stats.HP

		if char.ID == leaderCharID || mID == leaderCharID {
			leaderHP = currHP
		}

		icon := "chr/001.gif"

		maxMP := char.Stats.MaxMP
		if maxMP <= 0 {
			maxMP = char.Stats.MP
		}

		members = append(members, ChallengeMember{
			CharacterID:        char.ID,
			CharacterName:      char.Name,
			Icon:               icon,
			JobID:              char.JobID,
			OldJobID:           char.OldJobID,
			Level:              char.Level,
			CharacterCurrentHP: currHP,
			CharacterCurrentMP: &char.Stats.MP,
			MaxHP:              maxHP,
			MaxMP:              maxMP,
			Attack:             char.Stats.Attack,
			Defense:            char.Stats.Defense,
			Agility:            char.Stats.Agility,
		})
	}

	if partyColor == "" {
		partyColor = "#FFFFFF"
	}
	if partyName == "" {
		if len(members) > 0 {
			partyName = members[0].CharacterName
		} else {
			partyName = "Challengers"
		}
	}

	now := time.Now().UTC()
	session := ChallengeSession{
		ID:                 runID,
		Revision:           1,
		PartyID:            partyID,
		CharacterID:        leaderCharID,
		PartyName:          partyName,
		PartyColor:         partyColor,
		Members:            members,
		TierID:             tierID,
		CurrentRound:       1,
		CharacterCurrentHP: leaderHP,
		AccumulatedExp:     0,
		AccumulatedGold:    0,
		AccumulatedItems:   []string{},
		Status:             StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.SaveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("saving challenge session: %w", err)
	}
	if err := s.transitions.EnrollRun(ctx, session); err != nil {
		return nil, err
	}
	result := &CommandResult{ActorID: leaderCharID, Action: "start", ExpectedRevision: 0, Session: session}
	if err := s.transitions.SaveTransition(ctx, Transition{RunID: runID, Revision: 1, Pending: true, Publication: &session, Deadline: now.Add(s.runStore.SessionTTL())}, result); err != nil {
		return nil, err
	}
	return &session, nil
}

func validateNeedJoin(needJoin string, char corecharacter.Character) error {
	needJoin = strings.TrimSpace(needJoin)
	if needJoin == "" || needJoin == "0" {
		return nil
	}
	parts := strings.Split(needJoin, "_")
	if len(parts) != 3 {
		return nil
	}
	key := parts[0]
	val, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}
	uo := parts[2] // "u" (< val) or "o" (>= val)

	switch key {
	case "hp":
		if uo == "u" && char.Stats.MaxHP >= val {
			return fmt.Errorf("HP must be under %d", val)
		}
		if uo == "o" && char.Stats.MaxHP < val {
			return fmt.Errorf("HP must be at least %d", val)
		}
	case "joblv":
		if uo == "u" && char.JobLevel >= val {
			return fmt.Errorf("Job level must be under %d", val)
		}
		if uo == "o" && char.JobLevel < val {
			return fmt.Errorf("Job level must be at least %d", val)
		}
	}
	return nil
}
