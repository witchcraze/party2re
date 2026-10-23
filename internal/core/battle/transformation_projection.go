package battle

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
