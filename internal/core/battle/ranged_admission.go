package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateRanged admits complete descendants on private facts, before costs,
// argument consumption, shared mutation, external Apply hooks or committed RNG.
func (s *LifetimeState) ValidateRanged(c *RangedContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, job := range []int{11, 42, 54} {
		for _, entry := range RangedEntries(job) {
			known = known || entry.ID == id
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	child.Status.EffectContext = child.EffectContext
	a := p.actor(c.ActorID)
	switch id {
	case "job-11-entry-2", "job-54-entry-2":
		return nil
	case "job-11-entry-8":
		_, err := p.gemTensionCeiling(&child.EffectContext, a, 1)
		return err
	case "job-11-entry-1", "job-54-entry-0", "job-42-entry-0", "job-42-entry-1":
		symbol, chance, category := StatusDofuu, 80.0, CategoryPhysical
		if id == "job-42-entry-0" {
			symbol, category = StatusConfusion, CategoryMagic
		} else if id == "job-42-entry-1" {
			symbol, chance, category = StatusMagicSeal, 90, CategoryMagic
		}
		return p.ValidateStatus(&child.Status, controlSymbol(child.shift(), symbol, category), chance, false, false, rng)
	case "job-42-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(c.ActorID, 1, CategoryUntyped), StatAgility, rng)
	case "job-11-entry-4":
		if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(child.shift(), .15, CategoryPhysical), rng); err != nil {
			return err
		}
		if a.categorySealed(CategoryPhysical) {
			a.Status = ""
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(c.ActorID, 1, CategoryPhysical), rng)
	case "job-42-entry-5":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "collapse", CategoryMagic), false)
		if err != nil {
			return err
		}
		if a.categorySealed(CategoryMagic) {
			a.Status = ""
		}
		for _, target := range targets {
			if target != "" && p.actor(target).MaxHP <= 999 && p.actor(target).BaseStats.Defense <= 999 {
				if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .75, CategoryMagic), rng); err != nil {
					return err
				}
			}
		}
		return nil
	case "job-11-entry-3", "job-54-entry-4":
		return p.validateAreaDamage(&child.DamageContext, rangedDamage(a, id, ""), rng)
	case "job-11-entry-7", "job-54-entry-5":
		if a.HP <= 0 {
			return nil
		}
		return p.validateRepeatedDamage(&child.DamageContext, rangedDamage(a, id, ""), 4, rng)
	}
	e := rangedDamage(a, id, child.shift())
	symbol, chance, death := rangedTail(id)
	if chance == 0 {
		return p.ValidateDamage(&child.DamageContext, e, rng)
	}
	child.AdditionalEffect = true
	states, err := p.projectCallbackDamage(&child.DamageContext, child.Status.Defeat, e, e.Payload.Number, true, rng)
	if err != nil {
		return err
	}
	for _, next := range states {
		t := next.State.actor(next.TargetID)
		if t == nil || t.HP <= 0 {
			continue
		}
		if symbol == "" {
			if err := next.State.validateControlDown(&ControlClimaxContext{DamageContext: next.Context}, controlEffect(t.ID, chance, CategoryPhysical), rng); err != nil {
				return err
			}
		} else {
			status := child.Status
			status.EffectContext = next.Context.EffectContext
			if err := next.State.ValidateStatus(&status, controlSymbol(t.ID, symbol, CategoryPhysical), chance, false, death, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

// ProjectRanged carries only deterministic inline writes into later admission.
func (s *LifetimeState) ProjectRanged(c *RangedContext, id string) (bool, error) {
	if err := s.ValidateRanged(c, id, random.Default()); err != nil {
		return false, err
	}
	if id == "job-11-entry-2" || id == "job-54-entry-2" {
		s.actor(c.ActorID).Hit = 95
		return true, nil
	}
	return false, nil
}
