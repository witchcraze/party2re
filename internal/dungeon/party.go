package dungeon

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

var (
	ErrTooManyPartyMembers  = errors.New("dungeon party exceeds maximum 4 members")
	ErrPartyEmpty           = errors.New("dungeon party must have at least 1 member")
	ErrPartyNotFound        = errors.New("party not found")
	ErrNotPartyLeader       = errors.New("only party leader can start party expedition")
	ErrPartyNotRecruiting   = errors.New("party is not currently recruiting")
	ErrPartyNotReady        = errors.New("all party members must be ready before starting")
	ErrDuplicatePartyMember = errors.New("party has duplicate members")
	ErrCharacterUnconscious = errors.New("character is unconscious (HP <= 0)")
)

type ExpeditionMember struct {
	CharacterID string   `json:"character_id"`
	Name        string   `json:"name"`
	JobID       string   `json:"job_id"`
	Level       int      `json:"level"`
	CurrentHP   int      `json:"current_hp"`
	MaxHP       int      `json:"max_hp"`
	ItemIDs     []string `json:"item_ids,omitempty"`
}

// StartPartyExpedition begins a 2D dungeon exploration for an authorized party lobby.
func (s *Service) StartPartyExpedition(
	ctx context.Context,
	leaderCharID string,
	partyID string,
	dungeonID string,
) (*ActiveExpedition, error) {
	if strings.TrimSpace(leaderCharID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(partyID) == "" {
		return nil, ErrPartyNotFound
	}
	if s.partyRepo == nil {
		return nil, errors.New("party repository not configured")
	}

	var exp *ActiveExpedition
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

		partyMembers, err := s.partyRepo.GetMembers(lockedCtx, partyID)
		if err != nil || len(partyMembers) == 0 {
			return ErrPartyEmpty
		}
		if len(partyMembers) > 4 {
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

		for _, mID := range memberIDs {
			existing, err := s.activeStore.GetActiveExpedition(lockedCtx, mID)
			if err != nil {
				return err
			}
			if existing != nil && existing.Status == StatusExploring {
				return ErrActiveExpeditionExists
			}
		}

		if s.activityChecker != nil {
			for _, mID := range memberIDs {
				busy, err := s.activityChecker.HasActiveActivity(lockedCtx, mID)
				if err != nil {
					return err
				}
				if busy {
					return ErrActiveExpeditionExists
				}
			}
		}

		p.Status = party.StatusInProgress
		if err := s.partyRepo.UpdateParty(lockedCtx, p); err != nil {
			return err
		}

		expRes, err := s.startExpeditionRoster(lockedCtx, leaderCharID, memberIDs, dungeonID, partyID, p.Name)
		if err != nil {
			p.Status = party.StatusRecruiting
			//lint:ignore error-swallow compensatory rollback on start failure
			_ = s.partyRepo.UpdateParty(lockedCtx, p)
			return err
		}

		exp = expRes
		return nil
	})
	if err != nil {
		if errors.Is(err, party.ErrPartyNotRecruiting) {
			return nil, ErrPartyNotRecruiting
		}
		return nil, err
	}

	return exp, nil
}

func (s *Service) startExpeditionRoster(
	ctx context.Context,
	leaderCharID string,
	memberIDs []string,
	dungeonID string,
	partyID string,
	partyName string,
) (*ActiveExpedition, error) {
	dungeon, ok := s.dungeonMap[dungeonID]
	if !ok {
		return nil, ErrDungeonNotFound
	}

	if len(dungeon.Floors) == 0 {
		return nil, errors.New("dungeon has no floors")
	}

	firstFloor := dungeon.Floors[0]
	members := make([]ExpeditionMember, 0, len(memberIDs))
	leaderHP := 0

	for _, mID := range memberIDs {
		char, err := s.characterRepo.FindByID(ctx, mID)
		if err != nil {
			return nil, ErrCharacterNotFound
		}

		if char.Stats.HP <= 0 {
			return nil, ErrCharacterUnconscious
		}

		if char.Level < dungeon.MinLevel {
			return nil, ErrLevelRequirementNotMet
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

		var itemIDs []string
		if s.invRepo != nil {
			if inv, err := s.invRepo.FindByCharacterID(ctx, char.ID); err == nil {
				for _, it := range inv.Items {
					itemIDs = append(itemIDs, it.DefinitionID)
				}
			}
		}

		members = append(members, ExpeditionMember{
			CharacterID: char.ID,
			Name:        char.Name,
			JobID:       char.JobID,
			Level:       char.Level,
			CurrentHP:   currHP,
			MaxHP:       maxHP,
			ItemIDs:     itemIDs,
		})
	}

	now := time.Now().UTC()
	exp := ActiveExpedition{
		ID:                id.New(),
		CharacterID:       leaderCharID,
		PartyID:           partyID,
		PartyName:         partyName,
		Members:           members,
		DungeonID:         dungeon.ID,
		CurrentFloor:      1,
		PosX:              firstFloor.StartX,
		PosY:              firstFloor.StartY,
		CurrentHP:         leaderHP,
		TurnsRemaining:    dungeon.MaxTurnsPerFloor,
		AccumulatedExp:    0,
		AccumulatedGold:   0,
		AccumulatedItems:  []string{},
		AccumulatedMedals: 0,
		Status:            StatusExploring,
		StartedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.activeStore.SaveActiveExpedition(ctx, exp); err != nil {
		return nil, err
	}

	return &exp, nil
}
