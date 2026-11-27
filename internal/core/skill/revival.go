package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterRevivals binds concrete native producers without assembling partial
// job tables. The consumer rebuilds these operation-local closures after reload.
func (d *BattleDefinitions) RegisterRevivals(c *corebattle.RevivalContext) {
	if d.RevivalSkills == nil {
		d.RevivalSkills = make(map[string]bool)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range corebattle.RevivalEntries() {
		d.RevivalSkills[entry.ID] = true
		d.Invocations.Entries[entry.ID] = d.RevivalInvocation(c, entry.ID)
	}
}

// RevivalAction uses the executor's gate before an owner charges any ordinary
// MP/CMP/tool cost. A registry does not certify learning or complete job tables.
func (d BattleDefinitions) RevivalAction(s *corebattle.LifetimeState, c *corebattle.RevivalContext, id, targetID string, rng random.Generator) (corebattle.CallbackEntry, error) {
	if !d.RevivalSkills[id] {
		return corebattle.CallbackEntry{}, corebattle.ErrUnsupportedEffect
	}
	if err := s.ValidateRevivalAction(c, id, targetID, rng); err != nil {
		return corebattle.CallbackEntry{}, err
	}
	for _, entry := range corebattle.RevivalEntries() {
		if entry.ID == id {
			return entry, nil
		}
	}
	return corebattle.CallbackEntry{}, corebattle.ErrUnsupportedEffect
}

// RevivalInvocation preserves naked/explicit target arguments.
func (d BattleDefinitions) RevivalInvocation(c *corebattle.RevivalContext, id string) corebattle.CallbackDefinition {
	context := func(frame *corebattle.CallbackContext) corebattle.RevivalContext {
		child := *c
		child.TargetContext = frame.TargetContext
		if child.NativeDefeat != nil {
			defeat := *child.NativeDefeat
			defeat.TargetContext = frame.TargetContext
			if frame.Defeat != nil {
				defeat.Cursor = frame.Defeat.Cursor
			}
			child.NativeDefeat = &defeat
		}
		return child
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			if c == nil || !d.RevivalSkills[id] {
				return corebattle.ErrUnsupportedEffect
			}
			child := context(frame)
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			return s.ValidateRevival(&child, id, target, random.Default())
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			if c == nil || !d.RevivalSkills[id] {
				return corebattle.ErrUnsupportedEffect
			}
			rollbackContext(frame, c)
			*c = context(frame)
			target := ""
			if len(frame.Arguments) > 0 {
				target = frame.Arguments[0]
			}
			r, err := s.ApplyRevival(c, id, target, rng)
			if r.TargetConsumed && len(frame.Arguments) > 0 {
				frame.Arguments = frame.Arguments[1:]
			}
			frame.TargetContext = c.TargetContext
			if c.NativeDefeat != nil {
				frame.Defeat = c.NativeDefeat
			}
			return err
		},
	}
}
