package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCJob98 binds the complete ordinary stage20 table at
// _skill.cgi:7309–7408, independently of other modes sharing job98's identity.
// Rebuild closures against operation ports after loading frozen lifetime facts.
func (d *BattleDefinitions) RegisterNPCJob98(c *corebattle.NPCJob98Context) {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	entries := corebattle.NPCJobEntries(98)
	d.Invocations.JobTables[98] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	for _, entry := range entries {
		d.Invocations.Entries[entry.ID] = npcJob98Invocation(c, entry.ID)
	}
}

func npcJob98Invocation(c *corebattle.NPCJob98Context, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			if c == nil || f.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext, child.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			return s.ValidateNPCJob98(&child, id, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext, c.Arguments = f.TargetContext, append([]string(nil), f.Arguments...)
			err := s.ApplyNPCJob98(c, id, rng)
			f.TargetContext, f.Arguments = c.TargetContext, c.Arguments
			return err
		},
	}
}
