package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateDualAgility shares the damage/status admission owners and never runs
// external reset/defeat hooks, consumes arguments, or draws committed RNG.
func (s *LifetimeState) ValidateDualAgility(c *DualAgilityContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, job := range []int{77, 83} {
		for _, temporary := range []bool{false, true} {
			for _, entry := range DualAgilityEntries(job, temporary) {
				known = known || entry.ID == id
			}
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p := s.callbackCopy()
	child := *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	if id == "job-83-entry-1" || id == "job-83-entry-4" {
		return nil
	}
	switch id {
	case "job-77-entry-0":
		child.ToolCategory = ToolCategoryContext{Source: "job77", SavedAttack: float64(a.Attack)}
		a.Attack = int(float64(a.Attack) * .75)
		return p.validateNormalStrikes(&child.DamageContext, child.shift(), 2, rng)
	case "job-83-entry-0":
		a.Agility *= 5
		a.Hit += 5
		return p.validateNormalStrikes(&child.DamageContext, child.shift(), 1, rng)
	case "job-83-entry-2":
		if child.Status == nil {
			return ErrUnsupportedEffect
		}
		status := *child.Status
		status.EffectContext = child.EffectContext
		return p.ValidateStatus(&status, TargetEffect{TargetID: child.shift(), Category: CategoryPhysical, Payload: CallbackPayload{Symbol: "death"}}, 19, false, true, rng)
	case "job-77-entry-1":
		first := TargetEffect{TargetID: child.shift(), Category: CategoryUntyped, Payload: CallbackPayload{Symbol: "tension"}}
		converted, _, err := a.ConvertEnemyCategory(child.TargetContext, first)
		if err != nil {
			return err
		}
		var effects []TargetEffect
		for _, candidate := range p.damageCandidates(&child.DamageContext, converted) {
			t := p.actor(candidate.TargetID)
			effects = append(effects, dualAgilityEffect(t.ID, float64(a.Attack)*(t.Tension+1)/2, CategoryPhysical, false))
		}
		// Only an unresolved first helper uses the absent-argument fallback.
		unresolved := len(effects) == 0
		if a.Status != StatusConfusion {
			target := p.actor(first.TargetID)
			if target != nil && target.TeamID != a.TeamID {
				unresolved = unresolved || target.HP <= 0
			} else {
				for _, target := range child.EnemyIDs {
					unresolved = unresolved || p.actor(target).HP <= 0
				}
			}
		}
		if unresolved {
			effects = append(effects, dualAgilityEffect("", float64(a.Attack)*.5, CategoryPhysical, false))
		}
		for _, effect := range effects { // These are alternatives, not a sequence.
			if err := p.ValidateDamage(&child.DamageContext, effect, rng); err != nil {
				return err
			}
		}
		return nil
	}
	if a.HP <= 0 && id != "job-83-entry-5" {
		return nil
	}
	count := 1
	e := dualAgilityEffect("", float64(a.Attack), CategoryPhysical, false)
	switch id {
	case "job-77-entry-2":
		e.Payload.Number = float64(a.Agility) * 1.5
		if err := p.validateAreaDamage(&child.DamageContext, e, rng); err != nil {
			return err
		}
		var effects []TargetEffect
		decay := !p.canPreserveAreaPower(&child.DamageContext, e)
		for range 3 {
			power := e.Payload.Number
			for _, target := range child.EnemyIDs {
				next := e
				next.TargetID, next.Payload.Number = target, power
				effects = append(effects, next)
				if decay {
					power *= .85
				}
			}
		}
		child.AreaDamageCalls++
		return p.validateDamageSequence(&child.DamageContext, effects, rng)
	case "job-77-entry-3":
		count, e.TargetID, e.Category = 2, child.shift(), CategoryUntyped
	case "job-77-entry-4":
		count = 3
	case "job-77-entry-4-temporary":
		a.Temporary, count = "", 6
		e.Payload.Number *= .55
		e.Arguments = []string{"1"}
	case "job-83-entry-3":
		count = 3
		e.Payload.Number *= .85
	case "job-83-entry-5":
		e.TargetID = child.shift()
		e.Payload.Number = max(float64(a.Attack)*.85, float64(a.Agility)*1.5)
		e.Arguments = []string{"1"}
	}
	effects := make([]TargetEffect, count)
	for i := range effects {
		effects[i] = e
		if id == "job-83-entry-3" {
			effects[i].TargetID = child.shift()
		}
	}
	return p.validateDamageSequence(&child.DamageContext, effects, rng)
}
