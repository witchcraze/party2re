package dungeon

import (
	"context"
	"errors"
	"sort"
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

// PartyRepository defines the external party domain contracts required by dungeon.
type PartyRepository interface {
	GetParty(ctx context.Context, id string) (party.Party, error)
	GetMembers(ctx context.Context, partyID string) ([]party.Member, error)
	UpdateParty(ctx context.Context, p party.Party) error
	UpdateMemberReady(ctx context.Context, partyID string, characterID string, ready bool) error
	WithPartyLock(ctx context.Context, partyID string, fn func(ctx context.Context) error) error
}

// WithPartyRepository configures the PartyRepository for party roster derivation.
func WithPartyRepository(repo PartyRepository) Option {
	return func(s *Service) {
		s.partyRepo = repo
	}
}

func (s *Service) resetPartyState(ctx context.Context, partyID string) {
	if partyID == "" || s.partyRepo == nil {
		return
	}

	resetFn := func(lockedCtx context.Context) error {
		members, err := s.partyRepo.GetMembers(lockedCtx, partyID)
		if err != nil {
			return err
		}
		for _, m := range members {
			if err := s.partyRepo.UpdateMemberReady(lockedCtx, partyID, m.CharacterID, false); err != nil {
				return err
			}
		}
		p, err := s.partyRepo.GetParty(lockedCtx, partyID)
		if err != nil {
			return err
		}
		p.Status = party.StatusRecruiting
		return s.partyRepo.UpdateParty(lockedCtx, p)
	}

	// Use background context with a safety timeout so party reset is not aborted if the incoming HTTP request context is cancelled.
	resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const maxRetries = 5
	backoff := 20 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := s.partyRepo.WithPartyLock(resetCtx, partyID, resetFn)
		if err == nil {
			return
		}
		if resetCtx.Err() != nil {
			break
		}
		if !errors.Is(err, party.ErrPartyNotRecruiting) {
			break
		}
		select {
		case <-resetCtx.Done():
			break
		case <-time.After(backoff):
		}
		backoff *= 2
	}

	// Fallback: If lock could not be acquired due to persistent contention or Valkey failure,
	// guarantee party status and member ready states are reset directly so the party
	// is not permanently stuck in StatusInProgress.
	members, err := s.partyRepo.GetMembers(resetCtx, partyID)
	if err == nil {
		for _, m := range members {
			//lint:ignore error-swallow best-effort member ready state reset upon fallback
			_ = s.partyRepo.UpdateMemberReady(resetCtx, partyID, m.CharacterID, false)
		}
	}
	p, err := s.partyRepo.GetParty(resetCtx, partyID)
	if err == nil {
		p.Status = party.StatusRecruiting
		//lint:ignore error-swallow best-effort party status reset upon fallback
		_ = s.partyRepo.UpdateParty(resetCtx, p)
	}
}

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

		err = s.withRosterLocks(lockedCtx, memberIDs, func(rosterLockedCtx context.Context) error {
			for _, mID := range memberIDs {
				existing, err := s.activeStore.GetActiveExpedition(rosterLockedCtx, mID)
				if err != nil {
					return err
				}
				if existing != nil && existing.Status == StatusExploring {
					return ErrActiveExpeditionExists
				}
			}

			if s.activityChecker != nil {
				for _, mID := range memberIDs {
					busy, err := s.activityChecker.HasActiveActivity(rosterLockedCtx, mID)
					if err != nil {
						return err
					}
					if busy {
						return ErrActiveExpeditionExists
					}
				}
			}

			p.Status = party.StatusInProgress
			if err := s.partyRepo.UpdateParty(rosterLockedCtx, p); err != nil {
				return err
			}

			expRes, err := s.startExpeditionRoster(rosterLockedCtx, leaderCharID, memberIDs, dungeonID, partyID, p.Name)
			if err != nil {
				p.Status = party.StatusRecruiting
				//lint:ignore error-swallow compensatory rollback on start failure
				_ = s.partyRepo.UpdateParty(rosterLockedCtx, p)
				return err
			}

			exp = expRes
			return nil
		})
		if err != nil {
			if errors.Is(err, party.ErrPartyNotRecruiting) {
				return ErrActiveExpeditionExists
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
