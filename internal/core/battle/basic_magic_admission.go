package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateBasicMagic admits complete descendants on isolated state, without
// argument consumption, resource writes, external Apply hooks or RNG draws.
func (s *LifetimeState) ValidateBasicMagic(c *BasicMagicContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	switch id {
	case "job-5-entry-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), .4, CategoryMagic), StatDefense, rng)
	case "job-5-entry-2":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), StatusDeadlyPoison, CategoryMagic), rng)
	case "job-5-entry-5", "job-5-entry-8":
		power := 90.0
		if id == "job-5-entry-8" {
			power = 999
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), power, CategoryMagic), rng)
	case "job-6-entry-2", "job-6-entry-4":
		coefficient, target := .2, ""
		if id == "job-6-entry-2" {
			coefficient, target = .4, child.shift()
		}
		return p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, coefficient, CategoryMagic), rng)
	case "job-6-entry-6":
		return p.ValidateStatus(&StatusContext{EffectContext: child.EffectContext}, controlSymbol(child.shift(), StatusSleep, CategoryMagic), 65, false, false, rng)
	case "job-5-entry-0", "job-5-entry-4", "job-5-entry-6", "job-5-entry-9",
		"job-6-entry-0", "job-6-entry-5", "job-6-entry-7", "job-6-entry-8", "job-6-entry-9":
		power, width, area := p.basicMagicDamage(&child, id)
		if area {
			return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", power+width, CategoryMagic, true), rng)
		}
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), power, CategoryMagic, true), rng)
	}
	return ErrUnsupportedEffect
}
