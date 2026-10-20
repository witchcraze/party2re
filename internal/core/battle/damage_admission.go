package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Admission follows helper invocation order on a private projection. Only
// defenses reachable in an earlier invocation can be consumed on a later one.
func (s *LifetimeState) validateDamageSequence(c *DamageContext, effects []TargetEffect, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	context := *c
	for _, e := range effects {
		if err := projected.ValidateDamage(&context, e, rng); err != nil {
			return err
		}
		a := projected.actor(c.ActorID)
		converted, _, err := a.ConvertEnemyCategory(context.TargetContext, e)
		if err != nil {
			return err
		}
		var blocked []string
		activate := false
		for _, candidate := range projected.damageCandidates(&context, converted) {
			if candidate.Payload.Number == 0 {
				continue
			}
			t := projected.actor(candidate.TargetID)
			if t.HP <= 0 {
				t = a
			}
			if t.Temporary == TemporaryOneHitDefense {
				blocked = append(blocked, t.ID)
				continue
			}
			if damageHasPower(a, t, candidate) && hasItem(t.ItemDefinitionIDs, "item-195") {
				activate = true
			}
		}
		// Candidate alternatives belong to one invocation, not repeated hits.
		for _, id := range blocked {
			projected.actor(id).Temporary = ""
		}
		if len(context.EnemyIDs) > 0 && a.categorySealed(converted.Category) {
			a.Status = ""
		}
		context.Item195Activated = context.Item195Activated || activate
	}
	return nil
}

func (s *LifetimeState) validateRepeatedDamage(c *DamageContext, e TargetEffect, hits int, rng random.Generator) error {
	effects := make([]TargetEffect, hits)
	for i := range effects {
		effects[i] = e
	}
	return s.validateDamageSequence(c, effects, rng)
}

func (s *LifetimeState) validateAreaDamage(c *DamageContext, e TargetEffect, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateRecovery(&c.EffectContext, e, rng); err != nil {
		return err
	}
	projected := *s
	projected.Actors = slices.Clone(s.Actors)
	context := *c
	a := projected.actor(c.ActorID)
	if a.categorySealed(e.Category) {
		a.Status = ""
	}
	decay := !projected.canPreserveAreaPower(&context, e)
	power := e.Payload.Number
	if a.Extended == ExtendedAdditional && e.Category == CategoryMagic {
		power *= 1.5
	}
	context.AreaDamageCalls++
	var effects []TargetEffect
	if a.HP > 0 {
		for _, id := range c.EnemyIDs {
			next := e
			next.TargetID, next.Payload.Number = id, power
			effects = append(effects, next)
			if decay {
				power *= .85
			}
		}
	}
	return projected.validateDamageSequence(&context, effects, rng)
}

func (s *LifetimeState) canPreserveAreaPower(c *DamageContext, e TargetEffect) bool {
	if !hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-144") {
		return false
	}
	direct, _ := damageFlags(e)
	// Two optional weapon draws can cross144's nonmonotone300 threshold.
	for mask := 0; mask < 4; mask++ {
		bits := mask
		draw := func() float64 { value := float64(bits&1) * .5; bits >>= 1; return value }
		if s.modifyDamage(c, nil, e.Payload.Number, e.Category, direct, false, draw) <= 300 {
			return true
		}
	}
	return false
}

// Damage candidates use the enemy helper's predicates, then the same one-pass
// temporary continuations as ally effects. No selection/cure draw is consumed.
func (s *LifetimeState) damageCandidates(c *DamageContext, e TargetEffect) []TargetEffect {
	return s.damageCandidatesAfter(c, e, nil)
}

func (s *LifetimeState) damageInitialTargetIDs(c *DamageContext, e TargetEffect) []string {
	a := s.actor(c.ActorID)
	var ids []string
	if a.Status != StatusConfusion {
		if target := s.actor(e.TargetID); target != nil && target.TeamID != a.TeamID {
			if s.actor(e.TargetID).HP > 0 {
				ids = append(ids, e.TargetID)
			}
		} else {
			for _, id := range c.EnemyIDs {
				if s.actor(id).HP > 0 {
					ids = append(ids, id)
				}
			}
		}
	}
	if a.Status == StatusConfusion || (c.Item195Activated && a.Status == "") {
		if e.TargetID == a.ID && !s.SpecialBossField {
			ids = append(ids, a.ID)
		} else {
			for _, member := range s.Actors {
				ids = append(ids, member.ID)
			}
		}
	}
	return ids
}

func (s *LifetimeState) damageCandidatesAfter(c *DamageContext, e TargetEffect, mayReplace map[string]float64) []TargetEffect {
	ids := s.damageInitialTargetIDs(c, e)
	var candidates []TargetEffect
	a := s.actor(c.ActorID)
	if a.Temporary == TemporaryPhantom {
		for _, id := range ids {
			candidate := e
			candidate.TargetID = id
			candidates = append(candidates, candidate)
		}
	} else {
		candidates = s.temporaryTargetCandidates(c.TargetContext, e, ids, mayReplace)
	}
	return s.phantomTargetCandidates(c.TargetContext, candidates)
}

func (s *LifetimeState) phantomTargetCandidates(c TargetContext, candidates []TargetEffect) []TargetEffect {
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
