package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCJob97 binds all eleven ordinary positions at _skill.cgi:7215–7307.
// Rebuild closures against operation ports after loading frozen lifetime facts.
func (d *BattleDefinitions) RegisterNPCJob97(c *corebattle.NPCJob97Context) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	entries := corebattle.NPCJobEntries(97)
	d.Invocations.JobTables[97] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	for _, entry := range entries {
		d.Invocations.Entries[entry.ID] = npcJob97Invocation(c, entry.ID)
	}
}

func npcJob97Invocation(c *corebattle.NPCJob97Context, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateNPCJob97(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyNPCJob97(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
