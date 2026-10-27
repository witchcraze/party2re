package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNPCOrderedReset admits descendants on private state before any
// resource, argument, counter, reset hook or committed random draw changes.
func (s *LifetimeState) ValidateNPCOrderedReset(c *NPCOrderedResetContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	switch id {
	case "npc-job-403-position-0":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", float64(p.actor(c.ActorID).Attack)*.6, CategoryUntyped, true), rng)
	case "npc-job-403-position-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(c.ActorID, .5, CategoryUntyped), StatDefense, rng)
	case "npc-job-403-position-2":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "three-pronged-strike", CategoryUntyped), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			q, nested := p.callbackCopy(), child
			q.actor(c.ActorID).Tension = 1
			if q.actor(c.ActorID).HP <= 0 {
				continue
			}
			e := dualAgilityEffect("", float64(q.actor(c.ActorID).Attack)*.5, CategoryPhysical, true)
			if err := q.admitDamageSequence(&nested.DamageContext, []TargetEffect{e, e}, rng); err != nil {
				return err
			}
			q.actor(c.ActorID).Tension = s.actor(c.ActorID).Tension
			e.TargetID, e.Payload.Number = target, float64(q.actor(c.ActorID).Attack)*.8
			if err := q.ValidateDamage(&nested.DamageContext, e, rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-403-position-3":
		if c.EquipmentReset == nil {
			return ErrUnsupportedEffect
		}
		reset := *c.EquipmentReset
		reset.TargetContext = c.TargetContext
		for _, target := range append(slices.Clone(c.EnemyIDs), c.PartyIDs...) {
			if err := p.ValidateEquipmentReset(&reset, target, rng); err != nil {
				return err
			}
		}
		// Reset can heal HP or install a temporary before the damage amount
		// is read. Check both resource/stat extremes without external hooks.
		for _, boundary := range []resetBoundary{0, resetBoundary(math.Nextafter(1, 0))} {
			q, nested := p.callbackCopy(), child
			nestedReset := reset
			nested.EquipmentReset = &nestedReset
			for _, target := range nested.EnemyIDs {
				if err := q.npcOrderedReset(&nested, target, boundary); err != nil {
					return err
				}
				e := dualAgilityEffect(target, min(float64(q.actor(target).HP)*.5, 500), CategoryUntyped, true)
				if err := q.admitDamageSequence(&nested.DamageContext, []TargetEffect{e}, rng); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}
