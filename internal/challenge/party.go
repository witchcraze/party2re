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
	"github.com/witchcraze/party2re/internal/party"
)

var (
	ErrTooManyPartyMembers = errors.New("challenge party exceeds stage maximum participants")
	ErrPartyEmpty          = errors.New("challenge party must have at least 1 member")
	ErrNeedJoinNotMet      = errors.New("character does not meet stage join condition")
	ErrNeedOverLvNotMet    = errors.New("character does not meet reincarnation requirement")
)

// StartPartySession initializes a survival challenge run for up to stage MaxParticipants party members from an authorized party lobby.
func (s *Service) StartPartySession(
	ctx context.Context,
	leaderCharID string,
	partyID string,
	tierID string,
) (*ChallengeSession, error) {
	if strings.TrimSpace(leaderCharID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(partyID) == "" {
		return nil, ErrPartyNotFound
	}
	if s.partyRepo == nil {
		return nil, errors.New("party repository not configured")
	}

	p, err := s.partyRepo.GetParty(ctx, partyID)
	if err != nil {
		return nil, ErrPartyNotFound
	}
	if p.LeaderCharacterID != leaderCharID {
		return nil, ErrNotPartyLeader
	}
	if p.Status != party.StatusRecruiting {
		return nil, ErrPartyNotRecruiting
	}

	tier, err := s.GetTier(tierID)
	if err != nil {
		return nil, err
	}

	partyMembers, err := s.partyRepo.GetMembers(ctx, partyID)
	if err != nil || len(partyMembers) == 0 {
		return nil, ErrPartyEmpty
	}

	maxParts := tier.MaxParticipants
	if maxParts <= 0 {
		maxParts = 4
	}
	if len(partyMembers) > maxParts {
		return nil, ErrTooManyPartyMembers
	}

	seen := make(map[string]bool, len(partyMembers))
	for _, m := range partyMembers {
		if !m.ReadyState {
			return nil, ErrPartyNotReady
		}
		if seen[m.CharacterID] {
			return nil, ErrDuplicatePartyMember
		}
		seen[m.CharacterID] = true
	}

	memberIDs := make([]string, 0, len(partyMembers))
	memberIDs = append(memberIDs, leaderCharID)
	for _, m := range partyMembers {
		if m.CharacterID != leaderCharID {
			memberIDs = append(memberIDs, m.CharacterID)
		}
	}

	for _, mID := range memberIDs {
		existing, err := s.activeStore.GetActiveSession(ctx, mID)
		if err != nil {
			return nil, err
		}
		if existing != nil && existing.Status == StatusActive {
			return nil, ErrActiveSessionExists
		}
	}

	partyName := p.Name
	partyColor := "#FFFFFF"

	session, err := s.startSessionRoster(ctx, leaderCharID, memberIDs, tier, partyID, partyName, partyColor)
	if err != nil {
		return nil, err
	}

	p.Status = party.StatusInProgress
	//lint:ignore error-swallow best-effort party status update
	_ = s.partyRepo.UpdateParty(ctx, p)

	return session, nil
}

func (s *Service) startSessionRoster(
	ctx context.Context,
	leaderCharID string,
	memberIDs []string,
	tier *ChallengeTier,
	partyID string,
	partyName string,
	partyColor string,
) (*ChallengeSession, error) {
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

		if char.Stats.HP <= 0 {
			return nil, ErrCharacterUnconscious
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
		PartyID:            partyID,
		PartyName:          partyName,
		PartyColor:         partyColor,
		Members:            members,
		TierID:             tier.ID,
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
