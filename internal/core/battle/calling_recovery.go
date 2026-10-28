package battle

import "github.com/witchcraze/party2re/internal/core/random"

func (s *LifetimeState) applyCallingFairy(c *RosterContext, helperID string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{HelperID: helperID}
	if callingArcher(s.actor(helperID)) {
		if rng.Float64()*7 >= 1 {
			hits, err := s.ApplyAreaDamage(&c.DamageContext, float64(s.actor(helperID).Agility)*1.4, CategoryPhysical, false, rng)
			r.Effects, r.Applied = hits, len(hits) > 0
			return r, err
		}
		for _, target := range c.EnemyIDs {
			hit, err := s.ApplyDamage(&c.DamageContext, rosterEffect(target, float64(s.actor(helperID).Agility)*1.4, CategoryPhysical, false), rng)
			r.Effects = append(r.Effects, hit)
			if err != nil {
				return r, err
			}
			if hit.Returned && hit.TargetID != "" {
				heal, err := s.ApplyMPHealing(&c.EffectContext, rosterEffect(c.ActorID, float64(int(float64(hit.Amount)*.2)), CategoryPhysical, false), rng)
				r.Effects = append(r.Effects, heal)
				if err != nil {
					return r, err
				}
			}
		}
	} else {
		if rng.Float64()*6 >= 1 {
			r.Delegation = CallbackInvocation{Kind: "random"}
			return r, nil
		}
		heals, err := s.ApplyPartyHealing(&c.EffectContext, 300, CategoryUntyped, rng)
		r.Effects = append(r.Effects, heals...)
		if err != nil {
			return r, err
		}
		allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryUntyped, rng)
		if err != nil {
			return r, err
		}
		if allowed {
			c.PartyMPHealCalls++
			for _, target := range c.PartyIDs {
				heal, err := s.ApplyMPHealing(&c.EffectContext, rosterEffect(target, 200, CategoryUntyped, false), rng)
				r.Effects = append(r.Effects, heal)
				if err != nil {
					return r, err
				}
			}
		}
		for _, target := range c.PartyIDs {
			a := s.actor(target)
			if a.HP <= 0 {
				a.HP = int(float64(a.MaxHP) * .3)
				r.Applied = r.Applied || a.HP > 0
			}
		}
	}
	for _, v := range r.Effects {
		r.Applied = r.Applied || v.Applied
	}
	return r, nil
}

func (s *LifetimeState) applyCallingDrain(c *RosterContext, helperID string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{HelperID: helperID}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(v TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-v.MPDebit)
		applyStatusResources(s, v)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	hits := 2 + int(rng.Float64()*3)
	for range hits {
		if s.actor(c.ActorID).HP <= 0 {
			break
		}
		hit, err := s.ApplyDamage(&c.DamageContext, rosterEffect("", float64(s.actor(helperID).Agility)*1.2, CategoryPhysical, false), rng)
		r.Effects = append(r.Effects, hit)
		if err != nil {
			return r, err
		}
		if hit.Returned && hit.TargetID != "" {
			heal, err := s.ApplyHealing(&c.EffectContext, rosterEffect("", float64(hit.Amount)*.5, CategoryPhysical, false), rng)
			r.Effects = append(r.Effects, heal)
			if err != nil {
				return r, err
			}
		}
	}
	for _, v := range r.Effects {
		r.Applied = r.Applied || v.Applied
	}
	return r, nil
}
