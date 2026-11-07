package battle

import "github.com/witchcraze/party2re/internal/core/random"

// validateEnhancedTool admits only the branch the current hand reaches: helper
// facts are purchase-only and reset owners are item13/87-only. An unknown hand
// rejects before the use-versus-purchase choice; an NPC never touches the durable
// hand, so that fact is required only for a player actor.
func (s *LifetimeState) validateEnhancedTool(c *JobAssetContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	if a.HeldToolID == nil {
		return ErrUnsupportedEffect
	}
	if !a.NPC && (c.Assets.HeldToolID == nil || *c.Assets.HeldToolID != *a.HeldToolID) {
		return ErrUnsupportedEffect
	}
	if *a.HeldToolID == "" {
		if c.Assets.HelperToolIDs == nil {
			return ErrUnsupportedEffect
		}
		for _, number := range enhancedPurchasePool {
			if _, err := enhancedPurchaseCost(number); err != nil {
				return err
			}
		}
		return nil
	}
	number, ok := enhancedToolNumber(*a.HeldToolID)
	if !ok {
		return nil // Accepted no-effect, without consumption.
	}
	p, child := s.callbackCopy(), *c
	child.ActorNPC = a.NPC
	if child.DefeatContext != nil {
		child.DamageContext = p.nativeDamageContext(child.DamageContext, child.DefeatContext, rng)
	}
	argument := p.numericMember(number)
	self := controlEffect(argument, 1, CategoryUntyped)
	enemy := func(target string) error {
		_, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(target, enhancedToolSymbol, CategoryUntyped), false)
		return err
	}
	switch number {
	case 1, 2, 3, 11, 12:
		return p.validatePartyTarget(c.TargetContext, self, rng)
	case 4, 5:
		for _, id := range c.PartyIDs {
			if err := p.validatePartyTarget(c.TargetContext, controlEffect(id, 1, CategoryUntyped), rng); err != nil {
				return err
			}
		}
		return nil
	case 6:
		return p.ValidateRevival(&RevivalContext{TargetContext: c.TargetContext}, "tool-6", argument, rng)
	case 13:
		e := controlSymbol(argument, enhancedToolSymbol, CategoryUntyped)
		if err := p.validatePartyTarget(c.TargetContext, e, rng); err != nil {
			return err
		}
		for _, candidate := range p.partyTargetCandidates(c.TargetContext, e, nil) {
			if err := p.validateEnhancedReset(c, candidate.TargetID, rng); err != nil {
				return err
			}
		}
		return nil
	case 14:
		return p.validatePartyTarget(c.TargetContext, controlSymbol(argument, enhancedToolSymbol, CategoryUntyped), rng)
	case 15:
		if err := enemy(argument); err != nil {
			return err
		}
		// Actual power is the resolved target's attack; one admits every candidate.
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect("", 1, CategoryUntyped, true), rng)
	case 18, 19, 20:
		return p.ValidateStatUp(&child.EffectContext, self, enhancedToolStat(number), rng)
	case 41:
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 400, CategoryUntyped, true), rng)
	case 42:
		status := StatusContext{EffectContext: child.EffectContext, Defeat: child.DefeatContext}
		return p.ValidateAreaStatus(&status, controlSymbol("", "instant_death", CategoryUntyped), 40, false, true, rng)
	case 78:
		if c.Roster == nil {
			return ErrUnsupportedEffect
		}
		roster := *c.Roster
		roster.TargetContext = c.TargetContext
		return p.validateEnemySummon(&roster, "tool-78")
	case 9, 80:
		return nil
	case 81:
		return enemy(argument)
	case 84:
		if err := enemy(argument); err != nil {
			return err
		}
		if err := enemy(""); err != nil || len(c.EnemyIDs) == 0 {
			return err
		}
		return p.validatePartyTarget(c.TargetContext, controlEffect(c.ActorID, 1, CategoryPhysical), rng)
	case 87:
		if argument != "" {
			return p.validateEnhancedReset(c, argument, rng)
		}
		for _, member := range p.Actors {
			if err := p.validateEnhancedReset(c, member.ID, rng); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateEnhancedReset(c *JobAssetContext, target string, rng random.Generator) error {
	if c.DefeatContext == nil {
		return ErrUnsupportedEffect
	}
	reset := c.DefeatContext.EquipmentResetContext
	reset.TargetContext = c.TargetContext
	return s.ValidateEquipmentReset(&reset, target, rng)
}
