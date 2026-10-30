package battle

import "github.com/witchcraze/party2re/internal/core/random"

// AdvancedMagicContext retains operation facts; consumers rebuild ports after reload.
type AdvancedMagicContext struct {
	DamageContext
	Arguments []string
	Status    StatusContext
}

func (c *AdvancedMagicContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func advancedMagicTemporary(id string) TemporaryEffect {
	switch id {
	case "job-15-entry-3", "job-19-entry-1":
		return TemporaryReflectMagic
	case "job-16-entry-4":
		return TemporaryRevival
	case "job-16-entry-5":
		return TemporaryReduceMagic
	case "job-16-entry-8":
		return TemporaryNullAttack
	}
	return ""
}

// ApplyAdvancedMagic composes intrinsic bodies. Listed ordinary costs and
// indirect restoration/debits remain with the invoking consumer.
func (s *LifetimeState) ApplyAdvancedMagic(c *AdvancedMagicContext, id string, rng random.Generator) error {
	if err := s.ValidateAdvancedMagic(c, id, rng); err != nil {
		return err
	}
	if id == "job-15-entry-0" || id == "job-19-entry-8" {
		previousReset, previousDefeat := c.Reset, c.Defeat
		damage := s.nativeDamageContext(c.DamageContext, c.Status.Defeat, rng)
		c.Reset, c.Defeat = damage.Reset, damage.Defeat
		defer func() { c.Reset, c.Defeat = previousReset, previousDefeat }()
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	c.Status.EffectContext = c.EffectContext
	defer func() { c.Status.EffectContext = c.EffectContext }()
	if tmp := advancedMagicTemporary(id); tmp != "" {
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "temporary:"+string(tmp), CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			s.actor(r.TargetID).Temporary = tmp
		}
		if id == "job-16-entry-8" {
			_, err = s.ApplyPartyHealing(&c.EffectContext, 300, CategoryMagic, rng)
		}
		return err
	}
	switch id {
	case "job-15-entry-0":
		c.AdditionalEffect = true
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), 30, CategoryMagic, true), rng)
		if err != nil || !r.Returned || r.TargetID == "" || s.actor(r.TargetID).HP <= 0 {
			return err
		}
		c.Status.EffectContext = c.EffectContext
		_, err = s.ApplyStatus(&c.Status, controlSymbol(r.TargetID, StatusDeadlyPoison, CategoryMagic), 80, false, false, rng)
		c.EffectContext = c.Status.EffectContext
		return err
	case "job-15-entry-2", "job-16-entry-2", "job-19-entry-2", "job-19-entry-3", "job-19-entry-4", "job-19-entry-7":
		e, chance, area, death := advancedMagicStatus(id)
		var err error
		if area {
			_, err = s.ApplyAreaStatus(&c.Status, e, chance, false, death, rng)
		} else {
			e.TargetID = c.shift()
			_, err = s.ApplyStatus(&c.Status, e, chance, false, death, rng)
		}
		c.EffectContext = c.Status.EffectContext
		return err
	case "job-15-entry-4", "job-19-entry-6":
		coefficient := .2
		if id == "job-19-entry-6" {
			coefficient = .25
		}
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), coefficient, CategoryMagic), StatMP, rng)
		if err != nil || !r.Returned || r.TargetID == "" {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryMagic), rng)
		return err
	case "job-15-entry-5":
		power := 50 + rng.Float64()*100
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryMagic, true), rng)
		if err != nil || !r.Returned || r.TargetID == "" {
			return err
		}
		_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryMagic), rng)
		return err
	case "job-16-entry-0", "job-16-entry-3":
		power := 60.0
		if id == "job-16-entry-3" {
			power = 180
		}
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), rng)
		return err
	case "job-19-entry-0":
		_, err := s.ApplyPartyStat(&c.EffectContext, .25, CategoryMagic, StatDefense, true, rng)
		return err
	case "job-19-entry-5":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 125, CategoryMagic, true, rng)
		return err
	case "job-19-entry-8":
		return s.applyAdvancedMagicLoop(c, rng)
	}
	power := 35.0
	if id == "job-15-entry-6" {
		power = 260
	} else if id == "job-15-entry-7" {
		power = 500 * (s.actor(c.ActorID).Tension + 5) / 6
	} else if id == "job-16-entry-7" {
		power = 180
	}
	_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryMagic, true), rng)
	return err
}

func advancedMagicStatus(id string) (TargetEffect, float64, bool, bool) {
	symbol, chance, area, death := StatusSleep, 55.0, false, false
	switch id {
	case "job-16-entry-2":
		symbol, chance = StatusMagicSeal, 90
	case "job-19-entry-2":
		symbol, chance, area = StatusConfusion, 50, true
	case "job-19-entry-4":
		symbol, chance, area = StatusMagicSeal, 70, true
	case "job-19-entry-3", "job-19-entry-7":
		symbol, chance, area, death = "death", 20, id == "job-19-entry-7", true
	}
	return controlSymbol("", symbol, CategoryMagic), chance, area, death
}

func (s *LifetimeState) applyAdvancedMagicLoop(c *AdvancedMagicContext, rng random.Generator) error {
	hits := 4 + int(rng.Float64()*3)
	for range hits {
		if s.actor(c.ActorID).HP <= 0 {
			break
		}
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol("", "mixed-death-magic", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		c.targetResources(r)
		target := ""
		if r.Resolved {
			target = r.TargetID
		}
		t := s.actor(target)
		if t == nil || t.HP <= 999 && t.BaseStats.Defense <= 999 {
			c.Status.EffectContext = c.EffectContext
			_, err = s.ApplyStatus(&c.Status, controlSymbol(target, "death", CategoryMagic), 20, false, true, rng)
			c.EffectContext = c.Status.EffectContext
		} else {
			power := 400 + rng.Float64()*200
			_, err = s.ApplyDamage(&c.DamageContext, dualAgilityEffect(t.ID, power, CategoryMagic, true), rng)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
