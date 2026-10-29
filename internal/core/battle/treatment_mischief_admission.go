package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateTreatmentMischief checks descendants on isolated state; it never
// invokes external Apply hooks, changes counters/arguments or consumes RNG.
func (s *LifetimeState) ValidateTreatmentMischief(c *TreatmentMischiefContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, job := range []int{50, 51, 55, 56, 57, 78} {
		for pos, entry := range TreatmentMischiefEntries(job) {
			known = known || entry.ID == id && treatmentMischiefOwns(job, pos)
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	if power := treatmentHealing(id); power != 0 {
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), power, CategoryMagic), rng)
	}
	if e, ok := treatmentAreaDamage(id, a); ok {
		return p.validateAreaDamage(&child.DamageContext, e, rng)
	}
	switch id {
	case "job-50-entry-1":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "accuracy", CategoryMagic), rng)
	case "job-50-entry-2":
		status := child.Status
		status.EffectContext = child.EffectContext
		return p.ValidateStatus(&status, controlSymbol(child.shift(), "death", CategoryMagic), 17, false, true, rng)
	case "job-50-entry-3":
		return p.validateTreatmentArmor(&child, rng)
	case "job-51-entry-0":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .2, CategoryMagic), rng); err != nil {
				return err
			}
		}
	case "job-51-entry-1", "job-55-entry-0", "job-55-entry-2", "job-56-entry-1":
		return nil
	case "job-51-entry-2":
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, 50, CategoryMagic), rng); err != nil {
				return err
			}
		}
	case "job-51-entry-3":
		return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{StatusConfusion, StatusSleep}, CategoryMagic, 50, rng)
	case "job-51-entry-6":
		child.AdditionalEffect = true
		status := child.Status
		status.EffectContext = child.EffectContext
		e := controlSymbol("", "death", CategoryMagic)
		if err := p.ValidateAreaStatus(&status, e, 10, false, true, rng); err != nil {
			return err
		}
		states, err := p.projectAreaDeath(&status, e, 10, false, true, rng)
		if err != nil {
			return err
		}
		for _, state := range states {
			if err := state.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 70, CategoryMagic, true), rng); err != nil {
				return err
			}
		}
	case "job-51-entry-7":
		if a.MaxMP < 0 || a.MaxMP > 1e12 {
			return ErrUnsupportedEffect
		}
		e := dualAgilityEffect(child.shift(), float64(a.MaxMP)*.7, CategoryMagic, true)
		if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{e}, rng); err != nil {
			return err
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 1, CategoryMagic), rng)
	case "job-55-entry-1", "job-57-entry-5":
		if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(child.shift(), .15, CategoryPhysical), rng); err != nil {
			return err
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 1, CategoryPhysical), rng)
	case "job-55-entry-3":
		for _, member := range p.Actors {
			if member.HP <= 0 || member.ID == child.ActorID {
				continue
			}
			if _, err := p.gemTensionCeiling(&child.EffectContext, p.actor(member.ID), 1); err != nil {
				return err
			}
		}
	case "job-55-entry-5":
		target := child.shift()
		targets := []string{target}
		if p.actor(target) == nil {
			var err error
			targets, err = p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(target, "tension", CategoryUntyped), false)
			if err != nil {
				return err
			}
		}
		for _, target := range targets {
			if target == "" {
				target = child.ActorID
			}
			if _, err := p.gemTensionCeiling(&child.EffectContext, p.actor(target), 1); err != nil {
				return err
			}
		}
	case "job-57-entry-1", "job-57-entry-3":
		return p.validateTreatmentArrow(&child, id, rng)
	case "job-57-entry-6":
		e := controlEffect("", .7, CategoryMagic)
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e.TargetID = target
			if err := p.validateStatUp(&child.EffectContext, e, StatAttack, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, StatAttack, &admission)
		}
	case "job-78-entry-3":
		a.Defense -= int(float64(a.Defense) * .2)
		return p.validatePartyTarget(child.TargetContext, controlEffect(a.ID, 200, CategoryUntyped), rng)
	}
	return nil
}

func (s *LifetimeState) validateTreatmentArmor(c *TreatmentMischiefContext, rng random.Generator) error {
	e := controlEffect(c.shift(), .4, CategoryMagic)
	if err := s.ValidateStatUp(&c.EffectContext, e, StatDefense, rng); err != nil {
		return err
	}
	// The unguarded temporary tail requires the stat helper's returned target.
	if s.actor(c.ActorID).categorySealed(e.Category) {
		return ErrUnsupportedEffect
	}
	for _, candidate := range s.partyTargetCandidates(c.TargetContext, e, nil) {
		t, a := s.actor(candidate.TargetID), s.actor(c.ActorID)
		if t.HP <= 0 || candidate.Payload.Number <= 0 {
			return ErrUnsupportedEffect
		}
		if float64(t.Defense) >= t.statUpCap(a, StatDefense) {
			if c.AdditionalEffect {
				return ErrUnsupportedEffect
			}
			continue
		}
		if t.Status == StatusSabaku {
			continue
		}
		v := s.finalEffectPower(&c.EffectContext, s.initialEffectPower(&c.EffectContext, candidate, ModifierStatUp))
		if t.statUpAmount(a, candidate.Category, StatDefense, v) == 0 {
			return ErrUnsupportedEffect
		}
	}
	return nil
}

func (s *LifetimeState) validateTreatmentArrow(c *TreatmentMischiefContext, id string, rng random.Generator) error {
	c.AdditionalEffect = true
	coefficient := 1.2
	if id == "job-57-entry-3" {
		coefficient = 1.1
	}
	e := dualAgilityEffect(c.shift(), float64(s.actor(c.ActorID).Attack)*coefficient, CategoryPhysical, false)
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return err
	}
	candidates := s.damageCandidates(&c.DamageContext, converted)
	if err := s.ValidateDamage(&c.DamageContext, e, rng); err != nil {
		return err
	}
	for _, candidate := range candidates {
		target := candidate.TargetID
		if s.actor(target).HP <= 0 {
			target = c.ActorID
		}
		if candidate.Payload.Number == 0 || s.actor(target).Temporary == TemporaryOneHitDefense || !damageHasPower(s.actor(c.ActorID), s.actor(target), candidate) {
			continue
		}
		p, child := s.callbackCopy(), *c
		// Each returned recipient keeps its own activation facts; alternative
		// random targets must not activate one another's equipment.
		child.DamageCalls++
		child.Item195Activated = child.Item195Activated || hasItem(p.actor(target).ItemDefinitionIDs, "item-195")
		child.Item185Activated = child.Item185Activated || hasItem(p.actor(target).ItemDefinitionIDs, "item-185")
		if p.actor(c.ActorID).categorySealed(converted.Category) {
			p.actor(c.ActorID).Status = ""
		}
		// A surviving hit can cross the status immunity boundary and consumes
		// tension before the returned-target helper resolves again.
		p.actor(target).HP = min(999, p.actor(target).HP)
		if !hasItem(p.actor(c.ActorID).ItemDefinitionIDs, "item-149") {
			p.actor(c.ActorID).Tension = 1
		}
		if id == "job-57-entry-1" {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .07, CategoryPhysical), rng); err != nil {
				return err
			}
		} else {
			status := child.Status
			status.EffectContext = child.EffectContext
			if err := p.ValidateStatus(&status, controlSymbol(target, StatusSleep, CategoryPhysical), 55, false, false, rng); err != nil {
				return err
			}
		}
	}
	return nil
}
