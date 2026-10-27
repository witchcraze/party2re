package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCOrderedReset certifies the full job403 table at _skill.cgi:8077–8121.
// Rebuild closures with operation ports after loading the frozen lifetime facts.
func (d *BattleDefinitions) RegisterNPCOrderedReset(c *corebattle.NPCOrderedResetContext) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	entries := corebattle.NPCJobEntries(403)
	d.Invocations.JobTables[403] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	for _, entry := range entries {
		d.Invocations.Entries[entry.ID] = npcOrderedResetInvocation(c, entry.ID)
	}
}

func npcOrderedResetInvocation(c *corebattle.NPCOrderedResetContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateNPCOrderedReset(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyNPCOrderedReset(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
