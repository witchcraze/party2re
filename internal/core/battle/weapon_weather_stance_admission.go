package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateWeaponWeatherStance uses shared descendant admission on private
// state, without committed RNG, resource writes or external Apply callbacks.
func (s *LifetimeState) ValidateWeaponWeatherStance(c *WeaponWeatherStanceContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	if p.applyWeaponStanceRaw(&child, id) {
		return nil
	}
	if e, area, ok := weaponWeatherDamage(id, p.actor(c.ActorID)); ok {
		if area {
			return p.validateAreaDamage(&child.DamageContext, e, rng)
		}
		e.TargetID = child.shift()
		return p.ValidateDamage(&child.DamageContext, e, rng)
	}
	switch id {
	case "job-24-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), 1, CategoryMagic), StatAttack, rng)
	case "job-27-entry-0", "job-27-entry-2":
		coefficient, category := .2, CategoryBreath
		if id == "job-27-entry-2" {
			coefficient, category = .3, CategoryMagic
		}
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, coefficient, category), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-27-entry-4", "job-27-entry-8":
		for _, target := range child.PartyIDs {
			// Both helpers retain explicit loop targets even after an earlier
			// heal/cure changes HP, confusion or rage. Neither has Apply ports.
			e := controlSymbol(target, StatusSleep, CategoryMagic)
			if id == "job-27-entry-8" {
				e = controlEffect(target, 300, CategoryUntyped)
			}
			if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
				return err
			}
		}
		return nil
	case "job-27-entry-7":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "break", CategoryPhysical), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if target == "" {
				continue
			}
			q := p.callbackCopy()
			t := q.actor(target)
			t.Temporary, t.Tension, t.Attack = "", 1, t.BaseStats.Attack
			if err := q.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .2, CategoryPhysical), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-28-entry-1", "job-28-entry-5", "job-28-entry-6":
		status := child.statusContext()
		e, chance, death := controlSymbol("", StatusDofuu, CategoryPhysical), 80.0, false
		if id == "job-28-entry-5" {
			return p.ValidateAreaStatus(&status, e, 65, false, false, rng)
		}
		if id == "job-28-entry-6" {
			e.Payload.Symbol, chance, death = "death", 25, true
		}
		e.TargetID = child.shift()
		return p.ValidateStatus(&status, e, chance, false, death, rng)
	}
	return ErrUnsupportedEffect
}

// Only raw deterministic assignments project exactly across callback boundaries.
func (s *LifetimeState) ProjectWeaponWeatherStance(c *WeaponWeatherStanceContext, id string) (bool, error) {
	if err := s.ValidateWeaponWeatherStance(c, id, random.Default()); err != nil {
		return false, err
	}
	return s.applyWeaponStanceRaw(c, id), nil
}
