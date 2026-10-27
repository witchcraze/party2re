package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCOrderedResetContext shares the damage and equipment-reset owners. Ordinary
// NPC eligibility, listed costs and post-action tails remain consumer-owned.
type NPCOrderedResetContext struct {
	DamageContext
	EquipmentReset *EquipmentResetContext
	Arguments      []string
}

func (c *NPCOrderedResetContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func (s *LifetimeState) npcOrderedReset(c *NPCOrderedResetContext, target string, rng random.Generator) error {
	reset := *c.EquipmentReset
	reset.TargetContext, reset.Cursor = c.TargetContext, c.Cursor
	if err := s.ApplyEquipmentReset(&reset, target, rng); err != nil {
		return err
	}
	c.Cursor = reset.Cursor
	*c.EquipmentReset = reset
	return nil
}

// ApplyNPCOrderedReset executes the four intrinsic job403 bodies, preserving
// the source's distinct area helper, repeated hits and unguarded reset loop.
func (s *LifetimeState) ApplyNPCOrderedReset(c *NPCOrderedResetContext, id string, rng random.Generator) error {
	if err := s.ValidateNPCOrderedReset(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	switch id {
	case "npc-job-403-position-0":
		_, err := s.ApplyAreaDamage(&c.DamageContext, float64(s.actor(c.ActorID).Attack)*.6, CategoryUntyped, true, rng)
		return err
	case "npc-job-403-position-1":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .5, CategoryUntyped), StatDefense, rng)
		if err != nil {
			return err
		}
		s.actor(c.ActorID).Temporary = TemporaryReduceAttack
	case "npc-job-403-position-2":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "three-pronged-strike", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		target := ""
		if r.Resolved {
			target = r.TargetID
		}
		tension := s.actor(c.ActorID).Tension
		s.actor(c.ActorID).Tension = 1
		for hit := range 3 {
			a := s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			e := dualAgilityEffect("", float64(a.Attack)*.5, CategoryPhysical, true)
			if hit == 2 {
				a.Tension = tension
				e.TargetID, e.Payload.Number = target, float64(a.Attack)*.8
			}
			if _, err := s.ApplyDamage(&c.DamageContext, e, rng); err != nil {
				return err
			}
		}
	case "npc-job-403-position-3":
		for _, target := range c.EnemyIDs {
			if err := s.npcOrderedReset(c, target, rng); err != nil {
				return err
			}
			power := min(float64(s.actor(target).HP)*.5, 500)
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, power, CategoryUntyped, true), rng); err != nil {
				return err
			}
		}
		for _, target := range c.PartyIDs {
			if err := s.npcOrderedReset(c, target, rng); err != nil {
				return err
			}
		}
	}
	return nil
}
