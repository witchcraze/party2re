package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateMachineFamiliar checks complete descendants on private facts, before
// argument consumption, costs, committed RNG or external effect hooks.
func (s *LifetimeState) ValidateMachineFamiliar(c *MachineFamiliarContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	switch id {
	case "job-79-entry-0":
		if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(child.shift(), .25, CategoryMagic), rng); err != nil {
			return err
		}
		if a.categorySealed(CategoryMagic) {
			a.Status = ""
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(c.ActorID, 1, CategoryMagic), rng)
	case "job-79-entry-1":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), float64(a.Agility)*2.2, CategoryPhysical, false), rng)
	case "job-81-entry-0":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), float64(a.Attack)*1.2, CategoryPhysical, false), rng)
	case "job-81-entry-3":
		target := child.shift()
		for _, power := range []float64{float64(a.Attack) * 3, 20} {
			if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(target, power, CategoryPhysical, false), rng); err != nil {
				return err
			}
		}
	case "job-79-entry-4":
		e := dualAgilityEffect(child.shift(), 220, CategoryPhysical, true)
		if err := p.ValidateDamage(&child.DamageContext, e, rng); err != nil {
			return err
		}
		returned, err := p.crownHitCanReturn(&DuelCrownContext{DamageContext: child.DamageContext}, e)
		if err != nil || !returned {
			return err
		}
		// A returned hit consumes tension unless149 retains it. Two calls
		// include the lethal tail and each call's optional145 recursion.
		if !hasItem(a.ItemDefinitionIDs, "item-149") {
			a.Tension = 1
		}
		_, err = p.gemTensionCeiling(&child.EffectContext, a, 2)
		return err
	case "job-81-entry-4":
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			if p.actor(target).HP <= 0 {
				continue
			}
			for _, stat := range []BattleStat{StatAttack, StatDefense} {
				e := controlEffect(target, .2, CategorySong)
				if err := p.validateStatUp(&child.EffectContext, e, stat, rng, admission); err != nil {
					return err
				}
				p.recordStatUp205(&child.EffectContext, e, stat, &admission)
			}
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, 150, CategorySong), rng); err != nil {
				return err
			}
		}
	case "job-82-entry-1":
		a.Attack = int(float64(a.Attack) * 1.2)
		return p.validateNormalStrikes(&child.DamageContext, child.shift(), 1, rng)
	case "job-82-entry-2":
		e := controlSymbol(child.shift(), "tea", CategoryMagic)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		if a.categorySealed(CategoryMagic) {
			a.Status = ""
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(candidate.TargetID, 150, CategoryMagic), rng); err != nil {
				return err
			}
			if err := p.validatePartyTarget(child.TargetContext, controlSymbol(candidate.TargetID, "cure", CategoryMagic), rng); err != nil {
				return err
			}
		}
	case "job-82-entry-3":
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			if err := p.ValidateStatUp(&child.EffectContext, controlEffect(c.ActorID, .18, CategoryUntyped), stat, rng); err != nil {
				return err
			}
		}
	case "job-82-entry-4":
		if a.HP <= 0 {
			return nil
		}
		e := dualAgilityEffect("", float64(a.Attack)*.9, CategoryPhysical, false)
		for range 4 {
			returned, err := p.crownHitCanReturn(&DuelCrownContext{DamageContext: child.DamageContext}, e)
			if err != nil {
				return err
			}
			if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{e}, rng); err != nil {
				return err
			}
			if !returned {
				return nil
			}
		}
	case "job-82-entry-5":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), 170, CategoryBreath, true), rng)
	default:
		return ErrUnsupportedEffect
	}
	return nil
}
