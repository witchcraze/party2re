package battle

import "github.com/witchcraze/party2re/internal/core/random"

// TreatmentMischiefContext shares operation counters and the native status/
// defeat owner. Consumers rebuild hooks from serialized facts after reload.
type TreatmentMischiefContext struct {
	DamageContext
	Arguments []string
	Status    StatusContext
}

func (c *TreatmentMischiefContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func treatmentHealing(id string) float64 {
	switch id {
	case "job-50-entry-0":
		return 80
	case "job-50-entry-4":
		return 200
	case "job-57-entry-0":
		return 30
	case "job-57-entry-2":
		return 90
	}
	return 0
}

func treatmentAreaDamage(id string, a *LifetimeActor) (TargetEffect, bool) {
	e := dualAgilityEffect("", 0, CategoryMagic, true)
	switch id {
	case "job-51-entry-4":
		e.Payload.Number = 170
	case "job-55-entry-4":
		e = dualAgilityEffect("", float64(a.Agility)*1.4, CategoryPhysical, false)
	case "job-56-entry-0":
		e.Payload.Number = 25
	case "job-56-entry-2":
		e.Payload.Number = 70
	case "job-56-entry-4":
		e.Payload.Number = 160
	default:
		return e, false
	}
	return e, true
}

// ProjectTreatmentMischief supplies only the exact, unconditional inline body.
// All random transitions retain the invocation owner's conservative gate.
func (s *LifetimeState) ProjectTreatmentMischief(c *TreatmentMischiefContext, id string) bool {
	if id != "job-51-entry-1" {
		return false
	}
	for _, target := range c.PartyIDs {
		s.actor(target).Hit = 95
	}
	return true
}

// ApplyTreatmentMischief executes intrinsic bodies; enclosing invocation owns
// listed MP charges and random/delegated MP decrease restoration.
func (s *LifetimeState) ApplyTreatmentMischief(c *TreatmentMischiefContext, id string, rng random.Generator) error {
	if err := s.ValidateTreatmentMischief(c, id, rng); err != nil {
		return err
	}
	if s.ProjectTreatmentMischief(c, id) {
		return nil
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	a := s.actor(c.ActorID)
	if power := treatmentHealing(id); power != 0 {
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), rng)
		return err
	}
	if e, ok := treatmentAreaDamage(id, a); ok {
		_, err := s.ApplyAreaDamage(&c.DamageContext, e.Payload.Number, e.Category, len(e.Arguments) > 0, rng)
		return err
	}
	switch id {
	case "job-50-entry-1":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "accuracy", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			s.actor(r.TargetID).Hit = 95
		}
	case "job-50-entry-2":
		c.Status.EffectContext = c.EffectContext
		_, err := s.ApplyStatus(&c.Status, controlSymbol(c.shift(), "death", CategoryMagic), 17, false, true, rng)
		c.EffectContext = c.Status.EffectContext
		return err
	case "job-50-entry-3":
		r, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.shift(), .4, CategoryMagic), StatDefense, rng)
		if err != nil {
			return err
		}
		s.actor(r.TargetID).Temporary = TemporaryReduceBreath
	case "job-51-entry-0":
		_, err := s.ApplyPartyStat(&c.EffectContext, .2, CategoryMagic, StatHit, true, rng)
		return err
	case "job-51-entry-2":
		_, err := s.ApplyPartyHealing(&c.EffectContext, float64(int(rng.Float64()*50+50)), CategoryMagic, rng)
		return err
	case "job-51-entry-3":
		c.Status.EffectContext = c.EffectContext
		for _, target := range c.EnemyIDs {
			status := []string{StatusConfusion, StatusSleep}[int(rng.Float64()*2)]
			if _, err := s.ApplyStatus(&c.Status, controlSymbol(target, status, CategoryMagic), 50, false, false, rng); err != nil {
				return err
			}
		}
		c.EffectContext = c.Status.EffectContext
	case "job-51-entry-6":
		c.AdditionalEffect = true
		c.Status.EffectContext = c.EffectContext
		if _, err := s.ApplyAreaStatus(&c.Status, controlSymbol("", "death", CategoryMagic), 10, false, true, rng); err != nil {
			return err
		}
		c.EffectContext = c.Status.EffectContext
		_, err := s.ApplyAreaDamage(&c.DamageContext, 70, CategoryMagic, true, rng)
		return err
	case "job-51-entry-7":
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.MaxMP)*.7, CategoryMagic, true), rng)
		if err != nil || !r.Returned || r.TargetID == "" {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(int(float64(r.Amount)*.05)), CategoryMagic), rng)
		return err
	case "job-55-entry-0", "job-55-entry-2":
		t := &s.Actors[int(rng.Float64()*float64(len(s.Actors)))]
		if id == "job-55-entry-0" {
			t.Hit = int(float64(t.Hit) * .5)
		} else {
			t.Status, t.Tension = StatusMagicSeal, 1
		}
	case "job-55-entry-1", "job-57-entry-5":
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .15, CategoryPhysical), StatMP, rng)
		if err != nil || !r.Returned {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryPhysical), rng)
		return err
	case "job-55-entry-3":
		for _, member := range s.Actors {
			if member.HP <= 0 || member.ID == c.ActorID || rng.Float64()*2 < 1 {
				continue
			}
			if err := s.statTension(&c.EffectContext, s.actor(member.ID), false, rng); err != nil {
				return err
			}
		}
	case "job-55-entry-5":
		target := c.shift()
		if s.actor(target) == nil {
			r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(target, "tension", CategoryUntyped), rng)
			if err != nil {
				return err
			}
			resources(r)
			target = r.TargetID
			if !r.Resolved {
				target = ""
			}
		}
		if target == "" {
			target = c.ActorID
		}
		return s.statTension(&c.EffectContext, s.actor(target), false, rng)
	case "job-56-entry-1":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil {
			return err
		}
		if allowed {
			a.Temporary = TemporaryAbsorbMagic
		}
	case "job-57-entry-1", "job-57-entry-3":
		c.AdditionalEffect = true
		coefficient := 1.2
		if id == "job-57-entry-3" {
			coefficient = 1.1
		}
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Attack)*coefficient, CategoryPhysical, false), rng)
		if err != nil || !r.Returned || r.TargetID == "" || s.actor(r.TargetID).HP <= 0 {
			return err
		}
		if id == "job-57-entry-1" {
			_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .07, CategoryPhysical), StatHit, rng)
		} else {
			c.Status.EffectContext = c.EffectContext
			_, err = s.ApplyStatus(&c.Status, controlSymbol(r.TargetID, StatusSleep, CategoryPhysical), 55, false, false, rng)
			c.EffectContext = c.Status.EffectContext
		}
		return err
	case "job-57-entry-6":
		_, err := s.ApplyPartyStat(&c.EffectContext, .7, CategoryMagic, StatAttack, false, rng)
		return err
	case "job-78-entry-3":
		a.Defense -= int(float64(a.Defense) * .2)
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(a.ID, 200, CategoryUntyped), rng)
		return err
	}
	return nil
}
