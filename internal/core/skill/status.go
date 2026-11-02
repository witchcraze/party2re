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
	context := func(frame *corebattle.CallbackContext) corebattle.StatusContext {
		child := *c
		if frame.Status != nil {
			child = *frame.Status
			child.Defeat = c.Defeat
		}
		if frame.Damage != nil {
			child.EffectContext = frame.Damage.EffectContext
		}
		child.TargetContext = frame.TargetContext
		if child.Defeat != nil {
			defeat := *child.Defeat
			defeat.TargetContext = frame.TargetContext
			if frame.Defeat != nil {
				defeat.Cursor = frame.Defeat.Cursor
			}
			child.Defeat = &defeat
		}
		return child
	}
	consume := func(frame *corebattle.CallbackContext) {
		if (id == "gem_cure_all_abnormal" || id == "gem_instant_death_1") && len(frame.Arguments) > 0 {
			frame.Arguments = frame.Arguments[1:]
		}
	}
	write := func(frame *corebattle.CallbackContext, child corebattle.StatusContext) {
		frame.Status, frame.TargetContext = &child, child.TargetContext
		if child.Defeat != nil {
			frame.Defeat = child.Defeat
		}
		damage := corebattle.DamageContext{}
		if frame.Damage != nil {
			damage = *frame.Damage
		}
		damage.EffectContext = child.EffectContext
		frame.Damage = &damage
		consume(frame)
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			if c == nil || frame.AfterEffects {
				return corebattle.ErrUnsupportedEffect
			}
			child := context(frame)
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			_, err := d.StatusAction(s, &child, reset, id, target, random.Default())
			return err
		},
		ProjectBranches: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) ([]corebattle.CallbackContinuation, error) {
			if c == nil || frame.AfterEffects || !d.StatusSkills[id] {
				return nil, corebattle.ErrUnsupportedEffect
			}
			child := context(frame)
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			states, err := s.ProjectStatusGemBranches(&child, id, target)
			if err != nil {
				return nil, err
			}
			var branches []corebattle.CallbackContinuation
			for _, state := range states {
				next := *frame
				write(&next, state.Context)
				branches = append(branches, corebattle.CallbackContinuation{State: state.State, Context: next})
			}
			return branches, nil
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil {
				return corebattle.ErrUnsupportedEffect
			}
			*c = context(frame)
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			_, err := d.StatusAction(s, c, reset, id, target, rng)
			if err != nil {
				return err
			}
			_, err = s.ApplyStatusEntry(c, reset, id, target, rng)
			write(frame, *c)
			return err
		},
	}
}
