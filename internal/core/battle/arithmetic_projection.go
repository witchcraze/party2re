package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ProjectArithmetic carries exact intrinsic writes into later admission, never
// drawing RNG or invoking defeat. Unknown transitions leave both inputs intact.
func (s *LifetimeState) ProjectArithmetic(c *ArithmeticContext, id, targetID string) (bool, error) {
	if s == nil || c == nil {
		return false, ErrInvalidRequest
	}
	if err := s.ValidateArithmetic(c, id, targetID, random.Default()); err != nil {
		return false, err
	}
	if s.actor(c.ActorID).categorySealed(CategoryMagic) {
		return false, nil
	}
	callback, divisor, err := c.ArithmeticSelection(id)
	if err != nil {
		return false, err
	}
	if callback == "" {
		return true, nil
	}
	if callback == "job-31-reverse" {
		a := s.actor(targetID)
		if a == nil {
			return false, nil // Ally fallback may choose and redirect a target.
		}
		reverseArithmeticHP(a)
		return true, nil
	}
	for i := range s.Actors {
		if arithmeticEligible(&s.Actors[i], callback, divisor) {
			switch callback {
			case "job-72-damage", "job-72-heal", "job-31-death", "job-72-death":
				return false, nil // Random amounts or the external defeat transition.
			}
		}
	}
	if callback != id {
		c.Divisor = divisor
	}
	for i := range s.Actors {
		if arithmeticEligible(&s.Actors[i], callback, divisor) {
			applyArithmeticValue(&s.Actors[i], callback)
		}
	}
	return true, nil
}
