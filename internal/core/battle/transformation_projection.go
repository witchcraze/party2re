package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ProjectAppearanceGem changes only the actor's icon across all108 source positions.
func (s *LifetimeState) ProjectAppearanceGem(c *TransformationContext, frame *CallbackContext) ([]CallbackContinuation, error) {
	if s == nil || c == nil || frame == nil {
		return nil, ErrInvalidRequest
	}
	child := *c
	child.TargetContext = frame.TargetContext
	if err := s.ValidateTransformation(&child, "gem-100-appearance", "", random.Default()); err != nil {
		return nil, err
	}
	branches := make([]CallbackContinuation, 0, len(c.MonsterIcons))
	for _, icon := range c.MonsterIcons {
		state := s.callbackCopy()
		state.actor(frame.ActorID).Icon = monsterAppearance(icon)
		branches = append(branches, CallbackContinuation{State: state, Context: cloneCallbackContext(*frame)})
	}
	return branches, nil
}

// ProjectTransformation applies only exact intrinsic transitions to the
// admission-owned state. It never invokes damage/stat hooks or consumes RNG.
func (s *LifetimeState) ProjectTransformation(c *TransformationContext, id, targetID string) (bool, error) {
	if s == nil || c == nil || s.actor(c.ActorID) == nil {
		return false, ErrInvalidRequest
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-36-copy":
		target := s.actor(targetID)
		if target == nil || a.categorySealed(CategoryMagic) {
			return false, nil
		}
		a.copyTransformation(target, true)
	case "job-82-form":
		if a.switchForm("job-82", "job-300") && !a.NPC {
			a.Icon = nativeFormIcon("mechanical", c.RequesterGender)
		}
	case "job-300-form":
		a.switchForm("job-300", "job-82")
	default:
		return false, nil
	}
	return true, nil
}
