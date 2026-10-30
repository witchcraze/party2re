package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateAdvancedMagic admits complete descendants on private facts before
// argument consumption, incidental resource writes, external hooks or RNG.
func (s *LifetimeState) ValidateAdvancedMagic(c *AdvancedMagicContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	child.Status.EffectContext = child.EffectContext
	if tmp := advancedMagicTemporary(id); tmp != "" {
		e := controlSymbol(child.shift(), "temporary:"+string(tmp), CategoryMagic)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		if id != "job-16-entry-8" {
			return nil
		}
		// No source early return: a failed seal check writes an empty record.
		if p.actor(child.ActorID).categorySealed(CategoryMagic) {
			return ErrUnsupportedEffect
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			next := p.callbackCopy()
			next.actor(candidate.TargetID).Temporary = TemporaryNullAttack
			if err := next.validateTimePartyHealing(&TimeMixedMagicContext{DamageContext: child.DamageContext}, 300, rng); err != nil {
				return err
			}
		}
		return nil
	}
	switch id {
	case "job-15-entry-0":
		child.AdditionalEffect = true
		e := dualAgilityEffect(child.shift(), 30, CategoryMagic, true)
		states, err := p.projectCallbackDamage(&child.DamageContext, child.Status.Defeat, e, 30, true, rng)
		if err != nil {
			return err
		}
		for _, next := range states {
			status := child.Status
			status.EffectContext = next.Context.EffectContext
			// The returned recipient can differ from the submitted target. A
			// subsequent status helper resolves again after damage/defeat/reset.
			if t := next.State.actor(next.TargetID); t != nil && t.HP > 0 {
				if err := next.State.ValidateStatus(&status, controlSymbol(t.ID, StatusDeadlyPoison, CategoryMagic), 80, false, false, rng); err != nil {
					return err
				}
			}
		}
		return nil
	case "job-15-entry-2", "job-16-entry-2", "job-19-entry-2", "job-19-entry-3", "job-19-entry-4", "job-19-entry-7":
		e, chance, area, death := advancedMagicStatus(id)
		if area {
			return p.ValidateAreaStatus(&child.Status, e, chance, false, death, rng)
		}
		e.TargetID = child.shift()
		return p.ValidateStatus(&child.Status, e, chance, false, death, rng)
	case "job-15-entry-4", "job-19-entry-6":
		coefficient := .2
		if id == "job-19-entry-6" {
			coefficient = .25
		}
		if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(child.shift(), coefficient, CategoryMagic), rng); err != nil {
			return err
		}
		if p.actor(child.ActorID).categorySealed(CategoryMagic) {
			p.actor(child.ActorID).Status = ""
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 1, CategoryMagic), rng)
	case "job-15-entry-5":
		if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), 150, CategoryMagic, true), rng); err != nil {
			return err
		}
		if p.actor(child.ActorID).categorySealed(CategoryMagic) {
			p.actor(child.ActorID).Status = ""
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 1, CategoryMagic), rng)
	case "job-16-entry-0", "job-16-entry-3":
		power := 60.0
		if id == "job-16-entry-3" {
			power = 180
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), power, CategoryMagic), rng)
	case "job-19-entry-0":
		return p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect("", .25, CategoryMagic), rng)
	case "job-19-entry-5":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 125, CategoryMagic, true), rng)
	case "job-19-entry-8":
		return p.validateAdvancedMagicLoop(&child, rng)
	case "job-15-entry-1", "job-15-entry-6", "job-15-entry-7", "job-16-entry-7":
		power := 35.0
		switch id {
		case "job-15-entry-6":
			power = 260
		case "job-15-entry-7":
			power = 500 * (p.actor(child.ActorID).Tension + 5) / 6
		case "job-16-entry-7":
			power = 180
		}
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), power, CategoryMagic, true), rng)
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateAdvancedMagicLoop(c *AdvancedMagicContext, rng random.Generator) error {
	if _, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, controlSymbol("", "mixed-death-magic", CategoryUntyped)); err != nil {
		return err
	}
	if len(c.EnemyIDs) == 0 {
		return nil
	}
	states := []damageContinuation{{State: s.callbackCopy(), Context: c.DamageContext}}
	for hit := range 6 {
		var next []damageContinuation
		for _, state := range states {
			if state.State.actor(c.ActorID).HP <= 0 {
				continue
			}
			child := *c
			child.DamageContext = state.Context
			selection := controlSymbol("", "mixed-death-magic", CategoryUntyped)
			selection, _, err := state.State.actor(c.ActorID).ConvertEnemyCategory(child.TargetContext, selection)
			if err != nil {
				return err
			}
			candidates, err := state.State.statusTargetCandidates(&child.DamageContext, selection)
			if err != nil {
				return err
			}
			// Fallen/empty selections produce no first tuple, then call death
			// with an absent target, preserving its own random target resolution.
			missing := len(child.EnemyIDs) == 0
			for _, id := range child.EnemyIDs {
				missing = missing || state.State.actor(id).HP <= 0
			}
			if missing {
				candidates = append(candidates, TargetResolution{})
			}
			for _, candidate := range candidates {
				p := state.State.callbackCopy()
				applyStatusResources(&p, candidate)
				t := p.actor(candidate.TargetID)
				child.Status.EffectContext = child.EffectContext
				if t == nil || t.HP <= 999 && t.BaseStats.Defense <= 999 {
					e := controlSymbol(candidate.TargetID, "death", CategoryMagic)
					continuations, err := p.projectDeathTargets(&child.Status, e, 20, false, hit < 5, []string{candidate.TargetID}, rng)
					if err != nil {
						return err
					}
					for _, continuation := range continuations {
						next = appendDamageContinuation(next, continuation, child.DamageContext, "")
					}
				} else {
					e := dualAgilityEffect(t.ID, 400, CategoryMagic, true)
					continuations, err := p.projectCallbackDamage(&child.DamageContext, child.Status.Defeat, e, 600, hit < 5, rng)
					if err != nil {
						return err
					}
					for _, continuation := range continuations {
						next = appendDamageContinuation(next, continuation.State, continuation.Context, "")
					}
				}
			}
		}
		states = next
	}
	return nil
}
