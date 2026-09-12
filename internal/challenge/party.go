package challenge

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrTooManyPartyMembers = errors.New("challenge party exceeds maximum 4 members")
	ErrPartyEmpty          = errors.New("challenge party must have at least 1 member")
)

// StartPartySession initializes a survival challenge run for up to 4 party members.
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

	if len(memberIDs) > 4 {
		return nil, ErrTooManyPartyMembers
	}

	existing, err := s.activeStore.GetActiveSession(ctx, leaderCharID)
	if err == nil && existing != nil && existing.Status == StatusActive {
		return nil, ErrActiveSessionExists
	}

	members := make([]ChallengeMember, 0, len(memberIDs))
	leaderHP := 0

	for _, mID := range memberIDs {
		char, err := s.charRepo.FindByID(ctx, mID)
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
			OldJobID:           "",
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

	_ = s.repo.SaveSession(ctx, session)

	return &session, nil
}
