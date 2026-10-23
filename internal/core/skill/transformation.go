package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterTransformations binds eight bodies at seven source positions. Other
// bodies in these complete tables remain with their own registration owners.
func (d *BattleDefinitions) RegisterTransformations(c *corebattle.TransformationContext) {
	if d.TransformationSkills == nil {
		d.TransformationSkills = make(map[string]bool)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for job, table := range transformationTables(c) {
		d.Invocations.JobTables[job] = table
	}
	for _, id := range []string{"job-36-copy", "job-79-breath", "job-79-speed", "job-82-form", "job-300-tune", "job-300-strike", "job-300-burst", "job-300-form"} {
		d.TransformationSkills[id] = true
		d.Invocations.Entries[id] = d.TransformationInvocation(c, id)
	}
}

func (d BattleDefinitions) TransformationAction(s *corebattle.LifetimeState, c *corebattle.TransformationContext, id, targetID string, rng random.Generator) (corebattle.TransformationEntry, error) {
	if s == nil || c == nil {
		return corebattle.TransformationEntry{}, corebattle.ErrInvalidRequest
	}
	if !d.TransformationSkills[id] {
		return corebattle.TransformationEntry{}, corebattle.ErrUnsupportedEffect
	}
	for _, table := range transformationTables(c) {
		for _, entries := range table.Variants {
			for _, entry := range entries {
				if entry.ID == id {
					if err := s.ValidateTransformation(c, id, targetID, rng); err != nil {
						return corebattle.TransformationEntry{}, err
					}
					return corebattle.TransformationEntry{ID: id, Name: entry.Name, RequiredSP: entry.RequiredSP, MPCost: entry.MPCost}, nil
				}
			}
		}
	}
	return corebattle.TransformationEntry{}, corebattle.ErrUnsupportedEffect
}

// TransformationInvocation preserves captured variant identity; rebuilding a
// table later must not change a previously selected breath/speed callback.
func (d BattleDefinitions) TransformationInvocation(c *corebattle.TransformationContext, id string) corebattle.CallbackDefinition {
	context := func(frame *corebattle.CallbackContext) (corebattle.TransformationContext, string, error) {
		if c == nil {
			return corebattle.TransformationContext{}, "", corebattle.ErrUnsupportedEffect
		}
		child := *c
		child.TargetContext = frame.TargetContext
		target := ""
		if len(frame.Arguments) > 0 {
			target = frame.Arguments[0]
		}
		return child, target, nil
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			child, target, err := context(frame)
			if err != nil {
				return err
			}
			if frame.AfterEffects && id != "job-36-copy" && id != "job-82-form" && id != "job-300-form" {
				return corebattle.ErrUnsupportedEffect
			}
			_, err = d.TransformationAction(s, &child, id, target, random.Default())
			return err
		},
		Project: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) (bool, error) {
			child, target, err := context(frame)
			if err != nil || !d.TransformationSkills[id] {
				return false, corebattle.ErrUnsupportedEffect
			}
			return s.ProjectTransformation(&child, id, target)
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			_, target, err := context(frame)
			if err != nil {
				return err
			}
			c.TargetContext = frame.TargetContext
			if _, err := d.TransformationAction(s, c, id, target, rng); err != nil {
				return err
			}
			_, err = s.ApplyTransformation(c, id, target, rng)
			frame.TargetContext = c.TargetContext
			if (id == "job-36-copy" || id == "job-300-strike" || id == "job-300-burst") && len(frame.Arguments) > 0 {
				frame.Arguments = frame.Arguments[1:]
			}
			return err
		},
	}
}
