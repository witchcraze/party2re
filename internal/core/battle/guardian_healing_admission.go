package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateGuardianHealing checks complete descendants on private state.
func (s *LifetimeState) ValidateGuardianHealing(c *GuardianHealingContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	switch id {
	case "job-17-entry-0":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "cover", CategoryPhysical), rng)
	case "job-17-entry-1":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), 30, CategoryMagic), rng)
	case "job-17-entry-2":
		return nil
	case "job-17-entry-3", "job-18-entry-2", "job-18-entry-5":
		for _, target := range child.PartyIDs {
			e := controlSymbol(target, StatusParalyze, CategoryMagic)
			if id == "job-18-entry-2" {
				e.Payload.Symbol, e.Category = StatusSleep, CategorySong
			}
			if id == "job-18-entry-5" {
				e = controlEffect(target, 120, CategoryMagic)
			}
			if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
				return err
			}
		}
		return nil
	case "job-17-entry-5":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", float64(p.actor(c.ActorID).Defense)*1.5, CategoryPhysical, false), rng)
	case "job-18-entry-0":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), StatusDeadlyPoison, CategoryMagic), rng)
	case "job-18-entry-1":
		return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{StatusDanceSeal}, CategoryDance, 70, rng)
	case "job-18-entry-3":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "reflection", CategoryMagic), rng)
	case "job-18-entry-7":
		return p.validateRestorationStrike(&child, rng)
	}
	return ErrUnsupportedEffect
}

// Healing cannot defeat a living participant or change gear. Track ordered
// revivals and bound returned power; the damage owner admits every defeat tail.
func (s *LifetimeState) validateRestorationStrike(c *GuardianHealingContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	a.Tension = 1
	status := a.Status
	states := []string{status}
	if a.categorySealed(CategoryMagic) {
		states = append(states, "")
	}
	total := 0.0
	for _, target := range c.PartyIDs {
		t := s.actor(target)
		if t.MaxHP < 0 {
			return ErrInvalidParticipant
		}
		if t.HP <= 0 {
			t.HP = 1
		}
		e := controlEffect(target, float64(min(300, int(float64(t.MaxHP)*.3))), CategoryMagic)
		maximum := 0.0
		for _, state := range states {
			a.Status = state
			if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
				return err
			}
			for _, candidate := range s.partyTargetCandidates(c.TargetContext, e, nil) {
				if s.actor(candidate.TargetID).HP <= 0 || candidate.Payload.Number <= 0 {
					continue
				}
				power := candidate
				power.Payload.Number = s.healingBasePower(&c.EffectContext, candidate)
				v := s.finalEffectPower(&c.EffectContext, s.initialEffectPower(&c.EffectContext, power, ModifierHPHealing))
				if a.Status == StatusSabaku {
					v *= .9
				}
				if s.actor(candidate.TargetID).Status == StatusVirulentPoison {
					v = float64(int(v * .1))
				}
				maximum = max(maximum, float64(int(v*1.2)))
			}
		}
		total += maximum
	}
	if s.SkillHistory[a.TeamID]["job-18-entry-7"] {
		return nil
	}
	target := c.shift()
	for _, state := range states {
		a.Status = state
		if err := s.ValidateDamage(&c.DamageContext, dualAgilityEffect(target, total, CategoryMagic, true), rng); err != nil {
			return err
		}
	}
	return nil
}

// ProjectGuardianHealing exposes only the exact unsealed party barrier write.
func (s *LifetimeState) ProjectGuardianHealing(c *GuardianHealingContext, id string) (bool, error) {
	if err := s.ValidateGuardianHealing(c, id, random.Default()); err != nil {
		return false, err
	}
	if id != "job-17-entry-2" || s.actor(c.ActorID).categorySealed(CategoryMagic) {
		return false, nil
	}
	if err := s.ApplyGuardianHealing(c, id, random.Default()); err != nil {
		return false, err
	}
	return true, nil
}
