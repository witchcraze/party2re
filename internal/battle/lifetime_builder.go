package battle

import (
	"context"
	"errors"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

// JobHistoryRepository is deliberately read-only: battle copies never record
// training, change the permanent job pair or save a temporary SP baseline.
type JobHistoryRepository interface {
	FindByCharacterID(context.Context, string) (corejob.CharacterJob, error)
}

func WithJobHistoryRepository(repo JobHistoryRepository) Option {
	return func(s *Service) { s.jobHistory = repo }
}

// BuildLifetimeActor shares the equipment-aware participant builder. The
// consumer supplies the original appearance identity reference, and retains
// transaction/snapshot ownership. Run reloads use saved state, not this builder.
func (s *Service) BuildLifetimeActor(ctx context.Context, characterID, icon string) (corebattle.LifetimeActor, error) {
	if s.charRepo == nil || strings.TrimSpace(characterID) == "" {
		return corebattle.LifetimeActor{}, ErrCharacterNotFound
	}
	if s.jobHistory == nil {
		return corebattle.LifetimeActor{}, ErrInvalidRequest
	}
	char, err := s.charRepo.FindByID(ctx, strings.TrimSpace(characterID))
	if err != nil {
		return corebattle.LifetimeActor{}, err
	}
	p, err := s.buildParticipantFromCharacter(ctx, char)
	if err != nil {
		return corebattle.LifetimeActor{}, err
	}
	history, err := s.jobHistory.FindByCharacterID(ctx, char.ID)
	if errors.Is(err, corejob.ErrCharacterJobNotFound) {
		history, err = corejob.NewCharacterJob(char.ID, char.JobID)
	}
	if err != nil {
		return corebattle.LifetimeActor{}, err
	}
	if history.CharacterID != char.ID {
		return corebattle.LifetimeActor{}, ErrInvalidRequest
	}
	if icon == "" {
		icon = "assets/images/placeholders/character/character-default.svg"
	}
	p.Icon = icon
	// Explicit effects inspect equipped identities, never bag contents. The
	// batch participant's historical inventory-based passives stay separate.
	p.ItemDefinitionIDs = nil
	for _, id := range []string{p.Gear.WeaponID, p.Gear.ArmorID, p.Gear.AccessoryID} {
		if id != "" {
			p.ItemDefinitionIDs = append(p.ItemDefinitionIDs, id)
		}
	}
	// Explicit state preserves fallen entrants; the batch builder's HP fallback
	// remains unchanged for its existing callers.
	p.HP = char.Stats.HP
	jobs := corebattle.BattleJobs{JobID: char.JobID, SP: char.SP, OldJobID: char.OldJobID, OldSP: char.OldSP}
	a := corebattle.LifetimeActor{Participant: p, Jobs: jobs, OriginalJobs: jobs, WeaponSeal: char.WeaponSeal, Tension: 1, Hit: 95}
	a.JobHistory = make(map[string]*int)
	for _, change := range history.History {
		if change.FromJobID != "" {
			a.JobHistory[change.FromJobID] = nil
		}
	}
	for _, ids := range [][]string{history.MasteredJobs, history.RecordedJobIDs} {
		for _, id := range ids {
			if id != "" {
				a.JobHistory[id] = nil
			}
		}
	}
	for id := range a.JobHistory {
		if sp, known := history.SavedSP(id); known {
			a.JobHistory[id] = &sp
		}
	}
	if len(a.JobHistory) == 0 {
		a.JobHistory = nil
	}
	return a, nil
}
