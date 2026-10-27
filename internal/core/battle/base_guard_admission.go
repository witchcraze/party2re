package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateBaseGuard checks full descendants on private state without committed RNG.
func (s *LifetimeState) ValidateBaseGuard(c *BaseGuardContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for job := 0; job <= 3; job++ {
		for _, entry := range BaseGuardEntries(job) {
			known = known || entry.ID == id && id != "job-3-entry-6"
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	if baseGuardTemporary(id) != "" {
		return nil
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	if baseGuardCover(id) {
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "cover", CategoryPhysical), rng)
	}
	switch id {
	case "job-0-entry-0", "job-0-entry-1", "job-0-entry-2":
		return p.validateNormalStrikes(&child.DamageContext, child.shift(), 1, rng)
	case "job-0-entry-3":
		_, err := p.gemTensionCeiling(&child.EffectContext, a, 1)
		return err
	case "job-1-entry-3":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, 1, CategoryPhysical), StatAttack, rng)
	case "job-3-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .4, CategoryPhysical), StatDefense, rng)
	case "job-2-entry-2":
		if child.Status == nil {
			return ErrUnsupportedEffect
		}
		status := *child.Status
		status.EffectContext = child.EffectContext
		return p.ValidateStatus(&status, controlSymbol(child.shift(), StatusDofuu, CategoryPhysical), 80, false, false, rng)
	case "job-3-entry-5":
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e := controlEffect(target, .25, CategoryMagic)
			if err := p.validateStatUp(&child.EffectContext, e, StatDefense, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, StatDefense, &admission)
		}
		return nil
	}
	e, area := p.baseGuardDamage(&child, id, child.shift())
	if area {
		return p.validateAreaDamage(&child.DamageContext, e, rng)
	}
	if id == "job-1-entry-5" || id == "job-1-entry-6" {
		// Branches are alternatives; never consume a defense in both branches.
		low := 20.0
		if id == "job-1-entry-6" {
			low = float64(a.Attack) * 2
		}
		for _, power := range []float64{low, float64(a.Attack) * 3} {
			e.Payload.Number = power
			if err := p.ValidateDamage(&child.DamageContext, e, rng); err != nil {
				return err
			}
		}
		return nil
	}
	if id == "job-2-entry-6" {
		if a.HP <= 0 {
			return nil
		}
		return p.validateRepeatedDamage(&child.DamageContext, e, 2, rng)
	}
	if id == "job-1-entry-1" {
		child.AdditionalEffect = true
		if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{e}, rng); err != nil {
			return err
		}
		// A returned living target can enter a second physical conversion even
		// after the first helper cures the caster's category seal.
		return p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(e.TargetID, .15, CategoryPhysical), rng)
	}
	return p.ValidateDamage(&child.DamageContext, e, rng)
}

// ProjectBaseGuard carries exact unconditional writes into later admission.
func (s *LifetimeState) ProjectBaseGuard(c *BaseGuardContext, id string) (bool, error) {
	if err := s.ValidateBaseGuard(c, id, random.Default()); err != nil {
		return false, err
	}
	if tmp := baseGuardTemporary(id); tmp != "" {
		s.actor(c.ActorID).Temporary = tmp
		return true, nil
	}
	return false, nil
}
