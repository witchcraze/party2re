package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateIncidentalJob admits complete descendants on private state before
// costs, writes, external Apply hooks, argument consumption or committed RNG.
func (s *LifetimeState) ValidateIncidentalJob(c *IncidentalJobContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for job := 7; job <= 9; job++ {
		for pos, entry := range IncidentalJobEntries(job) {
			known = known || entry.ID == id && incidentalJobOwns(job, pos)
		}
	}
	if !known {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	child.Status.EffectContext = child.EffectContext
	if taunt := incidentalTaunt(id); taunt != "" {
		context := TreatmentMischiefContext{DamageContext: child.DamageContext, Arguments: child.Arguments, Status: child.Status}
		return p.ValidateTreatmentMischief(&context, taunt, rng)
	}
	if incidentalRaw(id) {
		return nil
	}
	if symbol, category, chance := incidentalAreaStatus(id); symbol != "" {
		return p.ValidateAreaStatus(&child.Status, controlSymbol("", symbol, category), chance, false, false, rng)
	}
	a := p.actor(c.ActorID)
	switch id {
	case "job-7-entry-1":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .4, CategoryPhysical), StatDefense, rng)
	case "job-7-entry-3":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .2, CategoryBreath), rng); err != nil {
				return err
			}
		}
	case "job-7-entry-5", "job-9-entry-7":
		power := float64(a.Defense) * 1.6
		if id == "job-9-entry-7" {
			power, child.AdditionalEffect = float64(a.Attack)*1.2, true
		}
		e := dualAgilityEffect(child.shift(), power, CategoryPhysical, false)
		states, err := p.projectCallbackDamage(&child.DamageContext, child.Status.Defeat, e, power, true, rng)
		if err != nil {
			return err
		}
		for _, next := range states {
			if next.TargetID == "" {
				continue
			}
			if id == "job-7-entry-5" {
				if _, err := next.State.ApplyRecoil(&next.Context.EffectContext, &next.Context.End, 0, resetBoundary(0)); err != nil {
					return err
				}
			} else if next.State.actor(next.TargetID).HP > 0 {
				status := child.Status
				status.EffectContext = next.Context.EffectContext
				if err := next.State.ValidateStatus(&status, controlSymbol(next.TargetID, StatusParalyze, CategoryPhysical), 30, false, false, rng); err != nil {
					return err
				}
			}
		}
	case "job-7-entry-6":
		e := controlSymbol(child.shift(), "share-mana", CategoryMagic)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		if a.categorySealed(CategoryMagic) {
			a.Status = ""
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			q := p.callbackCopy()
			amount := min(100, q.actor(c.ActorID).MP/2)
			if err := q.validateRecovery(&child.EffectContext, controlEffect(candidate.TargetID, float64(amount), CategoryMagic), rng); err != nil {
				return err
			}
			q.actor(c.ActorID).MP -= amount
			if err := q.validatePartyTarget(child.TargetContext, controlEffect(candidate.TargetID, float64(amount), CategoryMagic), rng); err != nil {
				return err
			}
		}
	case "job-8-entry-1":
		a.Status = StatusSleep
		e := controlEffect(a.ID, float64(a.MaxHP)*.5, CategoryUntyped)
		if err := p.validateRecovery(&child.EffectContext, e, rng); err != nil {
			return err
		}
		return p.validatePartyTarget(child.TargetContext, e, rng)
	case "job-9-entry-0":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), float64(a.Agility)*1.4, CategoryPhysical, false), rng)
	case "job-9-entry-1":
		return p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(child.shift(), .4, CategoryMagic), rng)
	case "job-9-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), .4, CategoryMagic), StatAgility, rng)
	case "job-9-entry-8":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "armor-break", CategoryPhysical), false)
		if err != nil {
			return err
		}
		if a.categorySealed(CategoryPhysical) {
			a.Status = ""
		}
		for _, target := range targets {
			if target == "" {
				continue
			}
			q := p.callbackCopy()
			t := q.actor(target)
			t.Temporary, t.Tension, t.Defense = "", 1, t.BaseStats.Defense
			if err := q.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .1, CategoryPhysical), rng); err != nil {
				return err
			}
		}
	case "job-9-entry-9":
		return p.validateIncidentalDrain(&child, rng)
	}
	return nil
}

func (s *LifetimeState) validateIncidentalDrain(c *IncidentalJobContext, rng random.Generator) error {
	targets, err := s.controlTargets(&ControlClimaxContext{DamageContext: c.DamageContext}, controlSymbol(c.shift(), "drain", CategoryUntyped), false)
	if err != nil {
		return err
	}
	for _, target := range targets {
		p, child := s.callbackCopy(), *c
		hit, defense := p.incidentalDrainPrepare(&child, target)
		e := dualAgilityEffect(target, float64(p.actor(c.ActorID).Agility)*1.7, CategoryPhysical, false)
		states, err := p.projectCallbackDamage(&child.DamageContext, child.Status.Defeat, e, e.Payload.Number, true, rng)
		if err != nil {
			return err
		}
		for _, next := range states {
			if next.TargetID == "" {
				continue
			}
			// Restore the original selected target even when damage redirects,
			// defeats or resets it, then resolve healing from the resulting facts.
			next.State.incidentalDrainRestore(&child, target, hit, defense)
			if err := next.State.validatePartyTarget(next.Context.TargetContext, controlEffect(c.ActorID, 1, CategoryPhysical), rng); err != nil {
				return err
			}
		}
	}
	return nil
}
