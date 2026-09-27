package challenge

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrTooManyPartyMembers = errors.New("challenge party exceeds stage maximum participants")
	ErrPartyEmpty          = errors.New("challenge party must have at least 1 member")
	ErrNeedJoinNotMet      = errors.New("character does not meet stage join condition")
	ErrNeedOverLvNotMet    = errors.New("character does not meet reincarnation requirement")
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
	if strings.TrimSpace(leaderCharID) == "" {
		return nil, ErrCharacterNotFound
	}

	tier, err := s.GetTier(tierID)
	if err != nil {
		return nil, err
	}

	if len(memberIDs) == 0 {
		memberIDs = []string{leaderCharID}
	}

	// Ensure leader is first
	hasLeader := false
	for _, mID := range memberIDs {
		if mID == leaderCharID {
			hasLeader = true
			break
		}
	}
	if !hasLeader {
		memberIDs = append([]string{leaderCharID}, memberIDs...)
	}

	maxParts := tier.MaxParticipants
	if maxParts <= 0 {
		maxParts = 4
	}
	if len(memberIDs) > maxParts {
		return nil, ErrTooManyPartyMembers
	}

	existing, err := s.activeStore.GetActiveSession(ctx, leaderCharID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status == StatusActive {
		return nil, ErrActiveSessionExists
	}

	members := make([]ChallengeMember, 0, len(memberIDs))
	leaderHP := 0

	for _, mID := range memberIDs {
		char, err := s.charRepo.FindByID(ctx, mID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) || errors.Is(err, ErrCharacterNotFound) {
				return nil, ErrCharacterNotFound
			}
			return nil, err
		}

		level := char.Level
		if level <= 0 {
			level = 1
		}
		if level < tier.MinLevel {
			return nil, ErrLevelTooLow
		}

		if tier.NeedJoin != "" {
			if err := validateNeedJoin(tier.NeedJoin, char); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrNeedJoinNotMet, err)
			}
		}

		if tier.NeedOverLv && char.OldJobID == "" {
			return nil, ErrNeedOverLvNotMet
		}

		maxHP := char.Stats.MaxHP
		if maxHP <= 0 {
			maxHP = char.Stats.HP
		}
		if maxHP <= 0 {
			maxHP = 100
		}
		currHP := char.Stats.HP
		if currHP <= 0 {
			currHP = maxHP
		}

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
		ID:                 id.New(),
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

	if err := s.activeStore.SaveActiveSession(ctx, session); err != nil {
		return nil, err
	}

	if err := s.repo.SaveSession(ctx, session); err != nil {
		return nil, fmt.Errorf("saving challenge session: %w", err)
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
