package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateTreatmentChance admits every reachable descendant on private facts,
// before costs, arguments, counters, external Apply hooks or committed RNG.
func (s *LifetimeState) ValidateTreatmentChance(c *TreatmentChanceContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	child.Status.EffectContext = child.EffectContext
	if symbol, category := treatmentChanceCure(id); symbol != "" {
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlSymbol(target, symbol, category), rng); err != nil {
				return err
			}
		}
		return nil
	}
	switch id {
	case "job-43-entry-3", "job-45-entry-5":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "regeneration", CategoryMagic), rng)
	case "job-43-entry-4":
		e := controlSymbol(child.shift(), "cure", CategoryUntyped)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			t := p.actor(candidate.TargetID)
			if err := p.validatePartyTarget(child.TargetContext, controlSymbol(t.ID, t.Status, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-43-entry-5":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), 400, CategoryMagic), rng)
	case "job-45-entry-0":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "accuracy", CategoryUntyped), rng)
	case "job-45-entry-1":
		return p.ValidateStatus(&child.Status, controlSymbol(child.shift(), StatusDofuu, CategoryMagic), 85, false, false, rng)
	case "job-45-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .5, CategoryUntyped), StatDefense, rng)
	case "job-45-entry-3", "job-45-entry-4":
		return p.validateChanceStatSequence(&child, id, rng)
	case "job-45-entry-6":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), 300, CategoryPhysical, true), rng)
	case "job-45-entry-7":
		return p.validateRestorationDance(&child, rng)
	case "job-46-entry-0":
		for _, power := range []float64{200, 400} {
			for _, target := range child.PartyIDs {
				if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, power, CategoryUntyped), rng); err != nil {
					return err
				}
			}
		}
		// All mismatch bases are positive; minimum7 and maximum77 straddle
		// modifier thresholds differently from the two non-direct triples.
		for _, power := range []float64{7, 77, 300, 500} {
			if err := p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", power, CategoryUntyped, power < 100), rng); err != nil {
				return err
			}
		}
		return p.ValidateAreaStatus(&child.Status, controlSymbol("", "death", CategoryUntyped), 90, false, true, rng)
	case "job-46-entry-1":
		return p.ValidateStatus(&child.Status, controlSymbol(child.shift(), "death", CategoryPhysical), 20, false, true, rng)
	case "job-46-entry-2", "job-46-entry-4":
		target := child.shift()
		firstSides := 6
		if id == "job-46-entry-4" {
			firstSides = 3
		}
		for x := 1; x <= firstSides; x++ {
			for y := 1; y <= 6; y++ {
				power := x*100 + y*10 + x
				if id == "job-46-entry-2" {
					power /= 2
				}
				if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(target, float64(power), CategoryMagic, true), rng); err != nil {
					return err
				}
			}
		}
		return nil
	case "job-46-entry-3":
		for _, target := range p.Actors {
			if target.HP <= 0 {
				target = *p.actor(child.ActorID)
			}
			if target.HP <= 999 && target.Defense <= 999 {
				if child.Status.Defeat == nil {
					return ErrUnsupportedEffect
				}
				defeat := *child.Status.Defeat
				defeat.TargetContext = child.TargetContext
				if err := p.ValidateDefeat(&defeat, target.ID, rng); err != nil {
					return err
				}
			}
		}
		return nil
	case "job-46-entry-5":
		return p.validateChanceCoin(&child, 0, rng)
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateChanceStatSequence(c *TreatmentChanceContext, id string, rng random.Generator) error {
	coefficient, category := .18, CategoryDance
	if id == "job-45-entry-4" {
		coefficient, category = .4, CategorySong
	}
	e := controlEffect(c.shift(), coefficient, category)
	control := ControlClimaxContext{DamageContext: c.DamageContext}
	if err := s.validateControlDown(&control, e, rng); err != nil {
		return err
	}
	if id == "job-45-entry-3" {
		if s.actor(c.ActorID).categorySealed(category) {
			s.actor(c.ActorID).Status = ""
		}
		return s.validatePartyTarget(c.TargetContext, controlEffect(c.ActorID, 1, category), rng)
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return err
	}
	for _, candidate := range s.damageCandidates(&c.DamageContext, converted) {
		if err := s.validateControlDown(&control, controlEffect(candidate.TargetID, .4, category), rng); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) validateRestorationDance(c *TreatmentChanceContext, rng random.Generator) error {
	if s.actor(c.ActorID).categorySealed(CategoryDance) {
		s.actor(c.ActorID).Status = ""
	}
	for _, target := range c.PartyIDs {
		t := s.actor(target)
		ceiling, err := s.gemTensionCeiling(&c.EffectContext, t, 1)
		if err != nil {
			return err
		}
		t.Tension = ceiling
		// Earlier recovery can change a later ally's HP/MP comparison; both
		// branches use the same target path and require no extra operation port.
		if err := s.validatePartyTarget(c.TargetContext, controlEffect(target, 100, CategoryDance), rng); err != nil {
			return err
		}
		if err := s.validatePartyTarget(c.TargetContext, controlEffect(target, 40, CategoryDance), rng); err != nil {
			return err
		}
	}
	return nil
}

// Coin iteration keeps death/reset/reward alternatives separate. A successful
// earlier death can change a later participant's HP or base defense predicate.
func (s *LifetimeState) validateChanceCoin(c *TreatmentChanceContext, index int, rng random.Generator) error {
	if index == len(c.EnemyIDs) {
		return nil
	}
	target := c.EnemyIDs[index]
	t := s.actor(target)
	if t.HP <= 999 && t.BaseStats.Defense <= 999 {
		states, err := s.projectDeathTargets(&c.Status, controlSymbol(target, "death", CategoryUntyped), 60, false, index+1 < len(c.EnemyIDs), []string{target}, rng)
		if err != nil {
			return err
		}
		for _, state := range states {
			child := *c
			if err := state.validateChanceCoin(&child, index+1, rng); err != nil {
				return err
			}
		}
		return nil
	}
	// The dodge branch always remains possible for either half-HP effect.
	if err := s.validateChanceCoin(c, index+1, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	if t.HP > 999 {
		e := dualAgilityEffect(target, float64(min(1800, t.MaxHP/2)), CategoryUntyped, true)
		states, err := p.projectDirectDamage(&child.DamageContext, child.Status.Defeat, e, e.Payload.Number, index+1 < len(c.EnemyIDs), rng)
		if err != nil {
			return err
		}
		for _, next := range states {
			tail := child
			tail.DamageContext = next.Context
			tail.Status.EffectContext = tail.EffectContext
			if err := next.State.validateChanceCoin(&tail, index+1, rng); err != nil {
				return err
			}
		}
		return nil
	} else {
		p.actor(target).HP = max(1, t.HP-t.HP/2)
	}
	return p.validateChanceCoin(&child, index+1, rng)
}
