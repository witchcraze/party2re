package battle

import "github.com/witchcraze/party2re/internal/core/random"

// Admit every intrinsic branch without sampling; child invocation admission is
// composed by RosterAction using the complete catalog and the same input frame.
func (s *LifetimeState) validateCallingBody(c *RosterContext, job int, helperID, targetID string, rng random.Generator) error {
	p, child := s.callbackCopy(), *c
	h := p.actor(helperID)
	status := func(value string, category CallbackCategory, chance float64, area, death bool) error {
		e := controlSymbol(targetID, value, category)
		sc := callingStatusContext(&child)
		if area {
			if !death {
				return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{value}, category, chance, rng)
			}
			return p.ValidateAreaStatus(&sc, e, chance, false, death, rng)
		}
		return p.ValidateStatus(&sc, e, chance, false, death, rng)
	}
	switch job {
	case 61:
		if err := status("instant-death", CategoryMagic, 22, true, true); err != nil {
			return err
		}
		child.AdditionalEffect = true
		if err := p.validateAreaDamageStatus(&child.DamageContext, 120, StatusDeadlyPoison, 40, rng); err != nil {
			return err
		}
		child.AdditionalEffect = c.AdditionalEffect
		return status(StatusDeadlyPoison, CategoryBreath, 70, true, false)
	case 62:
		if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{rosterEffect(targetID, 100, CategoryMagic, true)}, rng); err != nil {
			return err
		}
		if err := p.validatePartyTarget(child.TargetContext, rosterEffect(helperID, 100, CategoryMagic, false), rng); err != nil {
			return err
		}
		// MP drain and return have no damage or defeat tail; reuse their target
		// contracts rather than introducing a second resource-operation port.
		p, child = s.callbackCopy(), *c
		if err := p.validateRecovery(&child.EffectContext, rosterEffect(targetID, .3, CategoryMagic, false), rng); err != nil {
			return err
		}
		if _, _, err := p.actor(c.ActorID).ConvertEnemyCategory(child.TargetContext, rosterEffect(targetID, .3, CategoryMagic, false)); err != nil {
			return err
		}
		if err := p.validatePartyTarget(child.TargetContext, rosterEffect(child.ActorID, 150, CategoryMagic, false), rng); err != nil {
			return err
		}
		return status(StatusConfusion, CategorySong, 70, true, false)
	case 63:
		for i, value := range []string{StatusDeadlyPoison, StatusParalyze, StatusSleep} {
			chance := 50.0
			if i == 0 {
				chance = 90
			}
			if err := status(value, CategoryBreath, chance, true, false); err != nil {
				return err
			}
		}
	case 64:
		if err := p.ValidateControlClimax(&ControlClimaxContext{DamageContext: child.DamageContext, Arguments: []string{targetID}}, "job-53-entry-5", rng); err != nil {
			return err
		}
		if err := status(StatusPhysicalSeal, CategoryUntyped, 80, false, false); err != nil {
			return err
		}
		return status(StatusDofuu, CategoryPhysical, 60, true, false)
	case 66:
		if err := status("instant-death", CategoryDance, 20, true, true); err != nil {
			return err
		}
		if err := status("instant-death", CategoryPhysical, 40, false, true); err != nil {
			return err
		}
		return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{StatusPhysicalSeal, StatusDofuu}, CategoryMagic, 75, rng)
	case 67:
		switch callingAppearance(h, 67) {
		case 0:
			return status(StatusDeadlyPoison, CategoryBreath, 80, true, false)
		case 1:
			return p.validateAreaDamage(&child.DamageContext, rosterEffect("", 220, CategoryBreath, true), rng)
		default:
			child.AdditionalEffect = true
			return p.validateAreaDamageStatus(&child.DamageContext, 150, StatusParalyze, 20, rng)
		}
	case 71:
		if err := p.ValidateDamage(&child.DamageContext, rosterEffect(targetID, float64(h.Attack)*1.4, CategoryPhysical, false), rng); err != nil {
			return err
		}
		if err := p.validatePartyTarget(child.TargetContext, rosterEffect(targetID, 150, CategoryMagic, false), rng); err != nil {
			return err
		}
		return status(StatusDofuu, CategoryDance, 80, true, false)
	case 80:
		if callingArcher(h) {
			if err := p.validateAreaDamage(&child.DamageContext, rosterEffect("", float64(h.Agility)*1.4, CategoryPhysical, false), rng); err != nil {
				return err
			}
			for _, target := range child.EnemyIDs {
				if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{rosterEffect(target, float64(h.Agility)*1.4, CategoryPhysical, false)}, rng); err != nil {
					return err
				}
				if err := p.validatePartyTarget(child.TargetContext, rosterEffect(child.ActorID, 1, CategoryPhysical, false), rng); err != nil {
					return err
				}
			}
			return nil
		}
		return p.validateRosterHealing(&child, 300, CategoryUntyped, rng)
	case 87:
		for _, power := range []float64{float64(h.Agility) * 2, float64(h.Attack) * 1.2} {
			if err := p.ValidateDamage(&child.DamageContext, rosterEffect(targetID, power, CategoryPhysical, false), rng); err != nil {
				return err
			}
		}
		if err := p.ValidateStatUp(&child.EffectContext, rosterEffect(helperID, 1, CategoryPhysical, false), StatAttack, rng); err != nil {
			return err
		}
		// The buff can cure a physical seal before the explicit untyped status.
		if err := status(StatusDofuu, CategoryUntyped, 75, false, false); err != nil {
			return err
		}
		if p.actor(c.ActorID).categorySealed(CategoryPhysical) {
			p.actor(c.ActorID).Status = ""
		}
		return status(StatusDofuu, CategoryUntyped, 75, false, false)
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

func (s *LifetimeState) validateCallingDrain(c *RosterContext, helperID string, rng random.Generator) error {
	p, child := s.callbackCopy(), *c
	if p.actor(c.ActorID).HP <= 0 {
		return nil
	}
	for range 4 {
		if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{rosterEffect("", float64(p.actor(helperID).Agility)*1.2, CategoryPhysical, false)}, rng); err != nil {
			return err
		}
		if err := p.validatePartyTarget(child.TargetContext, rosterEffect("", 1, CategoryPhysical, false), rng); err != nil {
			return err
		}
	}
	return nil
}
