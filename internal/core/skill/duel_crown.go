package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterDuelCrown contributes the complete, ordered job74 table. Functions
// are rebuilt from saved operation facts; no callback is replayed on reload.
func (d *BattleDefinitions) RegisterDuelCrown(c *corebattle.DuelCrownContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	entries := corebattle.DuelCrownEntries()
	d.Invocations.JobTables[74] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	for _, entry := range entries {
		d.Invocations.Entries[entry.ID] = duelCrownInvocation(c, entry.ID)
	}
}

func duelCrownInvocation(c *corebattle.DuelCrownContext, id string) corebattle.CallbackDefinition {
	context := func(frame *corebattle.CallbackContext) (corebattle.DuelCrownContext, error) {
		if c == nil {
			return corebattle.DuelCrownContext{}, corebattle.ErrUnsupportedEffect
		}
		child := *c
		child.TargetContext = frame.TargetContext
		child.Arguments = append([]string(nil), frame.Arguments...)
		return child, nil
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			child, err := context(frame)
			if err != nil {
				return err
			}
			if frame.AfterEffects && id != "job-74-entry-1" && id != "job-74-entry-4" {
				return corebattle.ErrUnsupportedEffect
			}
			return s.ValidateDuelCrown(&child, id, random.Default())
		},
		Project: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) (bool, error) {
			child, err := context(frame)
			if err != nil {
				return false, err
			}
			return s.ProjectDuelCrown(&child, id)
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = frame.TargetContext, append([]string(nil), frame.Arguments...)
			err := s.ApplyDuelCrown(c, id, rng)
			frame.TargetContext, frame.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
