package dungeon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

var (
	ErrTooManyPartyMembers = errors.New("dungeon party exceeds maximum 4 members")
	ErrPartyEmpty          = errors.New("dungeon party must have at least 1 member")
)

type ExpeditionMember struct {
	CharacterID     string                    `json:"character_id"`
	Name            string                    `json:"name"`
	JobID           string                    `json:"job_id"`
	Level           int                       `json:"level"`
	CurrentHP       int                       `json:"current_hp"`
	MaxHP           int                       `json:"max_hp"`
	CurrentMP       int                       `json:"current_mp"`
	MaxMP           int                       `json:"max_mp"`
	AccumulatedExp  int                       `json:"accumulated_exp"`
	AccumulatedGold int                       `json:"accumulated_gold"`
	ItemIDs         []string                  `json:"item_ids,omitempty"`
	ConsumedItems   []corebattle.ConsumedItem `json:"consumed_items,omitempty"`
	mpKnown         bool
}

// HasCurrentMP reports whether the member carries an explicit MP snapshot rather than a pre-MP legacy payload.
func (m ExpeditionMember) HasCurrentMP() bool {
	return m.mpKnown || m.CurrentMP > 0
}

type rawExpeditionMember struct {
	CharacterID     string                    `json:"character_id"`
	Name            string                    `json:"name"`
	JobID           string                    `json:"job_id"`
	Level           int                       `json:"level"`
	CurrentHP       int                       `json:"current_hp"`
	MaxHP           int                       `json:"max_hp"`
	CurrentMP       *int                      `json:"current_mp,omitempty"`
	MaxMP           *int                      `json:"max_mp,omitempty"`
	AccumulatedExp  int                       `json:"accumulated_exp"`
	AccumulatedGold int                       `json:"accumulated_gold"`
	ItemIDs         []string                  `json:"item_ids,omitempty"`
	ConsumedItems   []corebattle.ConsumedItem `json:"consumed_items,omitempty"`
}

func (m *ExpeditionMember) UnmarshalJSON(data []byte) error {
	var raw rawExpeditionMember
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = ExpeditionMember{
		CharacterID:     raw.CharacterID,
		Name:            raw.Name,
		JobID:           raw.JobID,
		Level:           raw.Level,
		CurrentHP:       raw.CurrentHP,
		MaxHP:           raw.MaxHP,
		AccumulatedExp:  raw.AccumulatedExp,
		AccumulatedGold: raw.AccumulatedGold,
		ItemIDs:         raw.ItemIDs,
		ConsumedItems:   raw.ConsumedItems,
	}
	if raw.CurrentMP != nil {
		m.CurrentMP = *raw.CurrentMP
		m.mpKnown = true
	}
	if raw.MaxMP != nil {
		m.MaxMP = *raw.MaxMP
	}
	return nil
}

func EncodeMembers(members []ExpeditionMember) string {
	raw := make([]rawExpeditionMember, len(members))
	for i, m := range members {
		entry := rawExpeditionMember{
			CharacterID:     m.CharacterID,
			Name:            m.Name,
			JobID:           m.JobID,
			Level:           m.Level,
			CurrentHP:       m.CurrentHP,
			MaxHP:           m.MaxHP,
			AccumulatedExp:  m.AccumulatedExp,
			AccumulatedGold: m.AccumulatedGold,
			ItemIDs:         m.ItemIDs,
			ConsumedItems:   m.ConsumedItems,
		}
		if m.HasCurrentMP() {
			currMP := m.CurrentMP
			entry.CurrentMP = &currMP
		}
		if m.HasCurrentMP() || m.MaxMP > 0 {
			maxMP := m.MaxMP
			entry.MaxMP = &maxMP
		}
		raw[i] = entry
	}
	b, _ := json.Marshal(raw)
	return string(b)
}

func DecodeMembers(data string) []ExpeditionMember {
	members, _ := decodeMembers(data)
	return members
}

func decodeMembers(data string) ([]ExpeditionMember, error) {
	if strings.TrimSpace(data) == "" {
		return []ExpeditionMember{}, nil
	}
	var members []ExpeditionMember
	if err := json.Unmarshal([]byte(data), &members); err != nil {
		return nil, err
	}
	if members == nil {
		members = []ExpeditionMember{}
	}
	return members, nil
}

// StartPartyExpedition begins a 2D dungeon exploration for up to 4 party members.
func (s *Service) StartPartyExpedition(
	ctx context.Context,
	leaderCharID string,
	memberIDs []string,
	dungeonID string,
	partyID string,
) (*ActiveExpedition, error) {
	if s.admission == nil {
		return nil, ErrPartyUnavailable
	}
	var exp *ActiveExpedition
	err := s.admission.AdmitParty(ctx, leaderCharID, partyID, memberIDs, func(ctx context.Context, resolvedParty string, ids []string) error {
		var err error
		exp, err = s.startRoster(ctx, leaderCharID, ids, dungeonID, resolvedParty)
		return err
	})
	if err != nil && exp != nil {
		return s.reconcileStart(ctx, exp, err)
	}
	return exp, err
}

func (s *Service) startRoster(
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

	if len(memberIDs) > 4 {
		return nil, ErrTooManyPartyMembers
	}

	if len(dungeon.Floors) == 0 {
		return nil, errors.New("dungeon has no floors")
	}

	firstFloor := dungeon.Floors[0]
	members := make([]ExpeditionMember, 0, len(memberIDs))
	leaderHP := 0

	for _, mID := range memberIDs {
		existing, err := s.activeStore.GetActiveExpedition(ctx, mID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrActiveExpeditionExists
		}
		char, err := s.characterRepo.FindByID(ctx, mID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return nil, ErrCharacterNotFound
			}
			return nil, err
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

		maxMP := char.Stats.MaxMP
		if maxMP <= 0 {
			maxMP = char.Stats.MP
		}
		currMP := char.Stats.MP
		if currMP < 0 {
			currMP = 0
		} else if maxMP > 0 && currMP > maxMP {
			currMP = maxMP
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
			CurrentMP:   currMP,
			MaxMP:       maxMP,
			ItemIDs:     itemIDs,
			mpKnown:     true,
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
		ConsumedEvents:    []string{},
		Status:            StatusExploring,
		StartedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.activeStore.SaveActiveExpedition(ctx, exp); err != nil {
		return nil, err
	}

	return &exp, nil
}
