package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCJob407 binds the stage25 EX statue's table to the response's
// roster context, which carries the stage payloads. Rebuild after reload.
func (d *BattleDefinitions) RegisterNPCJob407(c *corebattle.RosterContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	entries := corebattle.NPCJobEntries(407)
	d.Invocations.JobTables[407] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	for _, entry := range entries {
		id := entry.ID
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{
			Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				if c == nil {
					return corebattle.ErrUnsupportedEffect
				}
				child := *c
				child.TargetContext = f.TargetContext
				return s.ValidateNPCJob407(&child, id, random.Default())
			},
			Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
				if c == nil {
					return corebattle.ErrUnsupportedEffect
				}
				rollbackContext(f, c)
				rollbackContext(f, c.Death)
				rollbackStageLoaded(f, c.StageLoaded)
				c.TargetContext = f.TargetContext
				err := s.ApplyNPCJob407(c, id, rng)
				f.TargetContext = c.TargetContext
				return err
			},
		}
	}
}
