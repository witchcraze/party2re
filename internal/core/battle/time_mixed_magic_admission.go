package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateTimeMixedMagic admits complete descendants on private state, without
// external Apply hooks, resource writes, argument consumption or committed RNG.
func (s *LifetimeState) ValidateTimeMixedMagic(c *TimeMixedMagicContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	if tmp := timeMagicTemporary(id); tmp != "" {
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), string(tmp), CategoryMagic), rng)
	}
	switch id {
	case "job-29-entry-0", "job-29-entry-5":
		coefficient := .45
		if id == "job-29-entry-5" {
			coefficient = .5
		}
		e := controlEffect(child.shift(), coefficient, CategoryMagic)
		return p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, e, rng)
	case "job-29-entry-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), .45, CategoryMagic), StatAgility, rng)
	case "job-29-entry-3":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .35, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-29-entry-4", "job-31-entry-3":
		coefficient, stat := .35, StatAgility
		if id == "job-31-entry-3" {
			coefficient, stat = .5, StatDefense
		}
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e := controlEffect(target, coefficient, CategoryMagic)
			if err := p.validateStatUp(&child.EffectContext, e, stat, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, stat, &admission)
		}
		return nil
	case "job-30-entry-0", "job-30-entry-5":
		power := 60.0
		if id == "job-30-entry-5" {
			power = 180
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), power, CategoryMagic), rng)
	case "job-31-entry-5":
		return p.validateTimePartyHealing(&child, float64(a.HP), rng)
	case "job-31-entry-1":
		for _, t := range p.Actors {
			if t.HP <= 0 {
				t = *a
			}
			if t.HP <= 999 && t.Defense <= 999 && child.Defeat == nil {
				return ErrUnsupportedEffect
			}
		}
		return nil
	case "job-30-entry-8":
		target := child.shift()
		if t := p.actor(target); t != nil {
			if t.TeamID != a.TeamID {
				return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(target, 350*(a.Tension+5)/6, CategoryMagic, true), rng)
			}
			if err := p.validateTimePartyHealing(&child, 250, rng); err != nil {
				return err
			}
			t.Temporary = TemporaryReduceAttack
			return p.validateTimePartyHealing(&child, 250, rng)
		}
		if a.HP <= 0 {
			return nil
		}
		var effects []TargetEffect
		priorPower := false
		for _, power := range []int{a.Attack, a.Defense, a.Agility} {
			// An earlier reset/defeat may replace a later zero stat. The damage
			// port certifies those transitions; retain a positive descendant gate.
			if priorPower {
				power = max(1, power)
			}
			effects = append(effects, dualAgilityEffect("", float64(power), CategoryMagic, true))
			priorPower = priorPower || power != 0
		}
		return p.validateDamageSequence(&child.DamageContext, effects, rng)
	}
	count, power, target := 1, 0.0, ""
	switch id {
	case "job-29-entry-2":
		count, power = 3, 50
	case "job-29-entry-6":
		count, power = 6, 99
	case "job-30-entry-1":
		target, power = child.shift(), 35
	case "job-30-entry-3":
		target, power = child.shift(), 80
	case "job-30-entry-7":
		count, power = 2, 180
	case "job-31-entry-0":
		if child.Defeat == nil {
			return ErrUnsupportedEffect
		}
		target, power = child.shift(), float64(a.HP)
	case "job-31-entry-2":
		target, power = child.shift(), float64(a.MaxHP-a.HP+5)
	default:
		return ErrUnsupportedEffect
	}
	if count > 1 && a.HP <= 0 {
		return nil
	}
	return p.validateRepeatedDamage(&child.DamageContext, dualAgilityEffect(target, power, CategoryMagic, true), count, rng)
}

func (s *LifetimeState) validateTimePartyHealing(c *TimeMixedMagicContext, power float64, rng random.Generator) error {
	e := controlEffect("", power, CategoryMagic)
	if err := s.validateRecovery(&c.EffectContext, e, rng); err != nil {
		return err
	}
	for _, target := range c.PartyIDs {
		e.TargetID = target
		if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
			return err
		}
	}
	return nil
}
