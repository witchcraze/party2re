package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateMartial admits descendants without consuming state, arguments or RNG.
func (s *LifetimeState) ValidateMartial(c *MartialContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, job := range []int{4, 23, 25} {
		for _, entry := range MartialEntries(job) {
			known = known || entry.ID == id
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	if applyMartialInline(&p, &child, id) {
		return nil
	}
	a := p.actor(c.ActorID)
	switch id {
	case "job-4-entry-0", "job-25-entry-0", "job-25-entry-5":
		power, direct := float64(a.Attack)*.75, false
		if id == "job-25-entry-0" {
			power = float64(a.Attack) * .8
		} else if id == "job-25-entry-5" {
			power, direct = 120, true
		}
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", power, CategoryPhysical, direct), rng)
	case "job-4-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .4, CategoryPhysical), StatAgility, rng)
	case "job-4-entry-8", "job-23-entry-1":
		e := controlEffect(child.ActorID, .4, CategoryPhysical)
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, stat := range []BattleStat{StatAttack, StatDefense} {
			if err := p.validateStatUp(&child.EffectContext, e, stat, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, stat, &admission)
		}
		return nil
	case "job-4-entry-4", "job-4-entry-5":
		if child.Status == nil {
			return ErrUnsupportedEffect
		}
		status := *child.Status
		status.EffectContext = child.EffectContext
		symbol, chance, death := StatusDofuu, 75.0, false
		if id == "job-4-entry-5" {
			symbol, chance, death = "death", 19, true
		}
		return p.ValidateStatus(&status, controlSymbol(child.shift(), symbol, CategoryPhysical), chance, false, death, rng)
	case "job-25-entry-1":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 100, CategoryPhysical), rng)
	}
	count := 1
	if id == "job-4-entry-7" {
		count = 5 // Admit the largest count before drawing the actual3..5.
	} else if id == "job-23-entry-3" {
		count = 2
	}
	if count > 1 && a.HP <= 0 {
		return nil
	}
	var effects []TargetEffect
	for range count {
		effects = append(effects, martialDamage(&child, a, id))
	}
	// The drain's returned positive amount heals the known actor with the same
	// physical category; it adds no context beyond a reachable damage helper.
	return p.admitDamageSequence(&child.DamageContext, effects, rng)
}

// ProjectMartial carries only exact inline transitions to later admission.
func (s *LifetimeState) ProjectMartial(c *MartialContext, id string) (bool, error) {
	if err := s.ValidateMartial(c, id, random.Default()); err != nil {
		return false, err
	}
	return applyMartialInline(s, c, id), nil
}
