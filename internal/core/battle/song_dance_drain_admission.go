package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateSongDanceDrain admits complete descendants on private facts before
// costs, effects, argument consumption, hooks or committed random draws.
func (s *LifetimeState) ValidateSongDanceDrain(c *SongDanceDrainContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	switch id {
	case "job-14-entry-2", "job-20-entry-2":
		return nil
	case "job-13-entry-0", "job-14-entry-1":
		control := ControlClimaxContext{DamageContext: child.DamageContext}
		if id == "job-14-entry-1" {
			return p.validateControlDown(&control, controlEffect(child.shift(), .4, CategoryDance), rng)
		}
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&control, controlEffect(target, .25, CategorySong), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-13-entry-1", "job-14-entry-6":
		power, category := 30.0, CategorySong
		if id == "job-14-entry-6" {
			power, category = 140, CategoryDance
		}
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlEffect(target, power, category), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-13-entry-3", "job-13-entry-5":
		coefficient, stat := .4, StatDefense
		if id == "job-13-entry-5" {
			coefficient, stat = .6, StatAttack
		}
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e := controlEffect(target, coefficient, CategorySong)
			if err := p.validateStatUp(&child.EffectContext, e, stat, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, stat, &admission)
		}
		return nil
	case "job-14-entry-0":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .4, CategoryDance), StatAgility, rng)
	case "job-14-entry-3":
		return p.validateAreaDamage(&child.DamageContext, controlEffect("", float64(a.Attack)*.8, CategoryPhysical), rng)
	case "job-14-entry-5":
		if a.HP <= 0 {
			return nil
		}
		return p.validateRepeatedDamage(&child.DamageContext, controlEffect("", float64(a.Attack)*.8, CategoryDance), 4, rng)
	case "job-20-entry-1", "job-20-entry-6":
		coefficient := .9
		if id == "job-20-entry-6" {
			coefficient = 1.7
		}
		if err := p.ValidateDamage(&child.DamageContext, controlEffect(child.shift(), float64(a.Attack)*coefficient, CategoryPhysical), rng); err != nil {
			return err
		}
		// A successful category cure and item195 activation precede recovery's
		// independent party-target helper. Explicit self remains its argument.
		converted, _, err := a.ConvertEnemyCategory(child.TargetContext, controlEffect("", 1, CategoryPhysical))
		if err != nil {
			return err
		}
		if a.categorySealed(converted.Category) {
			a.Status = ""
		}
		child.Item195Activated = true
		return p.validatePartyTarget(child.TargetContext, controlEffect(c.ActorID, 1, CategoryPhysical), rng)
	}
	if child.Status == nil {
		return ErrUnsupportedEffect
	}
	status := *child.Status
	status.EffectContext = child.EffectContext
	switch id {
	case "job-13-entry-2":
		for _, target := range child.PartyIDs {
			if err := p.validatePartyTarget(child.TargetContext, controlSymbol(target, StatusSleep, CategorySong), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-13-entry-4":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusSleep, CategorySong), 50, false, false, rng)
	case "job-20-entry-0":
		return p.ValidateStatus(&status, controlSymbol(child.shift(), StatusDofuu, CategoryDance), 85, false, false, rng)
	case "job-20-entry-3":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusSleep, CategoryBreath), 35, false, false, rng)
	case "job-20-entry-4":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusConfusion, CategoryDance), 60, false, false, rng)
	case "job-20-entry-5":
		return p.ValidateAreaStatus(&status, controlSymbol("", "instant_death", CategoryDance), 17, false, true, rng)
	}
	return ErrUnsupportedEffect
}
