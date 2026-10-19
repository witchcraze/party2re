package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterStatuses binds this owner's concrete bodies, never a partial job
// table. The consumer reconstructs operation contexts after loading saved facts.
func (d *BattleDefinitions) RegisterStatuses(c *corebattle.StatusContext, reset *corebattle.EquipmentResetContext) {
	if d.StatusSkills == nil {
		d.StatusSkills = map[string]bool{}
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = map[string]corebattle.CallbackDefinition{}
	}
	for _, entry := range corebattle.StatusEntries() {
		d.StatusSkills[entry.ID] = true
		d.Invocations.Entries[entry.ID] = d.StatusInvocation(c, reset, entry.ID)
	}
}

func (d BattleDefinitions) StatusAction(s *corebattle.LifetimeState, c *corebattle.StatusContext, reset *corebattle.EquipmentResetContext, id, targetID string, rng random.Generator) (corebattle.StatusEntry, error) {
	if !d.StatusSkills[id] {
		return corebattle.StatusEntry{}, corebattle.ErrUnsupportedEffect
	}
	if err := s.ValidateStatusEntry(c, reset, id, targetID, rng); err != nil {
		return corebattle.StatusEntry{}, err
	}
	for _, entry := range corebattle.StatusEntries() {
		if entry.ID == id {
			return entry, nil
		}
	}
	return corebattle.StatusEntry{}, corebattle.ErrUnsupportedEffect
}

func (d BattleDefinitions) StatusInvocation(c *corebattle.StatusContext, reset *corebattle.EquipmentResetContext, id string) corebattle.CallbackDefinition {
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			if c == nil || frame.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := *c
			child.TargetContext = frame.TargetContext
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			_, err := d.StatusAction(s, &child, reset, id, target, random.Default())
			return err
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			c.TargetContext = frame.TargetContext
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			entry, err := d.StatusAction(s, c, reset, id, target, rng)
			if err != nil {
				return err
			}
			_, err = s.ApplyStatusEntry(c, reset, id, target, rng)
			if (entry.SourceID == 62 || entry.SourceID == 70) && len(frame.Arguments) > 0 {
				frame.Arguments = frame.Arguments[1:]
			}
			frame.TargetContext = c.TargetContext
			return err
		},
	}
}
