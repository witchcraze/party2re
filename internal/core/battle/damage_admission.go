package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Earlier hits can consume one-hit defense before a later redirected hit.
// Project that change before any area/multi-hit draw or modifier evaluation.
func (s *LifetimeState) validateRepeatedDamage(c *DamageContext, e TargetEffect, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	context := *c
	if projected.actor(c.ActorID).categorySealed(e.Category) {
		projected.actor(c.ActorID).Status = ""
	}
	for i := range projected.Actors {
		if projected.Actors[i].Temporary == TemporaryOneHitDefense {
			projected.Actors[i].Temporary = ""
		}
	}
	converted, _, err := projected.actor(c.ActorID).ConvertEnemyCategory(context.TargetContext, e)
	if err != nil {
		return err
	}
	for _, candidate := range projected.damageCandidates(&context, converted) {
		t := projected.actor(candidate.TargetID)
		if t.HP <= 0 {
			t = projected.actor(c.ActorID)
		}
		if damageHasPower(projected.actor(c.ActorID), t, candidate) && hasItem(t.ItemDefinitionIDs, "item-195") {
			context.Item195Activated = true
		}
	}
	return projected.ValidateDamage(&context, e, rng)
}

func (s *LifetimeState) validateAreaDamage(c *DamageContext, e TargetEffect, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if len(c.EnemyIDs) > 1 {
		return s.validateRepeatedDamage(c, e, rng)
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	if projected.actor(c.ActorID).categorySealed(e.Category) {
		projected.actor(c.ActorID).Status = ""
	}
	return projected.ValidateDamage(c, e, rng)
}

// Damage candidates use the enemy helper's predicates, then the same one-pass
// temporary continuations as ally effects. No selection/cure draw is consumed.
func (s *LifetimeState) damageCandidates(c *DamageContext, e TargetEffect) []TargetEffect {
	a := s.actor(c.ActorID)
	var ids []string
	if a.Status == StatusConfusion || (c.Item195Activated && a.Status == "") {
		for _, a := range s.Actors {
			ids = append(ids, a.ID)
		}
	} else if slices.Contains(c.EnemyIDs, e.TargetID) {
		if s.actor(e.TargetID).HP > 0 {
			ids = []string{e.TargetID}
		}
	} else {
		for _, id := range c.EnemyIDs {
			if s.actor(id).HP > 0 {
				ids = append(ids, id)
			}
		}
	}
	var candidates []TargetEffect
	if a.Temporary == TemporaryPhantom {
		for _, id := range ids {
			candidate := e
			candidate.TargetID = id
			candidates = append(candidates, candidate)
		}
	} else {
		candidates = s.temporaryTargetCandidates(c.TargetContext, e, ids, nil)
	}
	if s.Fields.Current() == "phantom" && c.PhantomTargetID != "" && len(c.PhantomEnemyIDs) > 0 {
		var redirected []TargetEffect
		for _, candidate := range candidates {
			if candidate.TargetID == c.PhantomTargetID {
				for _, id := range c.PhantomEnemyIDs {
					next := candidate
					next.TargetID = id
					redirected = append(redirected, next)
				}
			} else {
				redirected = append(redirected, candidate)
			}
		}
		candidates = redirected
	}
	return candidates
}
