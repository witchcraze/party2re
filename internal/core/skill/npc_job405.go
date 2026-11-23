package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCJob405 binds both HP variants of the EX table. The variant is
// selected when the table is built. Rebuild after reload.
func (d *BattleDefinitions) RegisterNPCJob405(c *corebattle.NPCJob405Context) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	variants := corebattle.NPCJob405Variants()
	d.Invocations.JobTables[405] = corebattle.CallbackTable{
		Variants: [][]corebattle.CallbackEntry{variants[0], variants[1]},
		Select: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) int {
			for i := range s.Actors {
				if s.Actors[i].ID == f.ActorID && corebattle.NPCJob405Wounded(&s.Actors[i]) {
					return 1
				}
			}
			return 0
		},
	}
	for _, entries := range variants {
		for _, entry := range entries {
			d.Invocations.Entries[entry.ID] = npcJob405Invocation(c, entry.ID)
		}
	}
}

func npcJob405Invocation(c *corebattle.NPCJob405Context, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateNPCJob405(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			rollbackContext(f, c)
			rollbackContext(f, c.Death)
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyNPCJob405(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
