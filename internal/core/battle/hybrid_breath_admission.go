package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateHybridBreath uses isolated state and shared descendant admission;
// neither external Apply hooks nor committed RNG run during discovery.
func (s *LifetimeState) ValidateHybridBreath(c *HybridBreathContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	if e, area, ok := hybridBreathDamage(id, p.actor(c.ActorID)); ok {
		if area {
			return p.validateAreaDamage(&child.DamageContext, e, rng)
		}
		e.TargetID = child.shift()
		return p.ValidateDamage(&child.DamageContext, e, rng)
	}
	switch id {
	case "job-39-entry-1":
		e := controlEffect("", .25, CategoryMagic)
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e.TargetID = target
			if err := p.validateStatUp(&child.EffectContext, e, StatDefense, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, StatDefense, &admission)
		}
		return nil
	case "job-39-entry-3":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .25, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-39-entry-2", "job-44-entry-4":
		power := 30.0
		if id == "job-44-entry-4" {
			power = 150
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), power, CategoryMagic), rng)
	case "job-39-entry-4", "job-41-entry-3":
		status, category, chance := StatusConfusion, CategoryMagic, 50.0
		if id == "job-41-entry-3" {
			status, category, chance = StatusParalyze, CategoryBreath, 35
		}
		return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{status}, category, chance, rng)
	case "job-41-entry-5":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "assault", CategoryUntyped), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if err := p.ValidateDamage(&child.DamageContext, p.hybridAssault(target, c.ActorID), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-44-entry-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .5, CategoryUntyped), StatDefense, rng)
	}
	return ErrUnsupportedEffect
}
