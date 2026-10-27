package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNPCJob98 shares admission between discovery and execution. Private
// projections retain earlier seal cures, one-use defenses and vulnerability.
func (s *LifetimeState) ValidateNPCJob98(c *NPCJob98Context, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	switch id {
	case "npc-job-98-position-0", "npc-job-98-position-5":
		_, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "life-to-one", CategoryMagic), false)
		return err
	case "npc-job-98-position-1":
		return p.ValidateStatusEntry(&StatusContext{EffectContext: child.EffectContext}, child.EquipmentReset, "job-35-entry-1", "", rng)
	case "npc-job-98-position-2":
		return nil // Only a category cure and living-party temporary writes.
	case "npc-job-98-position-3":
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e := controlEffect(target, .5, CategoryMagic)
			if err := p.validateStatUp(&child.EffectContext, e, StatDefense, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, StatDefense, &admission)
		}
		return nil
	case "npc-job-98-position-4":
		_, err := p.gemTensionCeiling(&child.EffectContext, p.actor(child.ActorID), 2)
		return err
	case "npc-job-98-position-6":
		if err := p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 200, CategoryMagic, true), rng); err != nil {
			return err
		}
		// Healing uses explicit party IDs, so even post-hit confusion/death has
		// no absent target or physical deflection branch requiring extra facts.
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, 200, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-98-position-7":
		for _, target := range child.EnemyIDs {
			if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{dualAgilityEffect(target, 100, CategoryBreath, true)}, rng); err != nil {
				return err
			}
			p.actor(target).Temporary = TemporaryDouble
		}
		return nil
	case "npc-job-98-position-8":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 350, CategoryBreath, true), rng)
	case "npc-job-98-position-9":
		if p.actor(child.ActorID).HP <= 0 {
			return nil
		}
		return p.validateRepeatedDamage(&child.DamageContext, dualAgilityEffect("", 250, CategoryMagic, true), 6, rng)
	case "npc-job-98-position-10":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 400, CategoryMagic, true), rng)
	}
	return ErrUnsupportedEffect
}
