package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCJob402 binds all six ordinary positions at _skill.cgi:7802–7886.
// Discovery and execution share whole-body admission. Rebuild after reload.
func (d *BattleDefinitions) RegisterNPCJob402(c *corebattle.NPCJob402Context) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	entries := corebattle.NPCJobEntries(402)
	d.Invocations.JobTables[402] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	for _, entry := range entries {
		d.Invocations.Entries[entry.ID] = npcJob402Invocation(c, entry.ID)
	}
}

func npcJob402Invocation(c *corebattle.NPCJob402Context, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateNPCJob402(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyNPCJob402(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
