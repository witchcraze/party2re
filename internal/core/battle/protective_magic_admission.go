package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateProtectiveMagic admits every reachable descendant on private state.
func (s *LifetimeState) ValidateProtectiveMagic(c *ProtectiveMagicContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if protectiveMagicTemporary(id) != "" {
		return nil
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	if power, area := protectiveMagicDamage(id); power != 0 {
		if area {
			return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", power, CategoryMagic, true), rng)
		}
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), power, CategoryMagic, true), rng)
	}
	switch id {
	case "job-32-entry-0":
		target := child.shift()
		for _, power := range []float64{30, 100} {
			if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(target, power, CategoryMagic, true), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-32-entry-1", "job-33-entry-3", "job-34-entry-5":
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, 1, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-33-entry-4":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), 1, CategoryMagic), StatAttack, rng)
	case "job-33-entry-7":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.ActorID, p.actor(child.ActorID).Status, CategoryUntyped), rng)
	case "job-34-entry-0":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "cover", CategoryPhysical), rng)
	case "job-34-entry-2":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 300, CategoryUntyped), rng)
	case "job-34-entry-6":
		return p.validateProtectiveGroup(&child, rng)
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateProtectiveGroup(c *ProtectiveMagicContext, rng random.Generator) error {
	e, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, controlSymbol(c.shift(), "group", CategoryMagic))
	if err != nil || len(c.EnemyIDs) == 0 {
		return err
	}
	// A successful first helper cured the seal before the second resolution.
	if s.actor(c.ActorID).categorySealed(e.Category) {
		s.actor(c.ActorID).Status = ""
	}
	candidates, err := s.statusTargetCandidates(&c.DamageContext, e)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		p, child := s.callbackCopy(), *c
		// Symbolic absorption can make an ally eligible for the subsequent debit.
		applyStatusResources(&p, candidate)
		power := p.protectiveGroupPower(&child)
		if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(candidate.TargetID, power, CategoryMagic, true), rng); err != nil {
			return err
		}
	}
	return nil
}

// Only unsealed direct temporary writes have an exact transition without RNG.
func (s *LifetimeState) ProjectProtectiveMagic(c *ProtectiveMagicContext, id string) (bool, error) {
	if err := s.ValidateProtectiveMagic(c, id, random.Default()); err != nil {
		return false, err
	}
	if protectiveMagicTemporary(id) == "" || s.actor(c.ActorID).categorySealed(CategoryMagic) {
		return false, nil
	}
	if err := s.ApplyProtectiveMagic(c, id, random.Default()); err != nil {
		return false, err
	}
	return true, nil
}
