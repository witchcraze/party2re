package challenge

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

func (s *Service) withCharacterLock(ctx context.Context, characterID string, fn func(ctx context.Context) error) error {
	if s.partyRepo == nil || characterID == "" {
		return fn(ctx)
	}
	return s.partyRepo.WithPartyLock(ctx, "char:"+characterID, fn)
}

func (s *Service) withRosterLocks(ctx context.Context, memberIDs []string, fn func(ctx context.Context) error) error {
	if s.partyRepo == nil || len(memberIDs) == 0 {
		return fn(ctx)
	}
	seen := make(map[string]bool, len(memberIDs))
	sorted := make([]string, 0, len(memberIDs))
	for _, id := range memberIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			sorted = append(sorted, id)
		}
	}
	sort.Strings(sorted)

	var acquireNext func(idx int, curCtx context.Context) error
	acquireNext = func(idx int, curCtx context.Context) error {
		if idx >= len(sorted) {
			return fn(curCtx)
		}
		return s.partyRepo.WithPartyLock(curCtx, "char:"+sorted[idx], func(nextCtx context.Context) error {
			return acquireNext(idx+1, nextCtx)
		})
	}
	return acquireNext(0, ctx)
}

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

	var session *ChallengeSession
	err := s.partyRepo.WithPartyLock(ctx, partyID, func(lockedCtx context.Context) error {
		p, err := s.partyRepo.GetParty(lockedCtx, partyID)
		if err != nil {
			return ErrPartyNotFound
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}
		if p.Status != party.StatusRecruiting {
			return ErrPartyNotRecruiting
		}

		tier, err := s.GetTier(tierID)
		if err != nil {
			return err
		}

		partyMembers, err := s.partyRepo.GetMembers(lockedCtx, partyID)
		if err != nil || len(partyMembers) == 0 {
			return ErrPartyEmpty
		}

		maxParts := tier.MaxParticipants
		if maxParts <= 0 {
			maxParts = 4
		}
		if len(partyMembers) > maxParts {
			return ErrTooManyPartyMembers
		}

		seen := make(map[string]bool, len(partyMembers))
		for _, m := range partyMembers {
			if !m.ReadyState {
				return ErrPartyNotReady
			}
			if seen[m.CharacterID] {
				return ErrDuplicatePartyMember
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

		err = s.withRosterLocks(lockedCtx, memberIDs, func(rosterLockedCtx context.Context) error {
			for _, mID := range memberIDs {
				existing, err := s.activeStore.GetActiveSession(rosterLockedCtx, mID)
				if err != nil {
					return err
				}
				if existing != nil {
					return ErrActiveSessionExists
				}
			}

			if s.activityChecker != nil {
				for _, mID := range memberIDs {
					busy, err := s.activityChecker.HasActiveActivity(rosterLockedCtx, mID)
					if err != nil {
						return err
					}
					if busy {
						return ErrActiveSessionExists
					}
				}
			}

			partyName := p.Name
			partyColor := "#FFFFFF"

			p.Status = party.StatusInProgress
			if err := s.partyRepo.UpdateParty(rosterLockedCtx, p); err != nil {
				return err
			}

			sess, err := s.startSessionRoster(rosterLockedCtx, leaderCharID, memberIDs, tier, partyID, partyName, partyColor)
			if err != nil {
				p.Status = party.StatusRecruiting
				//lint:ignore error-swallow compensatory rollback on start failure
				_ = s.partyRepo.UpdateParty(rosterLockedCtx, p)
				return err
			}

			session = sess
			return nil
		})
		if err != nil {
			if errors.Is(err, party.ErrPartyNotRecruiting) {
				return ErrActiveSessionExists
			}
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, party.ErrPartyNotRecruiting) {
			return nil, ErrPartyNotRecruiting
		}
		return nil, err
	}

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
		//lint:ignore error-swallow best-effort active session cleanup on persistence failure
		_ = s.activeStore.DeleteActiveSession(ctx, leaderCharID)
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
