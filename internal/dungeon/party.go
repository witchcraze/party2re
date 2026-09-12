package dungeon

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrTooManyPartyMembers = errors.New("dungeon party exceeds maximum 4 members")
	ErrPartyEmpty          = errors.New("dungeon party must have at least 1 member")
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

// StartPartyExpedition begins a 2D dungeon exploration for up to 4 party members.
func (s *Service) StartPartyExpedition(
	ctx context.Context,
	leaderCharID string,
	memberIDs []string,
	dungeonID string,
	partyID string,
) (*ActiveExpedition, error) {
	if strings.TrimSpace(leaderCharID) == "" {
		return nil, ErrCharacterNotFound
	}

	dungeon, ok := s.dungeonMap[dungeonID]
	if !ok {
		return nil, ErrDungeonNotFound
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

	existing, err := s.activeStore.GetActiveExpedition(ctx, leaderCharID)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status == StatusExploring {
		return nil, ErrActiveExpeditionExists
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
