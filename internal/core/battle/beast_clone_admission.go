package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateBeastClone admits complete descendants on isolated facts, before
// costs, argument consumption, RNG or external Apply hooks.
func (s *LifetimeState) ValidateBeastClone(c *BeastCloneContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	if power := beastBreathPower(id); power != 0 {
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", power, CategoryBreath, true), rng)
	}
	if status, category, chance, area := beastStatus(id); status != "" {
		if area {
			return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child.DamageContext}, []string{status}, category, chance, rng)
		}
		sc := child.statusContext()
		return p.ValidateStatus(&sc, controlSymbol(child.shift(), status, category), chance, false, id == "job-26-entry-5", rng)
	}
	a := p.actor(child.ActorID)
	switch id {
	case "job-10-entry-0", "job-12-entry-0", "job-10-entry-3":
		power := float64(a.HP) * .75
		if id == "job-10-entry-3" {
			power = float64(a.Defense) * 1.6
		}
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), power, CategoryPhysical, false), rng)
	case "job-10-entry-1":
		a.Status = StatusSleep
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, float64(a.MaxHP)*.5, CategoryUntyped), rng)
	case "job-10-entry-2":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.shift(), .4, CategoryMagic), StatDefense, rng)
	case "job-10-entry-7":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, .5, CategoryUntyped), StatDefense, rng)
	case "job-26-entry-6":
		return p.ValidateStatUp(&child.EffectContext, controlEffect(child.ActorID, 1, CategoryUntyped), StatAgility, rng)
	case "job-10-entry-4":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.shift(), 90, CategoryMagic), rng)
	case "job-10-entry-6":
		return nil
	case "job-10-entry-8", "job-12-entry-7":
		if a.HP <= 0 {
			return nil
		}
		e, count := dualAgilityEffect("", float64(a.Agility)*1.6, CategoryPhysical, false), 4
		if id == "job-12-entry-7" {
			e.TargetID, e.Payload.Number, count = child.shift(), float64(a.Attack)*1.6, 2
		}
		return p.validateRepeatedDamage(&child.DamageContext, e, count, rng)
	case "job-12-entry-4":
		child.AdditionalEffect = true
		return p.validateBeastParalysis(&child, child.shift(), rng)
	case "job-26-entry-2":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .2, CategoryMagic), rng); err != nil {
				return err
			}
		}
		return nil
	case "job-26-entry-4":
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, target := range child.PartyIDs {
			e := controlEffect(target, .25, CategoryMagic)
			if err := p.validateStatUp(&child.EffectContext, e, StatAgility, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(&child.EffectContext, e, StatAgility, &admission)
		}
		return nil
	case "job-26-entry-7":
		high := HighPowerContext{DamageContext: child.DamageContext, Arguments: child.Arguments}
		return p.ValidateHighPower(&high, "job-52-entry-2", rng)
	case "job-26-entry-8":
		if p.rosterIdentity("cover-clone") != nil {
			return nil
		}
		roster := RosterContext{DamageContext: child.DamageContext, RequesterItemID: child.RequesterItemID}
		if err := validateSummonRequester(&roster); err != nil {
			return err
		}
		return p.validateHelper(&roster, coverClonePayload(*child.RequesterLevel), rng)
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateBeastParalysis(c *BeastCloneContext, target string, rng random.Generator) error {
	e := dualAgilityEffect(target, float64(s.actor(c.ActorID).Attack)*1.1, CategoryPhysical, false)
	if err := s.ValidateDamage(&c.DamageContext, e, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	a := p.actor(c.ActorID)
	if a.categorySealed(CategoryPhysical) {
		a.Status = ""
	}
	converted, _, err := a.ConvertEnemyCategory(child.TargetContext, e)
	if err != nil {
		return err
	}
	for _, candidate := range p.damageCandidates(&child.DamageContext, converted) {
		t := p.actor(candidate.TargetID)
		if t.HP <= 0 {
			t = a
		}
		if candidate.Payload.Number == 0 || t.Temporary == TemporaryOneHitDefense || !damageHasPower(a, t, candidate) {
			continue
		}
		q, tail := p.callbackCopy(), child
		target := q.actor(t.ID)
		// Positive returned damage can cross current-HP immunity. Keep each
		// possible recipient separate; a blocked/missed hit has no status tail.
		target.HP = min(target.HP, 999)
		tail.DamageCalls++
		tail.Item195Activated = tail.Item195Activated || hasItem(target.ItemDefinitionIDs, "item-195")
		sc := tail.statusContext()
		if err := q.ValidateStatus(&sc, controlSymbol(target.ID, StatusParalyze, CategoryPhysical), 40, false, false, rng); err != nil {
			return err
		}
	}
	return nil
}
