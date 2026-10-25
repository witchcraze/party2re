package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateCounterReset uses the shared effect owners on isolated state, before
// arguments, category cures, resources, counters or committed RNG can change.
func (s *LifetimeState) ValidateCounterReset(c *CounterResetContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if counterResetTemporary(id) != "" {
		return nil
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	status := child.statusContext()
	switch id {
	case "job-35-entry-5", "job-37-entry-1", "job-37-entry-3":
		return nil
	case "job-35-entry-2":
		return p.ValidateStatus(&status, controlSymbol(child.shift(), "death", CategoryMagic), 20, false, true, rng)
	case "job-35-entry-3":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 180, CategoryBreath, true), rng)
	case "job-35-entry-4", "job-37-entry-6":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 300, CategoryUntyped), rng)
	case "job-35-entry-6":
		child.AdditionalEffect = true
		return p.validateAreaDamageStatus(&child.DamageContext, 150, StatusParalyze, 20, rng)
	case "job-37-entry-0":
		return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{StatusMagicSeal}, CategoryMagic, 70, rng)
	case "job-37-entry-2":
		return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{StatusDanceSeal}, CategoryDance, 70, rng)
	case "job-37-entry-4":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), string(TemporaryReflectMagic), CategoryMagic), rng)
	case "job-37-entry-5":
		return p.ValidateStatus(&status, controlSymbol(child.shift(), StatusPhysicalSeal, CategoryUntyped), 80, false, false, rng)
	case "job-37-entry-7":
		_, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "break", CategoryPhysical), false)
		return err
	}
	return ErrUnsupportedEffect
}

// Only deterministic writes have an exact projection for later callbacks.
func (s *LifetimeState) ProjectCounterReset(c *CounterResetContext, id string) (bool, error) {
	if err := s.ValidateCounterReset(c, id, random.Default()); err != nil {
		return false, err
	}
	if counterResetTemporary(id) == "" && id != "job-37-entry-3" {
		return false, nil
	}
	if err := s.ApplyCounterReset(c, id, random.Default()); err != nil {
		return false, err
	}
	return true, nil
}
