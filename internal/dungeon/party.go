package dungeon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
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
	itemsKnown      bool
}

// HasCurrentMP reports whether the member carries an explicit MP snapshot rather than a pre-MP legacy payload.
func (m ExpeditionMember) HasCurrentMP() bool {
	return m.mpKnown || m.CurrentMP > 0
}

// HasInventorySnapshot reports whether the member carries a snapshotted inventory (including empty).
func (m ExpeditionMember) HasInventorySnapshot() bool {
	return m.itemsKnown || len(m.ItemIDs) > 0
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
	ItemIDs         json.RawMessage           `json:"item_ids,omitempty"`
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
		ConsumedItems:   raw.ConsumedItems,
	}
	if trimmed := bytes.TrimSpace(raw.ItemIDs); len(trimmed) > 0 {
		// Valkey Lua cjson encodes an empty nested array as "{}".
		if bytes.Equal(trimmed, []byte("{}")) {
			m.ItemIDs = []string{}
			m.itemsKnown = true
		} else {
			var ids []string
			if err := json.Unmarshal(trimmed, &ids); err != nil {
				return err
			}
			if ids != nil {
				m.ItemIDs = ids
				m.itemsKnown = true
			}
		}
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
	type encodedMember struct {
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
		ItemIDs         *[]string                 `json:"item_ids,omitempty"`
		ConsumedItems   []corebattle.ConsumedItem `json:"consumed_items,omitempty"`
	}
	raw := make([]encodedMember, len(members))
	for i, m := range members {
		entry := encodedMember{
			CharacterID:     m.CharacterID,
			Name:            m.Name,
			JobID:           m.JobID,
			Level:           m.Level,
			CurrentHP:       m.CurrentHP,
			MaxHP:           m.MaxHP,
			AccumulatedExp:  m.AccumulatedExp,
			AccumulatedGold: m.AccumulatedGold,
			ConsumedItems:   m.ConsumedItems,
		}
		if m.HasInventorySnapshot() {
			ids := m.ItemIDs
			if ids == nil {
				ids = []string{}
			}
			entry.ItemIDs = &ids
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

	if len(memberIDs) == 0 {
		return nil, ErrPartyEmpty
	}
	if len(memberIDs) > 4 {
		return nil, ErrTooManyPartyMembers
	}
	seen := make(map[string]bool, len(memberIDs))
	for _, mID := range memberIDs {
		if strings.TrimSpace(mID) == "" || seen[mID] {
			return nil, ErrForbidden
		}
		seen[mID] = true
	}
	if !seen[leaderCharID] {
		return nil, ErrForbidden
	}

	if len(dungeon.Variants) == 0 {
		return nil, errors.New("dungeon has no layouts")
	}
	if dungeon.LeaderJobLevel > 0 {
		leader, err := s.characterRepo.FindByID(ctx, leaderCharID)
		if errors.Is(err, corecharacter.ErrNotFound) {
			return nil, ErrCharacterNotFound
		}
		if err != nil {
			return nil, err
		}
		if leader.JobLevel < dungeon.LeaderJobLevel {
			return nil, ErrJobLevelRequired
		}
	}
	// Legacy picks one initial layout; accepted start is the leader's
	// initial advance from round 0 to 1 without a tile event.
	initial := dungeon.Initial
	if initial == 0 {
		initial = len(dungeon.Variants)
	}
	variant := s.rng.Intn(initial) + 1
	firstFloor := dungeon.Variants[variant-1]
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
		itemsKnown := false
		if s.invRepo != nil {
			inv, err := s.invRepo.FindByCharacterID(ctx, char.ID)
			if err != nil {
				return nil, err
			}
			itemsKnown = true
			maxHP, maxMP = battle.HeldVitalityMaxima(inv, maxHP, maxMP)
			for _, it := range inv.Items {
				if it.Quantity > 0 {
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
			itemsKnown:  itemsKnown,
		})
	}

	now := time.Now().UTC()
	exp := ActiveExpedition{
		ID:              id.New(),
		CharacterID:     leaderCharID,
		PartyID:         partyID,
		Members:         members,
		DungeonID:       dungeon.ID,
		MapVariant:      variant,
		PosX:            firstFloor.StartX,
		PosY:            firstFloor.StartY,
		CurrentHP:       leaderHP,
		Round:           1,
		RoundLimit:      dungeon.limitOf(firstFloor),
		AccumulatedExp:  0,
		AccumulatedGold: 0,
		ConsumedEvents:  []string{},
		Status:          StatusExploring,
		StartedAt:       now,
		UpdatedAt:       now,
	}

	if dungeon.newState != nil {
		exp.MapState = dungeon.newState(s.rng)
	}
	if err := s.activeStore.SaveActiveExpedition(ctx, exp); err != nil {
		return nil, err
	}

	return &exp, nil
}
